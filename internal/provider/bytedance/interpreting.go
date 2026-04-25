package bytedance

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	eventpb "github.com/JiaCheng2004/Polaris/internal/provider/bytedance/astpb/common/event"
	rpcmetapb "github.com/JiaCheng2004/Polaris/internal/provider/bytedance/astpb/common/rpcmeta"
	astpb "github.com/JiaCheng2004/Polaris/internal/provider/bytedance/astpb/products/understanding/ast"
	basepb "github.com/JiaCheng2004/Polaris/internal/provider/bytedance/astpb/products/understanding/base"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

const (
	defaultInterpretingURL        = "wss://openspeech.bytedance.com/api/v4/ast/v2/translate"
	defaultInterpretingResourceID = "volc.service_type.10053"
	interpretingReadyTimeout      = 10 * time.Second
	interpretingSuccessCode       = 20000000
)

type interpretingAdapter struct {
	client         *Client
	model          string
	url            string
	resourceID     string
	defaultSession time.Duration
}

type interpretingSession struct {
	ctx        context.Context
	adapter    *interpretingAdapter
	cfg        modality.InterpretingSessionConfig
	events     chan modality.InterpretingServerEvent
	closeCh    chan struct{}
	startReady chan error

	writeMu sync.Mutex
	mu      sync.Mutex

	conn       *websocket.Conn
	started    bool
	closed     bool
	committed  bool
	audioBytes int
	startOnce  sync.Once
	startErr   error
	closeOnce  sync.Once
	counter    uint64
	sessionID  string
	connectID  string
	sourceSeq  int32
	targetSeq  int32
	pendingUse modality.InterpretingUsage
}

func NewInterpretingAdapter(client *Client, model string, endpoint string) modality.InterpretingAdapter {
	return &interpretingAdapter{
		client:         client,
		model:          model,
		url:            strings.TrimSpace(endpoint),
		resourceID:     defaultInterpretingResourceID,
		defaultSession: 10 * time.Minute,
	}
}

func (a *interpretingAdapter) ConnectInterpreting(ctx context.Context, cfg *modality.InterpretingSessionConfig) (modality.InterpretingSession, error) {
	if a == nil || a.client == nil {
		return nil, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "adapter_unavailable", "", "Interpreting adapter is unavailable.")
	}
	if strings.TrimSpace(a.client.appID) == "" || strings.TrimSpace(a.client.speechToken) == "" {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_misconfigured", "", "ByteDance simultaneous interpretation requires providers.bytedance.app_id and providers.bytedance.speech_access_token.")
	}
	normalized, err := a.normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &interpretingSession{
		ctx:        ctx,
		adapter:    a,
		cfg:        *normalized,
		events:     make(chan modality.InterpretingServerEvent, 64),
		closeCh:    make(chan struct{}),
		startReady: make(chan error, 1),
		sessionID:  newRealtimeSessionID(),
		connectID:  newRealtimeSessionID(),
	}, nil
}

func (a *interpretingAdapter) normalizeConfig(cfg *modality.InterpretingSessionConfig) (*modality.InterpretingSessionConfig, error) {
	if cfg == nil {
		cfg = &modality.InterpretingSessionConfig{}
	}
	normalized := *cfg
	if strings.TrimSpace(normalized.Model) == "" {
		normalized.Model = a.model
	}
	switch strings.TrimSpace(normalized.Mode) {
	case "", modality.InterpretingModeSpeechToSpeech:
		normalized.Mode = modality.InterpretingModeSpeechToSpeech
	case modality.InterpretingModeSpeechToText:
	default:
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_mode", "mode", "Supported interpreting modes are speech_to_speech and speech_to_text.")
	}
	if strings.TrimSpace(normalized.SourceLanguage) == "" {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_source_language", "source_language", "Field 'source_language' is required.")
	}
	if strings.TrimSpace(normalized.TargetLanguage) == "" {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_target_language", "target_language", "Field 'target_language' is required.")
	}
	if strings.TrimSpace(normalized.InputAudioFormat) == "" {
		normalized.InputAudioFormat = modality.InterpretingAudioFormatPCM16
	}
	switch normalized.InputAudioFormat {
	case modality.InterpretingAudioFormatPCM16, modality.InterpretingAudioFormatWAV:
	default:
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_input_audio_format", "input_audio_format", "Supported input audio formats are pcm16 and wav.")
	}
	if normalized.InputSampleRateHz == 0 {
		normalized.InputSampleRateHz = 16000
	}
	if normalized.InputSampleRateHz != 16000 {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_input_sample_rate", "input_audio_format", "ByteDance simultaneous interpretation input must be 16000 Hz.")
	}
	if normalized.Mode == modality.InterpretingModeSpeechToSpeech {
		if strings.TrimSpace(normalized.OutputAudioFormat) == "" {
			normalized.OutputAudioFormat = modality.InterpretingAudioFormatOpus
		}
		switch normalized.OutputAudioFormat {
		case modality.InterpretingAudioFormatPCM16, modality.InterpretingAudioFormatOpus:
		default:
			return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_output_audio_format", "output_audio_format", "Supported output audio formats are pcm16 and ogg_opus.")
		}
		if normalized.OutputSampleRateHz == 0 {
			if normalized.OutputAudioFormat == modality.InterpretingAudioFormatOpus {
				normalized.OutputSampleRateHz = 48000
			} else {
				normalized.OutputSampleRateHz = 16000
			}
		}
		if normalized.OutputAudioFormat == modality.InterpretingAudioFormatPCM16 && normalized.OutputSampleRateHz != 16000 {
			return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_output_sample_rate", "output_sample_rate_hz", "PCM16 interpreting output only supports 16000 Hz.")
		}
		if normalized.OutputAudioFormat == modality.InterpretingAudioFormatOpus && normalized.OutputSampleRateHz != 48000 {
			return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_output_sample_rate", "output_sample_rate_hz", "Ogg Opus interpreting output only supports 48000 Hz.")
		}
	} else {
		normalized.OutputAudioFormat = ""
		normalized.OutputSampleRateHz = 0
		normalized.Voice = ""
	}
	return &normalized, nil
}

func (a *interpretingAdapter) streamURL() string {
	if a.url != "" {
		return a.url
	}
	return defaultInterpretingURL
}

func (s *interpretingSession) Send(event modality.InterpretingClientEvent) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return httputil.NewError(http.StatusGone, "invalid_request_error", "session_closed", "", "Interpreting session is closed.")
	}
	if s.committed && event.Type != modality.InterpretingClientEventSessionClose {
		s.mu.Unlock()
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "session_committed", "", "Interpreting session has already been committed.")
	}
	s.mu.Unlock()

	switch event.Type {
	case modality.InterpretingClientEventSessionUpdate:
		return s.updateSession(event.Session)
	case modality.InterpretingClientEventInputAudioAppend:
		return s.appendAudio(event.Audio)
	case modality.InterpretingClientEventInputAudioCommit:
		return s.commitAudio()
	case modality.InterpretingClientEventSessionClose:
		return s.Close()
	default:
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unknown_event_type", "type", "Unknown interpreting client event type.")
	}
}

func (s *interpretingSession) Events() <-chan modality.InterpretingServerEvent {
	return s.events
}

func (s *interpretingSession) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		conn := s.conn
		s.mu.Unlock()

		close(s.closeCh)
		if conn != nil {
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
			_ = conn.Close()
		}
	})
	return nil
}

func (s *interpretingSession) updateSession(update *modality.InterpretingSessionConfig) error {
	if update == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "session_config_immutable", "session", "Interpreting session configuration cannot be changed after the provider session starts.")
	}
	if strings.TrimSpace(update.Mode) != "" {
		s.cfg.Mode = strings.TrimSpace(update.Mode)
	}
	if strings.TrimSpace(update.SourceLanguage) != "" {
		s.cfg.SourceLanguage = strings.TrimSpace(update.SourceLanguage)
	}
	if strings.TrimSpace(update.TargetLanguage) != "" {
		s.cfg.TargetLanguage = strings.TrimSpace(update.TargetLanguage)
	}
	if strings.TrimSpace(update.Voice) != "" {
		s.cfg.Voice = strings.TrimSpace(update.Voice)
	}
	if strings.TrimSpace(update.InputAudioFormat) != "" {
		s.cfg.InputAudioFormat = strings.TrimSpace(update.InputAudioFormat)
	}
	if strings.TrimSpace(update.OutputAudioFormat) != "" {
		s.cfg.OutputAudioFormat = strings.TrimSpace(update.OutputAudioFormat)
	}
	if update.InputSampleRateHz > 0 {
		s.cfg.InputSampleRateHz = update.InputSampleRateHz
	}
	if update.OutputSampleRateHz > 0 {
		s.cfg.OutputSampleRateHz = update.OutputSampleRateHz
	}
	if update.Denoise != nil {
		value := *update.Denoise
		s.cfg.Denoise = &value
	}
	if update.Glossary != nil {
		s.cfg.Glossary = append([]modality.GlossaryEntry(nil), update.Glossary...)
	}
	s.emit(modality.InterpretingServerEvent{
		Type:    modality.InterpretingServerEventSessionUpdated,
		EventID: s.nextEventID(),
	})
	return nil
}

func (s *interpretingSession) appendAudio(encoded string) error {
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
	request := &astpb.TranslateRequest{
		RequestMeta: &rpcmetapb.RequestMeta{
			SessionID: s.sessionID,
			Sequence:  s.nextSequence(),
		},
		Event: eventpb.Type_TaskRequest,
		SourceAudio: &basepb.Audio{
			BinaryData: payload,
		},
	}
	if err := s.writeProto(request); err != nil {
		return err
	}
	s.mu.Lock()
	s.audioBytes += len(payload)
	s.mu.Unlock()
	return nil
}

func (s *interpretingSession) commitAudio() error {
	s.mu.Lock()
	pending := s.audioBytes
	if s.committed {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()
	if pending == 0 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_audio", "audio", "No buffered audio is available to commit.")
	}
	if err := s.ensureStarted(); err != nil {
		return err
	}
	request := &astpb.TranslateRequest{
		RequestMeta: &rpcmetapb.RequestMeta{
			SessionID: s.sessionID,
			Sequence:  s.nextSequence(),
		},
		Event: eventpb.Type_FinishSession,
	}
	if err := s.writeProto(request); err != nil {
		return err
	}
	s.mu.Lock()
	s.committed = true
	s.audioBytes = 0
	s.mu.Unlock()
	s.emit(modality.InterpretingServerEvent{
		Type:    modality.InterpretingServerEventInputAudioCommitted,
		EventID: s.nextEventID(),
	})
	return nil
}

func (s *interpretingSession) ensureStarted() error {
	s.startOnce.Do(func() {
		s.startErr = s.start()
	})
	return s.startErr
}

func (s *interpretingSession) start() error {
	headers := http.Header{}
	headers.Set("X-Api-App-Key", s.adapter.client.appID)
	headers.Set("X-Api-Access-Key", s.adapter.client.speechToken)
	headers.Set("X-Api-Resource-Id", s.adapter.resourceID)
	headers.Set("X-Api-Connect-Id", s.connectID)

	target, err := url.Parse(s.adapter.streamURL())
	if err != nil {
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_misconfigured", "", "ByteDance interpreting websocket URL is invalid.")
	}

	conn, _, err := websocket.DefaultDialer.DialContext(s.ctx, target.String(), headers)
	if err != nil {
		return httputil.ProviderTransportError(err, "ByteDance")
	}
	s.mu.Lock()
	s.conn = conn
	s.mu.Unlock()

	go s.readLoop()

	startRequest := &astpb.TranslateRequest{
		RequestMeta: &rpcmetapb.RequestMeta{
			SessionID: s.sessionID,
			Sequence:  s.nextSequence(),
		},
		Event: eventpb.Type_StartSession,
		User: &basepb.User{
			Uid: "polaris",
			Did: "polaris",
		},
		SourceAudio: &basepb.Audio{
			Format:  providerInterpretingInputFormat(s.cfg.InputAudioFormat),
			Codec:   "raw",
			Rate:    int32(s.cfg.InputSampleRateHz),
			Bits:    16,
			Channel: 1,
		},
		Request: &astpb.ReqParams{
			Mode:           providerInterpretingMode(s.cfg.Mode),
			SourceLanguage: s.cfg.SourceLanguage,
			TargetLanguage: s.cfg.TargetLanguage,
		},
	}
	if s.cfg.Denoise != nil {
		startRequest.Denoise = proto.Bool(*s.cfg.Denoise)
	}
	if len(s.cfg.Glossary) > 0 {
		startRequest.Request.Corpus = &basepb.Corpus{
			GlossaryList: providerGlossary(s.cfg.Glossary),
		}
	}
	if s.cfg.Mode == modality.InterpretingModeSpeechToSpeech {
		startRequest.TargetAudio = &basepb.Audio{
			Format:  providerInterpretingOutputFormat(s.cfg.OutputAudioFormat),
			Rate:    int32(s.cfg.OutputSampleRateHz),
			Bits:    providerInterpretingOutputBits(s.cfg.OutputAudioFormat),
			Channel: 1,
		}
		if strings.TrimSpace(s.cfg.Voice) != "" {
			startRequest.Request.SpeakerId = strings.TrimSpace(s.cfg.Voice)
		}
	}

	if err := s.writeProto(startRequest); err != nil {
		return err
	}

	select {
	case err := <-s.startReady:
		return err
	case <-time.After(interpretingReadyTimeout):
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_timeout", "", "ByteDance simultaneous interpretation session did not start in time.")
	case <-s.ctx.Done():
		return httputil.ProviderTransportError(s.ctx.Err(), "ByteDance")
	}
}

func (s *interpretingSession) readLoop() {
	defer func() {
		_ = s.Close()
	}()

	for {
		conn := s.currentConn()
		if conn == nil {
			return
		}
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			select {
			case <-s.closeCh:
				return
			default:
			}
			s.failStart(httputil.ProviderTransportError(err, "ByteDance"))
			s.emitError("provider_transport_error", "provider_transport_error", "ByteDance interpreting websocket connection failed.")
			return
		}
		if messageType != websocket.BinaryMessage {
			continue
		}
		response := &astpb.TranslateResponse{}
		if err := proto.Unmarshal(payload, response); err != nil {
			s.failStart(httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance interpreting websocket returned an invalid protobuf frame."))
			s.emitError("provider_invalid_response", "provider_invalid_response", "ByteDance interpreting websocket returned an invalid protobuf frame.")
			return
		}
		s.handleResponse(response)
	}
}

func (s *interpretingSession) handleResponse(response *astpb.TranslateResponse) {
	if response == nil {
		return
	}
	if meta := response.GetResponseMeta(); meta != nil && !interpretingStatusOK(meta.GetStatusCode()) {
		err := httputil.NewError(http.StatusBadGateway, "provider_error", "provider_error", "", firstNonEmptyString(strings.TrimSpace(meta.GetMessage()), "ByteDance simultaneous interpretation failed."))
		s.failStart(err)
		s.emitError("provider_error", "provider_error", firstNonEmptyString(meta.GetMessage(), "ByteDance simultaneous interpretation failed."))
		return
	}

	switch response.GetEvent() {
	case eventpb.Type_SessionStarted:
		s.mu.Lock()
		s.started = true
		s.mu.Unlock()
		s.resolveStart(nil)
		s.emit(modality.InterpretingServerEvent{
			Type:    modality.InterpretingServerEventSessionUpdated,
			EventID: s.nextEventID(),
		})
	case eventpb.Type_SourceSubtitleResponse:
		s.emit(modality.InterpretingServerEvent{
			Type:    modality.InterpretingServerEventSourceTranscriptDelta,
			EventID: s.nextEventID(),
			Text:    response.GetText(),
		})
	case eventpb.Type_SourceSubtitleEnd:
		s.emit(modality.InterpretingServerEvent{
			Type:    modality.InterpretingServerEventSourceTranscriptSegment,
			EventID: s.nextEventID(),
			Text:    response.GetText(),
			Segment: &modality.TranscriptSegment{
				ID:    int(atomic.AddInt32(&s.sourceSeq, 1)),
				Start: float64(response.GetStartTime()) / 1000,
				End:   float64(response.GetEndTime()) / 1000,
				Text:  response.GetText(),
			},
		})
	case eventpb.Type_TranslationSubtitleResponse:
		s.emit(modality.InterpretingServerEvent{
			Type:    modality.InterpretingServerEventTranslationDelta,
			EventID: s.nextEventID(),
			Text:    response.GetText(),
		})
	case eventpb.Type_TranslationSubtitleEnd:
		s.emit(modality.InterpretingServerEvent{
			Type:    modality.InterpretingServerEventTranslationSegment,
			EventID: s.nextEventID(),
			Text:    response.GetText(),
			Segment: &modality.TranscriptSegment{
				ID:    int(atomic.AddInt32(&s.targetSeq, 1)),
				Start: float64(response.GetStartTime()) / 1000,
				End:   float64(response.GetEndTime()) / 1000,
				Text:  response.GetText(),
			},
		})
	case eventpb.Type_TTSResponse:
		if len(response.GetData()) == 0 {
			return
		}
		s.emit(modality.InterpretingServerEvent{
			Type:    modality.InterpretingServerEventResponseAudioDelta,
			EventID: s.nextEventID(),
			Audio:   base64.StdEncoding.EncodeToString(response.GetData()),
		})
	case eventpb.Type_TTSSentenceEnd:
		s.emit(modality.InterpretingServerEvent{
			Type:    modality.InterpretingServerEventResponseAudioDone,
			EventID: s.nextEventID(),
		})
	case eventpb.Type_AudioMuted:
		s.emit(modality.InterpretingServerEvent{
			Type:    modality.InterpretingServerEventInputAudioMuted,
			EventID: s.nextEventID(),
		})
	case eventpb.Type_UsageResponse:
		s.pendingUse = interpretingUsageFromResponse(response)
	case eventpb.Type_SessionFinished:
		s.resolveStart(nil)
		s.emit(modality.InterpretingServerEvent{
			Type:    modality.InterpretingServerEventResponseCompleted,
			EventID: s.nextEventID(),
			Usage:   normalizeInterpretingUsage(s.pendingUse),
		})
	case eventpb.Type_SessionFailed:
		errMessage := "ByteDance simultaneous interpretation session failed."
		if meta := response.GetResponseMeta(); meta != nil && strings.TrimSpace(meta.GetMessage()) != "" {
			errMessage = strings.TrimSpace(meta.GetMessage())
		}
		s.failStart(httputil.NewError(http.StatusBadGateway, "provider_error", "provider_error", "", errMessage))
		s.emitError("provider_error", "provider_error", errMessage)
	}
}

func (s *interpretingSession) emit(event modality.InterpretingServerEvent) {
	select {
	case s.events <- event:
	case <-s.closeCh:
	}
}

func (s *interpretingSession) emitError(errType string, code string, message string) {
	s.emit(modality.InterpretingServerEvent{
		Type:    modality.InterpretingServerEventError,
		EventID: s.nextEventID(),
		Error: &modality.AudioError{
			Type:    errType,
			Code:    code,
			Message: message,
		},
	})
}

func (s *interpretingSession) writeProto(message proto.Message) error {
	payload, err := proto.Marshal(message)
	if err != nil {
		return httputil.NewError(http.StatusInternalServerError, "internal_error", "internal_error", "", "Failed to encode interpreting request.")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	conn := s.currentConn()
	if conn == nil {
		return httputil.NewError(http.StatusGone, "provider_error", "provider_transport_error", "", "Interpreting session connection is closed.")
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
		return httputil.ProviderTransportError(err, "ByteDance")
	}
	return nil
}

func (s *interpretingSession) currentConn() *websocket.Conn {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conn
}

func (s *interpretingSession) nextSequence() int32 {
	return int32(atomic.AddUint64(&s.counter, 1))
}

func (s *interpretingSession) nextEventID() string {
	return "evt_" + newRealtimeSessionID()
}

func (s *interpretingSession) resolveStart(err error) {
	select {
	case s.startReady <- err:
	default:
	}
}

func (s *interpretingSession) failStart(err error) {
	if err == nil {
		return
	}
	s.resolveStart(err)
}

func interpretingUsageFromResponse(response *astpb.TranslateResponse) modality.InterpretingUsage {
	usage := modality.InterpretingUsage{}
	if response == nil || response.GetResponseMeta() == nil || response.GetResponseMeta().GetBilling() == nil {
		return usage
	}
	billing := response.GetResponseMeta().GetBilling()
	if billing.GetDurationMsec() > 0 {
		usage.InputAudioSeconds = float64(billing.GetDurationMsec()) / 1000
	}
	var total float64
	for _, item := range billing.GetItems() {
		if item == nil {
			continue
		}
		switch strings.TrimSpace(item.GetUnit()) {
		case "input_audio_tokens", "output_text_tokens", "output_audio_tokens":
			total += float64(item.GetQuantity())
		}
	}
	if total > 0 {
		usage.TotalTokens = int(total + 0.5)
		usage.Source = modality.TokenCountSourceProviderReported
	} else if billing.GetWordCount() > 0 {
		usage.TotalTokens = int(billing.GetWordCount())
		usage.Source = modality.TokenCountSourceProviderReported
	}
	return usage
}

func normalizeInterpretingUsage(usage modality.InterpretingUsage) *modality.InterpretingUsage {
	if usage.Source == "" && usage.TotalTokens > 0 {
		usage.Source = modality.TokenCountSourceProviderReported
	}
	if usage.InputAudioSeconds == 0 && usage.OutputAudioSeconds == 0 && usage.TotalTokens == 0 && usage.Source == "" {
		return nil
	}
	return &usage
}

func providerInterpretingMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case modality.InterpretingModeSpeechToText:
		return "s2t"
	default:
		return "s2s"
	}
}

func providerInterpretingInputFormat(format string) string {
	return "wav"
}

func interpretingStatusOK(code int32) bool {
	return code == 0 || code == interpretingSuccessCode
}

func providerInterpretingOutputFormat(format string) string {
	switch strings.TrimSpace(format) {
	case modality.InterpretingAudioFormatPCM16:
		return "pcm"
	default:
		return "ogg_opus"
	}
}

func providerInterpretingOutputBits(format string) int32 {
	if strings.TrimSpace(format) == modality.InterpretingAudioFormatPCM16 {
		return 16
	}
	return 0
}

func providerGlossary(entries []modality.GlossaryEntry) map[string]string {
	items := make(map[string]string, len(entries))
	for _, entry := range entries {
		if strings.TrimSpace(entry.Source) == "" || strings.TrimSpace(entry.Target) == "" {
			continue
		}
		items[strings.TrimSpace(entry.Source)] = strings.TrimSpace(entry.Target)
	}
	return items
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
