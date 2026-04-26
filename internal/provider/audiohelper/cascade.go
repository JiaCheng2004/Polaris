package audiohelper

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/common/safeconv"
)

type Transcriber func(ctx context.Context, req *modality.STTRequest) (*modality.TranscriptResponse, error)
type Synthesizer func(ctx context.Context, req *modality.TTSRequest) (*modality.AudioResponse, error)

type CascadeAdapter struct {
	model          string
	chatModel      string
	sttModel       string
	ttsModel       string
	chat           modality.ChatAdapter
	transcribe     Transcriber
	synthesize     Synthesizer
	turnModes      map[string]struct{}
	defaultSession time.Duration
}

func NewCascadeAdapter(model string, chatModel string, sttModel string, ttsModel string, chat modality.ChatAdapter, transcribe Transcriber, synthesize Synthesizer, turnModes []string, defaultSession time.Duration) *CascadeAdapter {
	supported := make(map[string]struct{}, len(turnModes))
	for _, mode := range turnModes {
		supported[strings.TrimSpace(mode)] = struct{}{}
	}
	if defaultSession <= 0 {
		defaultSession = 10 * time.Minute
	}
	return &CascadeAdapter{
		model:          model,
		chatModel:      chatModel,
		sttModel:       sttModel,
		ttsModel:       ttsModel,
		chat:           chat,
		transcribe:     transcribe,
		synthesize:     synthesize,
		turnModes:      supported,
		defaultSession: defaultSession,
	}
}

func (a *CascadeAdapter) Connect(ctx context.Context, cfg *modality.AudioSessionConfig) (modality.AudioSession, error) {
	if a == nil || a.chat == nil || a.transcribe == nil || a.synthesize == nil {
		return nil, httputil.NewError(503, "provider_error", "adapter_unavailable", "", "Audio adapter is unavailable.")
	}
	normalized, err := a.normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	session := &CascadeSession{
		ctx:        ctx,
		cfg:        *normalized,
		chatModel:  a.chatModel,
		sttModel:   a.sttModel,
		ttsModel:   a.ttsModel,
		chat:       a.chat,
		transcribe: a.transcribe,
		synthesize: a.synthesize,
		events:     make(chan modality.AudioServerEvent, 32),
		closeCh:    make(chan struct{}),
		defaultTTL: a.defaultSession,
	}
	return session, nil
}

func (a *CascadeAdapter) normalizeConfig(cfg *modality.AudioSessionConfig) (*modality.AudioSessionConfig, error) {
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
		return nil, httputil.NewError(400, "invalid_request_error", "unsupported_sample_rate", "sample_rate_hz", "Only 16000 Hz audio sessions are supported.")
	}
	if normalized.InputAudioFormat != modality.AudioFormatPCM16 {
		return nil, httputil.NewError(400, "invalid_request_error", "unsupported_input_audio_format", "input_audio_format", "Only pcm16 input audio is supported.")
	}
	if normalized.OutputAudioFormat != modality.AudioFormatPCM16 {
		return nil, httputil.NewError(400, "invalid_request_error", "unsupported_output_audio_format", "output_audio_format", "Only pcm16 output audio is supported.")
	}
	if normalized.TurnDetection == nil {
		normalized.TurnDetection = &modality.TurnDetectionConfig{Mode: modality.TurnDetectionManual}
	}
	mode := strings.TrimSpace(normalized.TurnDetection.Mode)
	if mode == "" {
		mode = modality.TurnDetectionManual
		normalized.TurnDetection.Mode = mode
	}
	if _, ok := a.turnModes[mode]; !ok {
		return nil, httputil.NewError(400, "invalid_request_error", "unsupported_turn_detection", "turn_detection.mode", "Requested turn detection mode is not supported by this model.")
	}
	return &normalized, nil
}

type CascadeSession struct {
	ctx        context.Context
	cfg        modality.AudioSessionConfig
	chatModel  string
	sttModel   string
	ttsModel   string
	chat       modality.ChatAdapter
	transcribe Transcriber
	synthesize Synthesizer
	events     chan modality.AudioServerEvent
	closeCh    chan struct{}
	defaultTTL time.Duration

	mu               sync.Mutex
	closed           bool
	history          []modality.ChatMessage
	pendingPCM       []byte
	pendingText      string
	inputAudioSecs   float64
	outputAudioSecs  float64
	inputTextTokens  int
	outputTextTokens int
	activeCancel     context.CancelFunc
	sequence         uint64
}

func (s *CascadeSession) Send(event modality.AudioClientEvent) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return httputil.NewError(410, "invalid_request_error", "session_closed", "", "Audio session is closed.")
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
		return httputil.NewError(400, "invalid_request_error", "unknown_event_type", "type", "Unknown audio client event type.")
	}
}

func (s *CascadeSession) Events() <-chan modality.AudioServerEvent {
	return s.events
}

func (s *CascadeSession) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	cancel := s.activeCancel
	s.activeCancel = nil
	close(s.closeCh)
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

func (s *CascadeSession) Usage() modality.AudioUsage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return modality.AudioUsage{
		InputAudioSeconds:  s.inputAudioSecs,
		OutputAudioSeconds: s.outputAudioSecs,
		InputTextTokens:    s.inputTextTokens,
		OutputTextTokens:   s.outputTextTokens,
		TotalTokens:        s.inputTextTokens + s.outputTextTokens,
	}
}

func (s *CascadeSession) updateSession(update *modality.AudioSessionConfig) error {
	if update == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(update.Model) != "" && strings.TrimSpace(update.Model) != strings.TrimSpace(s.cfg.Model) {
		return httputil.NewError(400, "invalid_request_error", "model_immutable", "model", "Audio session model cannot be changed after creation.")
	}
	if update.Voice != "" {
		s.cfg.Voice = update.Voice
	}
	if update.Instructions != "" {
		s.cfg.Instructions = update.Instructions
	}
	if update.InputAudioFormat != "" && update.InputAudioFormat != modality.AudioFormatPCM16 {
		return httputil.NewError(400, "invalid_request_error", "unsupported_input_audio_format", "input_audio_format", "Only pcm16 input audio is supported.")
	}
	if update.OutputAudioFormat != "" && update.OutputAudioFormat != modality.AudioFormatPCM16 {
		return httputil.NewError(400, "invalid_request_error", "unsupported_output_audio_format", "output_audio_format", "Only pcm16 output audio is supported.")
	}
	if update.SampleRateHz != 0 && update.SampleRateHz != 16000 {
		return httputil.NewError(400, "invalid_request_error", "unsupported_sample_rate", "sample_rate_hz", "Only 16000 Hz audio sessions are supported.")
	}
	if update.TurnDetection != nil {
		mode := strings.TrimSpace(update.TurnDetection.Mode)
		if mode == "" {
			mode = modality.TurnDetectionManual
		}
		if mode != modality.TurnDetectionManual && mode != modality.TurnDetectionServerVAD {
			return httputil.NewError(400, "invalid_request_error", "unsupported_turn_detection", "turn_detection.mode", "Requested turn detection mode is not supported by this model.")
		}
		s.cfg.TurnDetection = update.TurnDetection
		s.cfg.TurnDetection.Mode = mode
	}
	s.emit(modality.AudioServerEvent{
		Type:    modality.AudioServerEventSessionUpdated,
		EventID: s.nextEventID("evt"),
	})
	return nil
}

func (s *CascadeSession) appendAudio(encoded string) error {
	payload, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return httputil.NewError(400, "invalid_request_error", "invalid_audio", "audio", "Audio payload must be valid base64.")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingPCM = append(s.pendingPCM, payload...)
	return nil
}

func (s *CascadeSession) commitAudio() error {
	s.mu.Lock()
	if len(s.pendingPCM) == 0 {
		s.mu.Unlock()
		return httputil.NewError(400, "invalid_request_error", "missing_audio", "audio", "No buffered audio is available to commit.")
	}
	pcm := append([]byte(nil), s.pendingPCM...)
	s.pendingPCM = nil
	s.mu.Unlock()

	wav, err := pcm16ToWAV(pcm, 16000)
	if err != nil {
		return httputil.NewError(400, "invalid_request_error", "invalid_audio", "audio", err.Error())
	}
	resp, err := s.transcribe(s.ctx, &modality.STTRequest{
		Model:          s.sttModel,
		File:           wav,
		Filename:       "input.wav",
		ContentType:    "audio/wav",
		ResponseFormat: "json",
	})
	if err != nil {
		return err
	}

	text := ""
	if resp != nil {
		text = strings.TrimSpace(resp.Text)
	}
	duration := float64(len(pcm)) / 2 / 16000

	s.mu.Lock()
	s.pendingText = text
	s.inputAudioSecs += duration
	mode := modality.TurnDetectionManual
	if s.cfg.TurnDetection != nil && strings.TrimSpace(s.cfg.TurnDetection.Mode) != "" {
		mode = s.cfg.TurnDetection.Mode
	}
	s.mu.Unlock()

	s.emit(modality.AudioServerEvent{
		Type:       modality.AudioServerEventInputAudioCommitted,
		EventID:    s.nextEventID("evt"),
		Transcript: text,
	})
	if mode == modality.TurnDetectionServerVAD {
		return s.createResponse(nil)
	}
	return nil
}

func (s *CascadeSession) setInputText(text string) error {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return httputil.NewError(400, "invalid_request_error", "missing_text", "text", "Input text must not be empty.")
	}
	s.mu.Lock()
	s.pendingText = trimmed
	s.mu.Unlock()
	return nil
}

func (s *CascadeSession) createResponse(response *modality.AudioResponseConfig) error {
	s.mu.Lock()
	if s.activeCancel != nil {
		s.mu.Unlock()
		return httputil.NewError(409, "invalid_request_error", "response_in_progress", "", "Audio session is already generating a response.")
	}
	pendingText := strings.TrimSpace(s.pendingText)
	if pendingText == "" {
		s.mu.Unlock()
		return httputil.NewError(400, "invalid_request_error", "missing_turn_input", "", "No pending audio or text input is available for response generation.")
	}
	history := append([]modality.ChatMessage(nil), s.history...)
	cfg := s.cfg
	ctx, cancel := context.WithCancel(s.ctx)
	s.activeCancel = cancel
	s.mu.Unlock()

	go s.runResponse(ctx, cancel, history, cfg, pendingText, response)
	return nil
}

func (s *CascadeSession) cancelResponse() error {
	s.mu.Lock()
	cancel := s.activeCancel
	s.activeCancel = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

func (s *CascadeSession) runResponse(ctx context.Context, cancel context.CancelFunc, history []modality.ChatMessage, cfg modality.AudioSessionConfig, userText string, override *modality.AudioResponseConfig) {
	defer cancel()

	systemText := strings.TrimSpace(cfg.Instructions)
	if override != nil && strings.TrimSpace(override.Instructions) != "" {
		systemText = strings.TrimSpace(override.Instructions)
	}
	voice := strings.TrimSpace(cfg.Voice)
	if override != nil && strings.TrimSpace(override.Voice) != "" {
		voice = strings.TrimSpace(override.Voice)
	}

	userMessage := modality.ChatMessage{Role: "user", Content: modality.NewTextContent(userText)}
	messages := make([]modality.ChatMessage, 0, len(history)+2)
	if systemText != "" {
		messages = append(messages, modality.ChatMessage{Role: "system", Content: modality.NewTextContent(systemText)})
	}
	messages = append(messages, history...)
	messages = append(messages, userMessage)

	chatResp, err := s.chat.Complete(ctx, &modality.ChatRequest{
		Model:    s.chatModel,
		Messages: messages,
	})
	if err != nil {
		s.failResponse(err)
		return
	}

	assistantText := strings.TrimSpace(firstChoiceText(chatResp))
	if assistantText == "" {
		s.failResponse(httputil.NewError(502, "provider_error", "provider_invalid_response", "", "Audio chat provider returned an empty assistant response."))
		return
	}

	audioResp, err := s.synthesize(ctx, &modality.TTSRequest{
		Model:          s.ttsModel,
		Input:          assistantText,
		Voice:          voice,
		ResponseFormat: "pcm",
	})
	if err != nil {
		s.failResponse(err)
		return
	}
	if audioResp == nil || len(audioResp.Data) == 0 {
		s.failResponse(httputil.NewError(502, "provider_error", "provider_invalid_response", "", "Audio synthesis provider returned an empty response."))
		return
	}

	responseID := s.nextEventID("resp")
	audioPayload := base64.StdEncoding.EncodeToString(audioResp.Data)
	audioSeconds := float64(len(audioResp.Data)) / 2 / 16000
	usage := modality.AudioUsage{
		InputTextTokens:    chatResp.Usage.PromptTokens,
		OutputTextTokens:   chatResp.Usage.CompletionTokens,
		InputAudioSeconds:  0,
		OutputAudioSeconds: audioSeconds,
		TotalTokens:        chatResp.Usage.TotalTokens,
	}

	s.emit(modality.AudioServerEvent{
		Type:       modality.AudioServerEventResponseTextDelta,
		EventID:    s.nextEventID("evt"),
		ResponseID: responseID,
		Text:       assistantText,
	})
	s.emit(modality.AudioServerEvent{
		Type:       modality.AudioServerEventResponseTextDone,
		EventID:    s.nextEventID("evt"),
		ResponseID: responseID,
		Text:       assistantText,
	})
	s.emit(modality.AudioServerEvent{
		Type:       modality.AudioServerEventResponseTranscriptDelta,
		EventID:    s.nextEventID("evt"),
		ResponseID: responseID,
		Transcript: assistantText,
	})
	s.emit(modality.AudioServerEvent{
		Type:       modality.AudioServerEventResponseTranscriptDone,
		EventID:    s.nextEventID("evt"),
		ResponseID: responseID,
		Transcript: assistantText,
	})
	s.emit(modality.AudioServerEvent{
		Type:       modality.AudioServerEventResponseAudioDelta,
		EventID:    s.nextEventID("evt"),
		ResponseID: responseID,
		Audio:      audioPayload,
	})
	s.emit(modality.AudioServerEvent{
		Type:       modality.AudioServerEventResponseAudioDone,
		EventID:    s.nextEventID("evt"),
		ResponseID: responseID,
		Audio:      audioPayload,
	})
	s.emit(modality.AudioServerEvent{
		Type:       modality.AudioServerEventResponseCompleted,
		EventID:    s.nextEventID("evt"),
		ResponseID: responseID,
		Usage:      &usage,
	})

	s.mu.Lock()
	s.pendingText = ""
	s.history = append(s.history, userMessage, modality.ChatMessage{Role: "assistant", Content: modality.NewTextContent(assistantText)})
	s.inputTextTokens += chatResp.Usage.PromptTokens
	s.outputTextTokens += chatResp.Usage.CompletionTokens
	s.outputAudioSecs += audioSeconds
	s.activeCancel = nil
	s.mu.Unlock()
}

func (s *CascadeSession) failResponse(err error) {
	s.mu.Lock()
	s.activeCancel = nil
	s.mu.Unlock()

	apiErr, ok := err.(*httputil.APIError)
	if !ok {
		apiErr = httputil.NewError(500, "internal_error", "internal_error", "", err.Error())
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

func (s *CascadeSession) emit(event modality.AudioServerEvent) {
	select {
	case <-s.closeCh:
		return
	case s.events <- event:
	}
}

func (s *CascadeSession) nextEventID(prefix string) string {
	value := atomic.AddUint64(&s.sequence, 1)
	return fmt.Sprintf("%s_%06d", prefix, value)
}

func firstChoiceText(response *modality.ChatResponse) string {
	if response == nil || len(response.Choices) == 0 {
		return ""
	}
	choice := response.Choices[0]
	if choice.Message.Content.Text != nil {
		return *choice.Message.Content.Text
	}
	var builder strings.Builder
	for _, part := range choice.Message.Content.Parts {
		if part.Type == "text" && part.Text != "" {
			if builder.Len() > 0 {
				builder.WriteByte('\n')
			}
			builder.WriteString(part.Text)
		}
	}
	return builder.String()
}

func pcm16ToWAV(pcm []byte, sampleRate int) ([]byte, error) {
	dataSize, err := safeconv.Uint32FromInt("wav pcm data size", len(pcm))
	if err != nil {
		return nil, err
	}
	byteRate, err := safeconv.Uint32FromInt("wav byte rate", sampleRate*2)
	if err != nil {
		return nil, err
	}
	headerSize, err := safeconv.Uint32FromInt("wav riff payload size", 36+len(pcm))
	if err != nil {
		return nil, err
	}
	sampleRateValue, err := safeconv.Uint32FromInt("wav sample rate", sampleRate)
	if err != nil {
		return nil, err
	}
	blockAlign := uint16(2)
	buf := bytes.NewBuffer(make([]byte, 0, 44+len(pcm)))
	buf.WriteString("RIFF")
	_ = binary.Write(buf, binary.LittleEndian, headerSize)
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	_ = binary.Write(buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(buf, binary.LittleEndian, sampleRateValue)
	_ = binary.Write(buf, binary.LittleEndian, byteRate)
	_ = binary.Write(buf, binary.LittleEndian, blockAlign)
	_ = binary.Write(buf, binary.LittleEndian, uint16(16))
	buf.WriteString("data")
	_ = binary.Write(buf, binary.LittleEndian, dataSize)
	buf.Write(pcm)
	return buf.Bytes(), nil
}
