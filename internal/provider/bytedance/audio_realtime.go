package bytedance

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/gorilla/websocket"
)

const (
	defaultRealtimeDialogueURL        = "wss://openspeech.bytedance.com/api/v3/realtime/dialogue"
	defaultRealtimeDialogueResourceID = "volc.speech.dialog"
	defaultRealtimeDialogueAppKey     = "PlgvMymc7f3tQnJ6"
	defaultRealtimeModelVersion       = "1.2.1.1"
	realtimeAuthAccessToken           = "access_token"
	realtimeAuthAPIKey                = "api_key"
	realtimeBotName                   = "Doubao"
	realtimeReadyTimeout              = 10 * time.Second
	realtimeUsageGracePeriod          = 150 * time.Millisecond
)

type realtimeAudioAdapter struct {
	client         *Client
	model          string
	url            string
	providerModel  string
	resourceID     string
	appKey         string
	authMode       string
	defaultSession time.Duration
	turnModes      map[string]struct{}
}

type realtimeAudioSession struct {
	ctx          context.Context
	adapter      *realtimeAudioAdapter
	cfg          modality.AudioSessionConfig
	events       chan modality.AudioServerEvent
	closeCh      chan struct{}
	sequence     uint64
	sessionID    string
	connectID    string
	started      bool
	closed       bool
	conn         *websocket.Conn
	pendingText  string
	pendingAudio int
	dialogID     string
	currentTurn  *realtimeTurnState
	writeMu      sync.Mutex
	mu           sync.Mutex
	startOnce    sync.Once
	startErr     error
	startReady   chan error
	closeOnce    sync.Once
}

type realtimeTurnState struct {
	responseID      string
	questionID      string
	replyID         string
	text            strings.Builder
	transcript      strings.Builder
	audio           []byte
	audioDone       bool
	textDone        bool
	usage           *modality.AudioUsage
	completed       bool
	completionTimer *time.Timer
}

type realtimeSessionStartedPayload struct {
	DialogID string `json:"dialog_id"`
}

type realtimeUsagePayload struct {
	Usage struct {
		InputTextTokens   int `json:"input_text_tokens"`
		InputAudioTokens  int `json:"input_audio_tokens"`
		CachedTextTokens  int `json:"cached_text_tokens"`
		CachedAudioTokens int `json:"cached_audio_tokens"`
		OutputTextTokens  int `json:"output_text_tokens"`
		OutputAudioTokens int `json:"output_audio_tokens"`
	} `json:"usage"`
}

type realtimeTTSStartPayload struct {
	TTSType    string `json:"tts_type"`
	Text       string `json:"text"`
	QuestionID string `json:"question_id"`
	ReplyID    string `json:"reply_id"`
}

type realtimeASRPayload struct {
	Results []struct {
		Text      string `json:"text"`
		IsInterim bool   `json:"is_interim"`
	} `json:"results"`
}

type realtimeChatPayload struct {
	Content    string `json:"content"`
	QuestionID string `json:"question_id"`
	ReplyID    string `json:"reply_id"`
}

type realtimeEndedPayload struct {
	QuestionID string `json:"question_id"`
	ReplyID    string `json:"reply_id"`
	StatusCode string `json:"status_code"`
}

type realtimeCommonErrorPayload struct {
	Error      string `json:"error"`
	Message    string `json:"message"`
	StatusCode string `json:"status_code"`
}

type realtimeStartSessionRequest struct {
	TTS    realtimeStartSessionTTS    `json:"tts,omitempty"`
	ASR    realtimeStartSessionASR    `json:"asr,omitempty"`
	Dialog realtimeStartSessionDialog `json:"dialog"`
}

type realtimeStartSessionTTS struct {
	Speaker     string                          `json:"speaker,omitempty"`
	AudioConfig realtimeStartSessionAudioConfig `json:"audio_config"`
}

type realtimeStartSessionAudioConfig struct {
	Channel    int    `json:"channel"`
	Format     string `json:"format"`
	SampleRate int    `json:"sample_rate"`
}

type realtimeStartSessionASR struct {
	AudioInfo realtimeStartSessionASRAudioInfo `json:"audio_info"`
}

type realtimeStartSessionASRAudioInfo struct {
	Format     string `json:"format"`
	SampleRate int    `json:"sample_rate"`
	Channel    int    `json:"channel"`
}

type realtimeStartSessionDialog struct {
	BotName    string                    `json:"bot_name,omitempty"`
	SystemRole string                    `json:"system_role,omitempty"`
	DialogID   string                    `json:"dialog_id,omitempty"`
	Extra      realtimeStartSessionExtra `json:"extra"`
}

type realtimeStartSessionExtra struct {
	InputMod string `json:"input_mod,omitempty"`
	Model    string `json:"model"`
}

type realtimeUpdateConfigRequest struct {
	TTS    *realtimeUpdateTTS    `json:"tts,omitempty"`
	Dialog *realtimeUpdateDialog `json:"dialog,omitempty"`
}

type realtimeUpdateTTS struct {
	Speaker string `json:"speaker,omitempty"`
}

type realtimeUpdateDialog struct {
	SystemRole string `json:"system_role,omitempty"`
	DialogID   string `json:"dialog_id,omitempty"`
}

type realtimeTextQueryRequest struct {
	Content string `json:"content"`
}

func newRealtimeAudioAdapter(client *Client, model string, modelCfg config.ModelConfig) modality.AudioAdapter {
	transport := strings.TrimSpace(modelCfg.RealtimeSession.Transport)
	if !strings.EqualFold(transport, "bytedance_dialog") {
		return nil
	}
	providerModel := strings.TrimSpace(modelCfg.RealtimeSession.Model)
	if providerModel == "" {
		providerModel = defaultRealtimeModelVersion
	}
	url := strings.TrimSpace(modelCfg.RealtimeSession.URL)
	if url == "" {
		url = defaultRealtimeDialogueURL
	}
	resourceID := strings.TrimSpace(modelCfg.RealtimeSession.ResourceID)
	if resourceID == "" {
		resourceID = defaultRealtimeDialogueResourceID
	}
	appKey := strings.TrimSpace(modelCfg.RealtimeSession.AppKey)
	if appKey == "" {
		appKey = defaultRealtimeDialogueAppKey
	}
	authMode := normalizeRealtimeAuthMode(modelCfg.RealtimeSession.Auth)
	defaultSession := modelCfg.SessionTTL
	if defaultSession <= 0 {
		defaultSession = 10 * time.Minute
	}
	return &realtimeAudioAdapter{
		client:         client,
		model:          model,
		url:            url,
		providerModel:  providerModel,
		resourceID:     resourceID,
		appKey:         appKey,
		authMode:       authMode,
		defaultSession: defaultSession,
		turnModes: map[string]struct{}{
			modality.TurnDetectionManual:    {},
			modality.TurnDetectionServerVAD: {},
		},
	}
}

func (a *realtimeAudioAdapter) Connect(ctx context.Context, cfg *modality.AudioSessionConfig) (modality.AudioSession, error) {
	if a == nil || a.client == nil {
		return nil, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "adapter_unavailable", "", "Audio adapter is unavailable.")
	}
	switch a.authMode {
	case realtimeAuthAPIKey:
		if strings.TrimSpace(a.client.speechAPIKey) == "" {
			return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_misconfigured", "", "ByteDance native realtime audio with realtime_session.auth=api_key requires providers.bytedance.speech_api_key.")
		}
	default:
		if strings.TrimSpace(a.client.appID) == "" || strings.TrimSpace(a.client.speechToken) == "" {
			return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_misconfigured", "", "ByteDance native realtime audio requires providers.bytedance.app_id and providers.bytedance.speech_access_token.")
		}
	}
	normalized, err := a.normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &realtimeAudioSession{
		ctx:        ctx,
		adapter:    a,
		cfg:        *normalized,
		events:     make(chan modality.AudioServerEvent, 64),
		closeCh:    make(chan struct{}),
		sessionID:  newRealtimeSessionID(),
		connectID:  newRealtimeSessionID(),
		startReady: make(chan error, 1),
	}, nil
}

func (a *realtimeAudioAdapter) normalizeConfig(cfg *modality.AudioSessionConfig) (*modality.AudioSessionConfig, error) {
	if cfg == nil {
		cfg = &modality.AudioSessionConfig{}
	}
	normalized := *cfg
	if strings.TrimSpace(normalized.Model) == "" {
		normalized.Model = a.model
	}
	if strings.TrimSpace(normalized.InputAudioFormat) == "" {
		normalized.InputAudioFormat = modality.AudioFormatPCM16
	}
	if strings.TrimSpace(normalized.OutputAudioFormat) == "" {
		normalized.OutputAudioFormat = modality.AudioFormatPCM16
	}
	if normalized.SampleRateHz == 0 {
		normalized.SampleRateHz = 16000
	}
	if normalized.SampleRateHz != 16000 {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_sample_rate", "sample_rate_hz", "Only 16000 Hz audio sessions are supported.")
	}
	if normalized.InputAudioFormat != modality.AudioFormatPCM16 {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_input_audio_format", "input_audio_format", "Only pcm16 input audio is supported.")
	}
	if normalized.OutputAudioFormat != modality.AudioFormatPCM16 {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_output_audio_format", "output_audio_format", "Only pcm16 output audio is supported.")
	}
	if normalized.TurnDetection == nil {
		normalized.TurnDetection = &modality.TurnDetectionConfig{Mode: modality.TurnDetectionManual}
	}
	mode := strings.TrimSpace(normalized.TurnDetection.Mode)
	if mode == "" {
		mode = modality.TurnDetectionManual
	}
	if _, ok := a.turnModes[mode]; !ok {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_turn_detection", "turn_detection.mode", "Requested turn detection mode is not supported by this model.")
	}
	normalized.TurnDetection.Mode = mode
	return &normalized, nil
}

func (s *realtimeAudioSession) Send(event modality.AudioClientEvent) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return httputil.NewError(http.StatusGone, "invalid_request_error", "session_closed", "", "Audio session is closed.")
	}
	s.mu.Unlock()

	switch event.Type {
	case modality.AudioClientEventSessionUpdate:
		return s.updateSession(event.Session)
	case modality.AudioClientEventInputAudioAppend:
		return s.appendAudio(event.Audio)
	case modality.AudioClientEventInputAudioCommit:
		return s.commitAudio()
	case modality.AudioClientEventInputText:
		return s.setInputText(event.Text)
	case modality.AudioClientEventResponseCreate:
		return s.createResponse(event.Response)
	case modality.AudioClientEventResponseCancel:
		return s.cancelResponse()
	case modality.AudioClientEventSessionClose:
		return s.Close()
	default:
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unknown_event_type", "type", "Unknown audio client event type.")
	}
}

func (s *realtimeAudioSession) Events() <-chan modality.AudioServerEvent {
	return s.events
}

func (s *realtimeAudioSession) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		conn := s.conn
		started := s.started
		s.mu.Unlock()

		close(s.closeCh)
		if conn != nil && started {
			_, _ = s.writeJSONEvent(dialogMessageTypeFullClient, dialogEventFinishSession, map[string]any{})
			_, _ = s.writeJSONEvent(dialogMessageTypeFullClient, dialogEventFinishConnection, map[string]any{})
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
			_ = conn.Close()
		}
	})
	return nil
}

func (s *realtimeAudioSession) updateSession(update *modality.AudioSessionConfig) error {
	if update == nil {
		return nil
	}
	s.mu.Lock()
	if strings.TrimSpace(update.Model) != "" && strings.TrimSpace(update.Model) != strings.TrimSpace(s.cfg.Model) {
		s.mu.Unlock()
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "model_immutable", "model", "Audio session model cannot be changed after creation.")
	}
	if update.InputAudioFormat != "" && update.InputAudioFormat != modality.AudioFormatPCM16 {
		s.mu.Unlock()
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_input_audio_format", "input_audio_format", "Only pcm16 input audio is supported.")
	}
	if update.OutputAudioFormat != "" && update.OutputAudioFormat != modality.AudioFormatPCM16 {
		s.mu.Unlock()
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_output_audio_format", "output_audio_format", "Only pcm16 output audio is supported.")
	}
	if update.SampleRateHz != 0 && update.SampleRateHz != 16000 {
		s.mu.Unlock()
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_sample_rate", "sample_rate_hz", "Only 16000 Hz audio sessions are supported.")
	}
	if update.TurnDetection != nil {
		mode := strings.TrimSpace(update.TurnDetection.Mode)
		if mode == "" {
			mode = modality.TurnDetectionManual
		}
		if _, ok := s.adapter.turnModes[mode]; !ok {
			s.mu.Unlock()
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_turn_detection", "turn_detection.mode", "Requested turn detection mode is not supported by this model.")
		}
		if s.started && mode != s.cfg.TurnDetection.Mode {
			s.mu.Unlock()
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "turn_detection_immutable", "turn_detection.mode", "Turn detection mode cannot be changed after the native realtime session starts.")
		}
		s.cfg.TurnDetection = &modality.TurnDetectionConfig{
			Mode:            mode,
			SilenceMS:       update.TurnDetection.SilenceMS,
			PrefixPaddingMS: update.TurnDetection.PrefixPaddingMS,
		}
	}
	if strings.TrimSpace(update.Voice) != "" {
		s.cfg.Voice = strings.TrimSpace(update.Voice)
	}
	if strings.TrimSpace(update.Instructions) != "" {
		s.cfg.Instructions = strings.TrimSpace(update.Instructions)
	}
	alreadyStarted := s.started
	dialogID := s.dialogID
	voice := s.cfg.Voice
	instructions := s.cfg.Instructions
	s.mu.Unlock()

	if !alreadyStarted {
		s.emit(modality.AudioServerEvent{Type: modality.AudioServerEventSessionUpdated, EventID: s.nextEventID("evt")})
		return nil
	}

	req := realtimeUpdateConfigRequest{}
	if voice != "" {
		req.TTS = &realtimeUpdateTTS{Speaker: voice}
	}
	if instructions != "" || dialogID != "" {
		req.Dialog = &realtimeUpdateDialog{
			SystemRole: instructions,
			DialogID:   dialogID,
		}
	}
	if _, err := s.writeJSONEvent(dialogMessageTypeFullClient, dialogEventUpdateConfig, req); err != nil {
		return err
	}
	return nil
}

func (s *realtimeAudioSession) appendAudio(encoded string) error {
	payload, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_audio", "audio", "Audio payload must be valid base64.")
	}
	if len(payload) == 0 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_audio", "audio", "Audio payload must not be empty.")
	}
	if err := s.ensureStarted(); err != nil {
		return err
	}
	if _, err := s.writeAudioEvent(dialogEventTaskRequest, payload); err != nil {
		return err
	}
	s.mu.Lock()
	s.pendingAudio += len(payload)
	s.mu.Unlock()
	return nil
}

func (s *realtimeAudioSession) commitAudio() error {
	s.mu.Lock()
	pendingAudio := s.pendingAudio
	mode := s.cfg.TurnDetection.Mode
	s.mu.Unlock()
	if pendingAudio == 0 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_audio", "audio", "No buffered audio is available to commit.")
	}
	if mode != modality.TurnDetectionManual {
		return nil
	}
	if err := s.ensureStarted(); err != nil {
		return err
	}
	_, err := s.writeJSONEvent(dialogMessageTypeFullClient, dialogEventEndASR, map[string]any{})
	return err
}

func (s *realtimeAudioSession) setInputText(text string) error {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_text", "text", "Input text must not be empty.")
	}
	s.mu.Lock()
	s.pendingText = trimmed
	s.mu.Unlock()
	return nil
}

func (s *realtimeAudioSession) createResponse(response *modality.AudioResponseConfig) error {
	s.mu.Lock()
	pendingText := strings.TrimSpace(s.pendingText)
	s.mu.Unlock()
	if pendingText == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_turn_input", "", "No pending audio or text input is available for response generation.")
	}
	if err := s.ensureStarted(); err != nil {
		return err
	}
	if response != nil && (strings.TrimSpace(response.Voice) != "" || strings.TrimSpace(response.Instructions) != "") {
		update := &modality.AudioSessionConfig{
			Voice:        response.Voice,
			Instructions: response.Instructions,
		}
		if err := s.updateSession(update); err != nil {
			return err
		}
	}
	s.mu.Lock()
	payload := realtimeTextQueryRequest{Content: s.pendingText}
	s.pendingText = ""
	s.mu.Unlock()
	_, err := s.writeJSONEvent(dialogMessageTypeFullClient, dialogEventChatTextQuery, payload)
	return err
}

func (s *realtimeAudioSession) cancelResponse() error {
	s.mu.Lock()
	connected := s.conn != nil && s.started
	s.mu.Unlock()
	if !connected {
		return nil
	}
	_, err := s.writeJSONEvent(dialogMessageTypeFullClient, dialogEventClientInterrupt, map[string]any{})
	return err
}

func (s *realtimeAudioSession) ensureStarted() error {
	s.startOnce.Do(func() {
		s.startErr = s.start()
		if s.startErr != nil {
			s.startReady <- s.startErr
		}
	})
	s.mu.Lock()
	started := s.started
	startErr := s.startErr
	s.mu.Unlock()
	if started {
		return nil
	}
	if startErr != nil {
		return startErr
	}
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	case <-time.After(realtimeReadyTimeout):
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_timeout", "", "ByteDance realtime audio session did not start in time.")
	case err := <-s.startReady:
		return err
	}
}

func (s *realtimeAudioSession) start() error {
	wsURL := strings.TrimSpace(s.adapter.url)
	if wsURL == "" {
		wsURL = defaultRealtimeDialogueURL
	}
	if _, err := url.Parse(wsURL); err != nil {
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_misconfigured", "", "ByteDance realtime audio URL is invalid.")
	}
	headers := http.Header{}
	headers.Set("X-Api-Resource-Id", s.adapter.resourceID)
	headers.Set("X-Api-App-Key", s.adapter.appKey)
	headers.Set("X-Api-Connect-Id", s.connectID)
	switch s.adapter.authMode {
	case realtimeAuthAPIKey:
		headers.Set("X-Api-Key", s.adapter.client.speechAPIKey)
	default:
		headers.Set("X-Api-App-ID", s.adapter.client.appID)
		headers.Set("X-Api-Access-Key", s.adapter.client.speechToken)
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 30 * time.Second,
	}
	conn, resp, err := dialer.DialContext(s.ctx, wsURL, headers)
	if err != nil {
		if resp != nil && resp.StatusCode > 0 {
			return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_transport_error", "", fmt.Sprintf("ByteDance realtime audio handshake failed with status %d.", resp.StatusCode))
		}
		return translateTransportError(err, "ByteDance")
	}
	s.mu.Lock()
	s.conn = conn
	s.mu.Unlock()

	go s.readLoop(conn)

	if _, err := s.writeJSONEvent(dialogMessageTypeFullClient, dialogEventStartConnection, map[string]any{}); err != nil {
		return err
	}
	if _, err := s.writeJSONEvent(dialogMessageTypeFullClient, dialogEventStartSession, s.startSessionPayload()); err != nil {
		return err
	}
	return nil
}

func normalizeRealtimeAuthMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case "", realtimeAuthAccessToken:
		return realtimeAuthAccessToken
	case realtimeAuthAPIKey:
		return realtimeAuthAPIKey
	default:
		return strings.TrimSpace(mode)
	}
}

func (s *realtimeAudioSession) startSessionPayload() realtimeStartSessionRequest {
	payload := realtimeStartSessionRequest{
		TTS: realtimeStartSessionTTS{
			Speaker: s.cfg.Voice,
			AudioConfig: realtimeStartSessionAudioConfig{
				Channel:    1,
				Format:     "pcm_s16le",
				SampleRate: 24000,
			},
		},
		ASR: realtimeStartSessionASR{
			AudioInfo: realtimeStartSessionASRAudioInfo{
				Format:     "pcm",
				SampleRate: 16000,
				Channel:    1,
			},
		},
		Dialog: realtimeStartSessionDialog{
			BotName:    realtimeBotName,
			SystemRole: strings.TrimSpace(s.cfg.Instructions),
			DialogID:   "",
			Extra: realtimeStartSessionExtra{
				InputMod: realtimeInputMode(s.cfg.TurnDetection),
				Model:    s.adapter.providerModel,
			},
		},
	}
	return payload
}

func (s *realtimeAudioSession) readLoop(conn *websocket.Conn) {
	defer func() {
		_ = s.Close()
	}()
	for {
		select {
		case <-s.closeCh:
			return
		default:
		}

		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			select {
			case <-s.closeCh:
				return
			default:
			}
			s.emitError(httputil.NewError(http.StatusBadGateway, "provider_error", "provider_transport_error", "", "ByteDance realtime audio connection closed unexpectedly."))
			s.signalStart(httputil.NewError(http.StatusBadGateway, "provider_error", "provider_transport_error", "", "ByteDance realtime audio connection closed unexpectedly."))
			return
		}
		if messageType != websocket.BinaryMessage {
			continue
		}
		frame, err := decodeDialogFrame(payload)
		if err != nil {
			s.emitError(httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance returned an invalid realtime audio frame."))
			s.signalStart(httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance returned an invalid realtime audio frame."))
			return
		}
		if err := s.handleFrame(frame); err != nil {
			s.emitError(err)
			s.signalStart(err)
			return
		}
	}
}

func (s *realtimeAudioSession) handleFrame(frame dialogFrame) error {
	if frame.MessageType == dialogMessageTypeError {
		return s.dialogueErrorFromFrame(frame)
	}

	var eventID uint32
	if frame.Event != nil {
		eventID = *frame.Event
	}

	switch frame.MessageType {
	case dialogMessageTypeFullServer:
		return s.handleJSONEvent(eventID, frame.Payload)
	case dialogMessageTypeAudioServer:
		if eventID != dialogEventTTSResponse {
			return nil
		}
		return s.handleTTSAudio(frame.Payload)
	default:
		return nil
	}
}

func (s *realtimeAudioSession) handleJSONEvent(eventID uint32, payload []byte) error {
	switch eventID {
	case dialogEventConnectionStarted:
		return nil
	case dialogEventConnectionFailed:
		return s.parseRealtimeError(payload, "ByteDance realtime audio connection failed.")
	case dialogEventConnectionClosed:
		return nil
	case dialogEventSessionStarted:
		var started realtimeSessionStartedPayload
		if len(payload) > 0 {
			if err := json.Unmarshal(payload, &started); err != nil {
				return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance returned an invalid SessionStarted payload.")
			}
		}
		s.mu.Lock()
		s.started = true
		s.dialogID = started.DialogID
		s.mu.Unlock()
		s.signalStart(nil)
		return nil
	case dialogEventSessionFinished:
		return nil
	case dialogEventSessionFailed:
		return s.parseRealtimeError(payload, "ByteDance realtime audio session failed.")
	case dialogEventUsageResponse:
		return s.handleUsage(payload)
	case dialogEventConfigUpdated:
		s.emit(modality.AudioServerEvent{Type: modality.AudioServerEventSessionUpdated, EventID: s.nextEventID("evt")})
		return nil
	case dialogEventTTSSentenceStart:
		return s.handleTTSStart(payload)
	case dialogEventTTSSentenceEnd:
		return nil
	case dialogEventTTSEnded:
		return s.handleTTSEnded(payload)
	case dialogEventASRInfo:
		return nil
	case dialogEventASRResponse:
		return s.handleASRResponse(payload)
	case dialogEventASREnded:
		return s.handleASREnded()
	case dialogEventChatResponse:
		return s.handleChatResponse(payload)
	case dialogEventChatConfirmed:
		return nil
	case dialogEventChatEnded:
		return s.handleChatEnded(payload)
	case dialogEventDialogError:
		return s.parseRealtimeError(payload, "ByteDance realtime audio returned an error.")
	default:
		return nil
	}
}

func (s *realtimeAudioSession) handleTTSStart(payload []byte) error {
	var data realtimeTTSStartPayload
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &data); err != nil {
			return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance returned an invalid TTSSentenceStart payload.")
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	turn := s.ensureCurrentTurnLocked(data.ReplyID, data.QuestionID)
	if data.Text != "" && turn.transcript.Len() == 0 {
		turn.transcript.WriteString(data.Text)
	}
	return nil
}

func (s *realtimeAudioSession) handleASRResponse(payload []byte) error {
	var data realtimeASRPayload
	if err := json.Unmarshal(payload, &data); err != nil {
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance returned an invalid ASR payload.")
	}
	var latest string
	for _, result := range data.Results {
		if text := strings.TrimSpace(result.Text); text != "" {
			latest = text
		}
	}
	if latest == "" {
		return nil
	}
	s.mu.Lock()
	s.pendingText = latest
	s.mu.Unlock()
	return nil
}

func (s *realtimeAudioSession) handleASREnded() error {
	s.mu.Lock()
	transcript := strings.TrimSpace(s.pendingText)
	audioBytes := s.pendingAudio
	s.pendingAudio = 0
	s.mu.Unlock()

	if audioBytes > 0 {
		s.addInputAudioSeconds(float64(audioBytes) / 2 / 16000)
	}
	s.emit(modality.AudioServerEvent{
		Type:       modality.AudioServerEventInputAudioCommitted,
		EventID:    s.nextEventID("evt"),
		Transcript: transcript,
	})
	return nil
}

func (s *realtimeAudioSession) handleChatResponse(payload []byte) error {
	var data realtimeChatPayload
	if err := json.Unmarshal(payload, &data); err != nil {
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance returned an invalid chat response payload.")
	}
	content := data.Content
	if content == "" {
		return nil
	}

	s.mu.Lock()
	turn := s.ensureCurrentTurnLocked(data.ReplyID, data.QuestionID)
	turn.text.WriteString(content)
	turn.transcript.WriteString(content)
	responseID := turn.responseID
	s.mu.Unlock()

	s.emit(modality.AudioServerEvent{
		Type:       modality.AudioServerEventResponseTextDelta,
		EventID:    s.nextEventID("evt"),
		ResponseID: responseID,
		Text:       content,
	})
	s.emit(modality.AudioServerEvent{
		Type:       modality.AudioServerEventResponseTranscriptDelta,
		EventID:    s.nextEventID("evt"),
		ResponseID: responseID,
		Transcript: content,
	})
	return nil
}

func (s *realtimeAudioSession) handleChatEnded(payload []byte) error {
	var data realtimeEndedPayload
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &data); err != nil {
			return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance returned an invalid ChatEnded payload.")
		}
	}
	s.mu.Lock()
	turn := s.ensureCurrentTurnLocked(data.ReplyID, data.QuestionID)
	turn.textDone = true
	text := turn.text.String()
	transcript := turn.transcript.String()
	responseID := turn.responseID
	s.mu.Unlock()

	s.emit(modality.AudioServerEvent{
		Type:       modality.AudioServerEventResponseTextDone,
		EventID:    s.nextEventID("evt"),
		ResponseID: responseID,
		Text:       text,
	})
	s.emit(modality.AudioServerEvent{
		Type:       modality.AudioServerEventResponseTranscriptDone,
		EventID:    s.nextEventID("evt"),
		ResponseID: responseID,
		Transcript: transcript,
	})
	return nil
}

func (s *realtimeAudioSession) handleTTSAudio(payload []byte) error {
	audio := resamplePCM16Mono(payload, 24000, 16000)
	if len(audio) == 0 {
		return nil
	}
	s.mu.Lock()
	turn := s.ensureCurrentTurnLocked("", "")
	turn.audio = append(turn.audio, audio...)
	responseID := turn.responseID
	s.mu.Unlock()
	s.emit(modality.AudioServerEvent{
		Type:       modality.AudioServerEventResponseAudioDelta,
		EventID:    s.nextEventID("evt"),
		ResponseID: responseID,
		Audio:      base64.StdEncoding.EncodeToString(audio),
	})
	return nil
}

func (s *realtimeAudioSession) handleTTSEnded(payload []byte) error {
	var data realtimeEndedPayload
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &data); err != nil {
			return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance returned an invalid TTSEnded payload.")
		}
	}

	s.mu.Lock()
	turn := s.ensureCurrentTurnLocked(data.ReplyID, data.QuestionID)
	turn.audioDone = true
	audio := append([]byte(nil), turn.audio...)
	responseID := turn.responseID
	audioSeconds := float64(len(audio)) / 2 / 16000
	hasProviderUsage := turnHasProviderUsage(turn)
	if turn.completionTimer == nil && !hasProviderUsage {
		timer := time.AfterFunc(realtimeUsageGracePeriod, func() {
			s.completeTurn(responseID)
		})
		turn.completionTimer = timer
	}
	s.mu.Unlock()

	s.addOutputAudioSeconds(audioSeconds)
	s.emit(modality.AudioServerEvent{
		Type:       modality.AudioServerEventResponseAudioDone,
		EventID:    s.nextEventID("evt"),
		ResponseID: responseID,
		Audio:      base64.StdEncoding.EncodeToString(audio),
	})
	if hasProviderUsage {
		s.completeTurn(responseID)
	}
	return nil
}

func (s *realtimeAudioSession) handleUsage(payload []byte) error {
	var data realtimeUsagePayload
	if err := json.Unmarshal(payload, &data); err != nil {
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance returned an invalid usage payload.")
	}
	usage := modality.AudioUsage{
		InputTextTokens:  data.Usage.InputTextTokens + data.Usage.CachedTextTokens,
		OutputTextTokens: data.Usage.OutputTextTokens,
		TotalTokens:      data.Usage.InputTextTokens + data.Usage.CachedTextTokens + data.Usage.OutputTextTokens,
		Source:           modality.TokenCountSourceProviderReported,
	}
	s.mu.Lock()
	turn := s.ensureCurrentTurnLocked("", "")
	turn.usage = &usage
	responseID := turn.responseID
	audioDone := turn.audioDone
	if turn.completionTimer != nil {
		turn.completionTimer.Stop()
		turn.completionTimer = nil
	}
	s.mu.Unlock()
	if audioDone {
		s.completeTurn(responseID)
	}
	return nil
}

func (s *realtimeAudioSession) completeTurn(responseID string) {
	s.mu.Lock()
	turn := s.currentTurn
	if turn == nil || turn.responseID != responseID || turn.completed {
		s.mu.Unlock()
		return
	}
	turn.completed = true
	if turn.completionTimer != nil {
		turn.completionTimer.Stop()
		turn.completionTimer = nil
	}
	usage := turn.usage
	if usage == nil {
		audioSeconds := float64(len(turn.audio)) / 2 / 16000
		usage = &modality.AudioUsage{
			OutputAudioSeconds: audioSeconds,
			Source:             modality.TokenCountSourceUnavailable,
		}
	} else {
		usage = &modality.AudioUsage{
			InputAudioSeconds:  usage.InputAudioSeconds,
			OutputAudioSeconds: usage.OutputAudioSeconds,
			InputTextTokens:    usage.InputTextTokens,
			OutputTextTokens:   usage.OutputTextTokens,
			TotalTokens:        usage.TotalTokens,
			Source:             usage.Source,
		}
		audioSeconds := float64(len(turn.audio)) / 2 / 16000
		if usage.OutputAudioSeconds == 0 {
			usage.OutputAudioSeconds = audioSeconds
		}
	}
	text := strings.TrimSpace(turn.text.String())
	s.currentTurn = nil
	s.mu.Unlock()

	s.emit(modality.AudioServerEvent{
		Type:       modality.AudioServerEventResponseCompleted,
		EventID:    s.nextEventID("evt"),
		ResponseID: responseID,
		Text:       text,
		Usage:      usage,
	})
}

func (s *realtimeAudioSession) writeJSONEvent(messageType byte, eventID uint32, payload any) ([]byte, error) {
	frame, err := encodeDialogJSONFrame(messageType, eventID, sessionIDForEvent(eventID, s.sessionID), "", payload)
	if err != nil {
		return nil, err
	}
	if err := s.writeFrame(frame); err != nil {
		return nil, err
	}
	return frame, nil
}

func (s *realtimeAudioSession) writeAudioEvent(eventID uint32, audio []byte) ([]byte, error) {
	frame, err := encodeDialogAudioFrame(eventID, s.sessionID, audio)
	if err != nil {
		return nil, err
	}
	if err := s.writeFrame(frame); err != nil {
		return nil, err
	}
	return frame, nil
}

func (s *realtimeAudioSession) writeFrame(frame []byte) error {
	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()
	if conn == nil {
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_transport_error", "", "ByteDance realtime audio connection is not available.")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		return translateTransportError(err, "ByteDance")
	}
	return nil
}

func (s *realtimeAudioSession) signalStart(err error) {
	if err != nil {
		s.mu.Lock()
		s.startErr = err
		s.mu.Unlock()
	}
	select {
	case s.startReady <- err:
	default:
	}
}

func (s *realtimeAudioSession) emit(event modality.AudioServerEvent) {
	select {
	case <-s.closeCh:
		return
	case s.events <- event:
	}
}

func (s *realtimeAudioSession) emitError(err error) {
	apiErr, ok := err.(*httputil.APIError)
	if !ok {
		apiErr = httputil.NewError(http.StatusBadGateway, "provider_error", "provider_error", "", err.Error())
	}
	s.emit(modality.AudioServerEvent{
		Type:    modality.AudioServerEventError,
		EventID: s.nextEventID("evt"),
		Error: &modality.AudioError{
			Type:    apiErr.Type,
			Code:    apiErr.Code,
			Message: apiErr.Message,
			Param:   apiErr.Param,
		},
	})
}

func (s *realtimeAudioSession) parseRealtimeError(payload []byte, fallback string) error {
	var data realtimeCommonErrorPayload
	if len(payload) > 0 {
		_ = json.Unmarshal(payload, &data)
	}
	message := strings.TrimSpace(data.Message)
	if message == "" {
		message = strings.TrimSpace(data.Error)
	}
	if message == "" {
		message = fallback
	}
	code := strings.TrimSpace(data.StatusCode)
	if code == "" {
		code = "provider_realtime_error"
	}
	return httputil.NewError(http.StatusBadGateway, "provider_error", code, "", message)
}

func (s *realtimeAudioSession) dialogueErrorFromFrame(frame dialogFrame) error {
	var data realtimeCommonErrorPayload
	if len(frame.Payload) > 0 {
		_ = json.Unmarshal(frame.Payload, &data)
	}
	message := strings.TrimSpace(data.Message)
	if message == "" {
		message = strings.TrimSpace(data.Error)
	}
	if message == "" {
		message = "ByteDance realtime audio returned an error."
	}
	code := "provider_realtime_error"
	if frame.Code != nil && *frame.Code != 0 {
		code = fmt.Sprintf("provider_realtime_error_%d", *frame.Code)
	}
	return httputil.NewError(http.StatusBadGateway, "provider_error", code, "", message)
}

func (s *realtimeAudioSession) ensureCurrentTurnLocked(replyID string, questionID string) *realtimeTurnState {
	if s.currentTurn == nil {
		s.currentTurn = &realtimeTurnState{
			responseID: replyID,
			replyID:    replyID,
			questionID: questionID,
		}
		if s.currentTurn.responseID == "" {
			s.currentTurn.responseID = s.nextEventID("resp")
		}
	}
	if replyID != "" {
		s.currentTurn.replyID = replyID
		s.currentTurn.responseID = replyID
	}
	if questionID != "" {
		s.currentTurn.questionID = questionID
	}
	if s.currentTurn.responseID == "" {
		s.currentTurn.responseID = s.nextEventID("resp")
	}
	return s.currentTurn
}

func (s *realtimeAudioSession) addInputAudioSeconds(seconds float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	turn := s.ensureCurrentTurnLocked("", "")
	if turn.usage == nil {
		turn.usage = &modality.AudioUsage{Source: modality.TokenCountSourceUnavailable}
	}
	turn.usage.InputAudioSeconds += seconds
}

func (s *realtimeAudioSession) addOutputAudioSeconds(seconds float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	turn := s.ensureCurrentTurnLocked("", "")
	if turn.usage == nil {
		turn.usage = &modality.AudioUsage{Source: modality.TokenCountSourceUnavailable}
	}
	turn.usage.OutputAudioSeconds += seconds
}

func (s *realtimeAudioSession) nextEventID(prefix string) string {
	value := atomic.AddUint64(&s.sequence, 1)
	return fmt.Sprintf("%s_%06d", prefix, value)
}

func turnHasProviderUsage(turn *realtimeTurnState) bool {
	if turn == nil || turn.usage == nil {
		return false
	}
	if turn.usage.Source == modality.TokenCountSourceProviderReported {
		return true
	}
	return turn.usage.InputTextTokens > 0 || turn.usage.OutputTextTokens > 0 || turn.usage.TotalTokens > 0
}

func realtimeInputMode(turn *modality.TurnDetectionConfig) string {
	if turn == nil {
		return "push_to_talk"
	}
	switch strings.TrimSpace(turn.Mode) {
	case modality.TurnDetectionServerVAD:
		return "keep_alive"
	default:
		return "push_to_talk"
	}
}

func sessionIDForEvent(eventID uint32, sessionID string) string {
	switch eventID {
	case dialogEventStartConnection, dialogEventFinishConnection:
		return ""
	default:
		return sessionID
	}
}

func newRealtimeSessionID() string {
	raw := newRequestID()
	if len(raw) < 32 {
		return raw
	}
	return fmt.Sprintf("%s-%s-%s-%s-%s", raw[0:8], raw[8:12], raw[12:16], raw[16:20], raw[20:32])
}

func resamplePCM16Mono(input []byte, fromRate int, toRate int) []byte {
	if fromRate <= 0 || toRate <= 0 || len(input) < 2 || fromRate == toRate {
		return append([]byte(nil), input...)
	}
	sampleCount := len(input) / 2
	if sampleCount == 0 {
		return nil
	}
	outputCount := int(math.Round(float64(sampleCount) * float64(toRate) / float64(fromRate)))
	if outputCount <= 0 {
		return nil
	}
	output := make([]byte, outputCount*2)
	lastIndex := sampleCount - 1
	for i := 0; i < outputCount; i++ {
		position := float64(i) * float64(fromRate) / float64(toRate)
		left := int(position)
		if left >= lastIndex {
			copy(output[i*2:], input[lastIndex*2:lastIndex*2+2])
			continue
		}
		right := left + 1
		fraction := position - float64(left)
		leftSample := float64(int16(uint16(input[left*2]) | uint16(input[left*2+1])<<8))
		rightSample := float64(int16(uint16(input[right*2]) | uint16(input[right*2+1])<<8))
		value := int16(math.Round(leftSample + (rightSample-leftSample)*fraction))
		output[i*2] = byte(value)
		output[i*2+1] = byte(uint16(value) >> 8)
	}
	return output
}
