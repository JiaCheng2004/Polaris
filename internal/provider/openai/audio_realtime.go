package openai

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/gorilla/websocket"
)

const (
	defaultRealtimeModel = "gpt-realtime"
	realtimeBetaHeader   = "realtime=v1"
)

type realtimeAudioAdapter struct {
	client         *Client
	model          string
	url            string
	providerModel  string
	defaultSession time.Duration
	turnModes      map[string]struct{}
}

type realtimeAudioSession struct {
	ctx          context.Context
	adapter      *realtimeAudioAdapter
	cfg          modality.AudioSessionConfig
	events       chan modality.AudioServerEvent
	closeOnce    sync.Once
	closeCh      chan struct{}
	startOnce    sync.Once
	startErr     error
	mu           sync.Mutex
	conn         *websocket.Conn
	started      bool
	closed       bool
	voiceLocked  bool
	pendingText  string
	pendingAudio int
	responseID   string
}

func newRealtimeAudioAdapter(client *Client, model string, modelCfg config.ModelConfig) modality.AudioAdapter {
	if strings.TrimSpace(modelCfg.RealtimeSession.Transport) != "openai_realtime" {
		return nil
	}
	providerModel := strings.TrimSpace(modelCfg.RealtimeSession.Model)
	if providerModel == "" {
		providerModel = defaultRealtimeModel
	}
	realtimeURL := strings.TrimSpace(modelCfg.RealtimeSession.URL)
	if realtimeURL == "" {
		realtimeURL = openAIRealtimeURL(client.baseURL)
	}
	defaultSession := modelCfg.SessionTTL
	if defaultSession <= 0 {
		defaultSession = 10 * time.Minute
	}
	return &realtimeAudioAdapter{
		client:         client,
		model:          model,
		url:            realtimeURL,
		providerModel:  providerModel,
		defaultSession: defaultSession,
		turnModes: map[string]struct{}{
			modality.TurnDetectionManual:    {},
			modality.TurnDetectionServerVAD: {},
		},
	}
}

func openAIRealtimeURL(baseURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Host == "" {
		return "wss://api.openai.com/v1/realtime"
	}
	switch parsed.Scheme {
	case "http":
		parsed.Scheme = "ws"
	default:
		parsed.Scheme = "wss"
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/realtime"
	return parsed.String()
}

func (a *realtimeAudioAdapter) Connect(ctx context.Context, cfg *modality.AudioSessionConfig) (modality.AudioSession, error) {
	if a == nil || a.client == nil {
		return nil, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "adapter_unavailable", "", "Audio adapter is unavailable.")
	}
	normalized, err := a.normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &realtimeAudioSession{
		ctx:     ctx,
		adapter: a,
		cfg:     *normalized,
		events:  make(chan modality.AudioServerEvent, 64),
		closeCh: make(chan struct{}),
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
		s.mu.Unlock()

		close(s.closeCh)
		if conn != nil {
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
		s.cfg.TurnDetection = &modality.TurnDetectionConfig{
			Mode:            mode,
			SilenceMS:       update.TurnDetection.SilenceMS,
			PrefixPaddingMS: update.TurnDetection.PrefixPaddingMS,
		}
	}
	if strings.TrimSpace(update.Voice) != "" {
		if s.voiceLocked && strings.TrimSpace(update.Voice) != strings.TrimSpace(s.cfg.Voice) {
			s.mu.Unlock()
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "voice_immutable", "voice", "Audio session voice cannot be changed after audio output has started.")
		}
		s.cfg.Voice = strings.TrimSpace(update.Voice)
	}
	if strings.TrimSpace(update.Instructions) != "" {
		s.cfg.Instructions = strings.TrimSpace(update.Instructions)
	}
	started := s.started
	s.mu.Unlock()

	if !started {
		s.emit(modality.AudioServerEvent{Type: modality.AudioServerEventSessionUpdated})
		return nil
	}
	return s.writeJSON(sessionUpdateEvent(s.cfg))
}

func (s *realtimeAudioSession) appendAudio(encoded string) error {
	trimmed := strings.TrimSpace(encoded)
	payload, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_audio", "audio", "Audio payload must be valid base64.")
	}
	if len(payload) == 0 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_audio", "audio", "Audio payload must not be empty.")
	}
	if err := s.ensureStarted(); err != nil {
		return err
	}
	s.mu.Lock()
	s.pendingAudio += len(payload)
	s.mu.Unlock()
	return s.writeJSON(map[string]any{
		"type":  "input_audio_buffer.append",
		"audio": base64.StdEncoding.EncodeToString(resamplePCM16MonoBytes(payload, 16000, 24000)),
	})
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
	return s.writeJSON(map[string]any{"type": "input_audio_buffer.commit"})
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
	pendingAudio := s.pendingAudio
	s.mu.Unlock()
	if pendingText == "" && pendingAudio == 0 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_turn_input", "", "No pending audio or text input is available for response generation.")
	}
	if err := s.ensureStarted(); err != nil {
		return err
	}
	if pendingAudio > 0 && s.cfg.TurnDetection.Mode == modality.TurnDetectionManual {
		if err := s.writeJSON(map[string]any{"type": "input_audio_buffer.commit"}); err != nil {
			return err
		}
	}
	if response != nil && (strings.TrimSpace(response.Voice) != "" || strings.TrimSpace(response.Instructions) != "") {
		if err := s.updateSession(&modality.AudioSessionConfig{
			Voice:        response.Voice,
			Instructions: response.Instructions,
		}); err != nil {
			return err
		}
	}
	if pendingText != "" {
		if err := s.writeJSON(map[string]any{
			"type": "conversation.item.create",
			"item": map[string]any{
				"type": "message",
				"role": "user",
				"content": []map[string]any{{
					"type": "input_text",
					"text": pendingText,
				}},
			},
		}); err != nil {
			return err
		}
		s.mu.Lock()
		s.pendingText = ""
		s.mu.Unlock()
	}
	return s.writeJSON(map[string]any{
		"type": "response.create",
		"response": map[string]any{
			"modalities":          []string{"audio", "text"},
			"conversation":        "auto",
			"output_audio_format": modality.AudioFormatPCM16,
		},
	})
}

func (s *realtimeAudioSession) cancelResponse() error {
	if err := s.ensureStarted(); err != nil {
		return err
	}
	return s.writeJSON(map[string]any{"type": "response.cancel"})
}

func (s *realtimeAudioSession) ensureStarted() error {
	s.startOnce.Do(func() {
		s.startErr = s.start()
	})
	return s.startErr
}

func (s *realtimeAudioSession) start() error {
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+s.adapter.client.apiKey)
	headers.Set("OpenAI-Beta", realtimeBetaHeader)

	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	realtimeURL := s.adapter.url + "?model=" + url.QueryEscape(s.adapter.providerModel)
	conn, _, err := dialer.DialContext(s.ctx, realtimeURL, headers)
	if err != nil {
		return httputil.ProviderTransportError(err, "OpenAI")
	}

	s.mu.Lock()
	s.conn = conn
	s.started = true
	s.mu.Unlock()

	go s.readLoop()
	return s.writeJSON(sessionUpdateEvent(s.cfg))
}

func sessionUpdateEvent(cfg modality.AudioSessionConfig) map[string]any {
	session := map[string]any{
		"instructions":        cfg.Instructions,
		"voice":               cfg.Voice,
		"input_audio_format":  modality.AudioFormatPCM16,
		"output_audio_format": modality.AudioFormatPCM16,
	}
	if cfg.TurnDetection != nil && cfg.TurnDetection.Mode == modality.TurnDetectionServerVAD {
		session["turn_detection"] = map[string]any{
			"type":                "server_vad",
			"silence_duration_ms": cfg.TurnDetection.SilenceMS,
			"prefix_padding_ms":   cfg.TurnDetection.PrefixPaddingMS,
		}
	} else {
		session["turn_detection"] = nil
	}
	return map[string]any{
		"type":    "session.update",
		"session": session,
	}
}

func (s *realtimeAudioSession) writeJSON(payload map[string]any) error {
	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()
	if conn == nil {
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_unavailable", "", "OpenAI realtime session is not connected.")
	}
	if err := conn.WriteJSON(payload); err != nil {
		return httputil.ProviderTransportError(err, "OpenAI")
	}
	return nil
}

func (s *realtimeAudioSession) readLoop() {
	for {
		select {
		case <-s.closeCh:
			return
		default:
		}

		var payload map[string]any
		if err := s.conn.ReadJSON(&payload); err != nil {
			select {
			case <-s.closeCh:
				return
			default:
			}
			s.emit(modality.AudioServerEvent{
				Type: modality.AudioServerEventError,
				Error: &modality.AudioError{
					Type:    "provider_transport_error",
					Code:    "provider_transport_error",
					Message: err.Error(),
				},
			})
			return
		}
		s.handleProviderEvent(payload)
	}
}

func (s *realtimeAudioSession) handleProviderEvent(payload map[string]any) {
	eventType := stringValue(payload["type"])
	switch eventType {
	case "session.updated":
		s.emit(modality.AudioServerEvent{Type: modality.AudioServerEventSessionUpdated})
	case "input_audio_buffer.committed":
		s.mu.Lock()
		s.pendingAudio = 0
		s.mu.Unlock()
		s.emit(modality.AudioServerEvent{Type: modality.AudioServerEventInputAudioCommitted})
	case "response.created":
		if response, ok := payload["response"].(map[string]any); ok {
			s.mu.Lock()
			s.responseID = stringValue(response["id"])
			s.mu.Unlock()
		}
	case "response.audio.delta", "response.output_audio.delta":
		s.mu.Lock()
		s.voiceLocked = true
		s.mu.Unlock()
		audioDelta := stringValue(payload["delta"])
		if decoded, err := base64.StdEncoding.DecodeString(audioDelta); err == nil {
			audioDelta = base64.StdEncoding.EncodeToString(resamplePCM16MonoBytes(decoded, 24000, 16000))
		}
		s.emit(modality.AudioServerEvent{
			Type:       modality.AudioServerEventResponseAudioDelta,
			ResponseID: s.currentResponseID(),
			Audio:      audioDelta,
		})
	case "response.audio.done", "response.output_audio.done":
		s.mu.Lock()
		s.voiceLocked = true
		s.mu.Unlock()
		s.emit(modality.AudioServerEvent{
			Type:       modality.AudioServerEventResponseAudioDone,
			ResponseID: s.currentResponseID(),
		})
	case "response.audio_transcript.delta", "response.output_audio_transcript.delta":
		s.emit(modality.AudioServerEvent{
			Type:       modality.AudioServerEventResponseTranscriptDelta,
			ResponseID: s.currentResponseID(),
			Transcript: stringValue(payload["delta"]),
		})
	case "response.audio_transcript.done", "response.output_audio_transcript.done":
		s.emit(modality.AudioServerEvent{
			Type:       modality.AudioServerEventResponseTranscriptDone,
			ResponseID: s.currentResponseID(),
			Transcript: firstNonEmptyString(stringValue(payload["transcript"]), stringValue(payload["text"])),
		})
	case "response.text.delta", "response.output_text.delta":
		s.emit(modality.AudioServerEvent{
			Type:       modality.AudioServerEventResponseTextDelta,
			ResponseID: s.currentResponseID(),
			Text:       firstNonEmptyString(stringValue(payload["delta"]), stringValue(payload["text"])),
		})
	case "response.text.done", "response.output_text.done":
		s.emit(modality.AudioServerEvent{
			Type:       modality.AudioServerEventResponseTextDone,
			ResponseID: s.currentResponseID(),
			Text:       stringValue(payload["text"]),
		})
	case "response.done":
		response, _ := payload["response"].(map[string]any)
		if response != nil {
			s.mu.Lock()
			s.responseID = stringValue(response["id"])
			s.mu.Unlock()
		}
		usage := nativeUsageFromMap(nil)
		if response != nil {
			usage = nativeUsageFromMap(response["usage"])
		}
		audioUsage := (*modality.AudioUsage)(nil)
		if usage != nil {
			audioUsage = &modality.AudioUsage{
				InputTextTokens:  usage.PromptTokens,
				OutputTextTokens: usage.CompletionTokens,
				TotalTokens:      usage.TotalTokens,
				Source:           usage.Source,
			}
		}
		s.emit(modality.AudioServerEvent{
			Type:       modality.AudioServerEventResponseCompleted,
			ResponseID: s.currentResponseID(),
			Usage:      audioUsage,
		})
	case "error":
		errObject, _ := payload["error"].(map[string]any)
		s.emit(modality.AudioServerEvent{
			Type: modality.AudioServerEventError,
			Error: &modality.AudioError{
				Type:    firstNonEmptyString(stringValue(errObject["type"]), "provider_error"),
				Code:    stringValue(errObject["code"]),
				Message: firstNonEmptyString(stringValue(errObject["message"]), "OpenAI realtime request failed."),
				Param:   stringValue(errObject["param"]),
			},
		})
	}
}

func (s *realtimeAudioSession) currentResponseID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.responseID
}

func (s *realtimeAudioSession) emit(event modality.AudioServerEvent) {
	select {
	case <-s.closeCh:
		return
	case s.events <- event:
	}
}

func stringValue(value any) string {
	typed, _ := value.(string)
	return strings.TrimSpace(typed)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func resamplePCM16MonoBytes(data []byte, srcRate int, dstRate int) []byte {
	if len(data) == 0 || srcRate <= 0 || dstRate <= 0 || srcRate == dstRate {
		return append([]byte(nil), data...)
	}
	samples := pcm16BytesToSamples(data)
	if len(samples) == 0 {
		return nil
	}
	return samplesToPCM16Bytes(resamplePCM16Mono(samples, srcRate, dstRate))
}

func pcm16BytesToSamples(data []byte) []int16 {
	if len(data) < 2 {
		return nil
	}
	sampleCount := len(data) / 2
	samples := make([]int16, sampleCount)
	for i := 0; i < sampleCount; i++ {
		samples[i] = int16(binary.LittleEndian.Uint16(data[i*2:]))
	}
	return samples
}

func samplesToPCM16Bytes(samples []int16) []byte {
	if len(samples) == 0 {
		return nil
	}
	data := make([]byte, len(samples)*2)
	for i, sample := range samples {
		binary.LittleEndian.PutUint16(data[i*2:], uint16(sample))
	}
	return data
}

func resamplePCM16Mono(samples []int16, srcRate int, dstRate int) []int16 {
	if len(samples) == 0 || srcRate <= 0 || dstRate <= 0 || srcRate == dstRate {
		return append([]int16(nil), samples...)
	}
	outputLen := int(math.Round(float64(len(samples)) * float64(dstRate) / float64(srcRate)))
	if outputLen < 1 {
		outputLen = 1
	}
	if len(samples) == 1 {
		return []int16{samples[0]}
	}
	out := make([]int16, outputLen)
	for i := 0; i < outputLen; i++ {
		position := float64(i) * float64(srcRate) / float64(dstRate)
		index := int(position)
		if index >= len(samples)-1 {
			out[i] = samples[len(samples)-1]
			continue
		}
		fraction := position - float64(index)
		value := (1-fraction)*float64(samples[index]) + fraction*float64(samples[index+1])
		out[i] = int16(math.Round(value))
	}
	return out
}
