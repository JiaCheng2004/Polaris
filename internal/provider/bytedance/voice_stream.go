package bytedance

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/gorilla/websocket"
)

const (
	defaultStreamingTranscriptionURL   = "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async"
	streamingTranscriptionReadyTimeout = 10 * time.Second
)

type streamingTranscriptionAdapter struct {
	client         *Client
	model          string
	url            string
	resourceID     string
	defaultSession time.Duration
}

type streamingTranscriptionSession struct {
	ctx        context.Context
	adapter    *streamingTranscriptionAdapter
	cfg        modality.StreamingTranscriptionSessionConfig
	events     chan modality.StreamingTranscriptionServerEvent
	closeCh    chan struct{}
	connectID  string
	startReady chan error

	writeMu sync.Mutex
	mu      sync.Mutex

	conn       *websocket.Conn
	started    bool
	closed     bool
	audioBytes int
	lastText   string
	lastDurSec float64
	segments   []modality.TranscriptSegment
	segmentSet map[string]struct{}

	startOnce sync.Once
	startErr  error
	closeOnce sync.Once
	counter   uint64
}

type streamingASRRequest struct {
	User    sttUserConfig             `json:"user"`
	Audio   streamingASRAudioConfig   `json:"audio"`
	Request streamingASRRequestConfig `json:"request"`
}

type streamingASRAudioConfig struct {
	Language string `json:"language,omitempty"`
	Format   string `json:"format"`
	Codec    string `json:"codec,omitempty"`
	Rate     int    `json:"rate"`
	Bits     int    `json:"bits"`
	Channel  int    `json:"channel"`
}

type streamingASRRequestConfig struct {
	ModelName       string `json:"model_name"`
	EnableITN       bool   `json:"enable_itn"`
	EnablePunc      bool   `json:"enable_punc"`
	ShowUtterances  bool   `json:"show_utterances,omitempty"`
	ResultType      string `json:"result_type,omitempty"`
	EnableNonstream bool   `json:"enable_nonstream,omitempty"`
}

func NewStreamingTranscriptionAdapter(client *Client, model string, endpoint string) modality.StreamingTranscriptionAdapter {
	return &streamingTranscriptionAdapter{
		client:         client,
		model:          model,
		url:            strings.TrimSpace(endpoint),
		resourceID:     providerStreamingASRResourceID(model, model),
		defaultSession: 10 * time.Minute,
	}
}

func (a *streamingTranscriptionAdapter) ConnectStreamingTranscription(ctx context.Context, cfg *modality.StreamingTranscriptionSessionConfig) (modality.StreamingTranscriptionSession, error) {
	if a == nil || a.client == nil {
		return nil, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "adapter_unavailable", "", "Streaming transcription adapter is unavailable.")
	}
	if strings.TrimSpace(a.client.appID) == "" || strings.TrimSpace(a.client.speechToken) == "" {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_misconfigured", "", "ByteDance streaming transcription requires providers.bytedance.app_id and providers.bytedance.speech_access_token.")
	}
	normalized, err := a.normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &streamingTranscriptionSession{
		ctx:        ctx,
		adapter:    a,
		cfg:        *normalized,
		events:     make(chan modality.StreamingTranscriptionServerEvent, 64),
		closeCh:    make(chan struct{}),
		connectID:  newRealtimeSessionID(),
		startReady: make(chan error, 1),
		segmentSet: map[string]struct{}{},
	}, nil
}

func (a *streamingTranscriptionAdapter) normalizeConfig(cfg *modality.StreamingTranscriptionSessionConfig) (*modality.StreamingTranscriptionSessionConfig, error) {
	if cfg == nil {
		cfg = &modality.StreamingTranscriptionSessionConfig{}
	}
	normalized := *cfg
	if strings.TrimSpace(normalized.Model) == "" {
		normalized.Model = a.model
	}
	if strings.TrimSpace(normalized.InputAudioFormat) == "" {
		normalized.InputAudioFormat = modality.AudioFormatPCM16
	}
	if normalized.SampleRateHz == 0 {
		normalized.SampleRateHz = 16000
	}
	if normalized.SampleRateHz != 16000 {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_sample_rate", "sample_rate_hz", "Only 16000 Hz streaming transcription sessions are supported.")
	}
	if normalized.InputAudioFormat != modality.AudioFormatPCM16 {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_input_audio_format", "input_audio_format", "Only pcm16 input audio is supported.")
	}
	if normalized.InterimResults == nil {
		value := true
		normalized.InterimResults = &value
	}
	if normalized.ReturnUtterances == nil {
		value := true
		normalized.ReturnUtterances = &value
	}
	streamURL := a.streamURL()
	if strings.TrimSpace(normalized.Language) != "" && !strings.Contains(streamURL, "bigmodel_nostream") {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_language", "language", "ByteDance streaming transcription only accepts 'language' on the bigmodel_nostream endpoint.")
	}
	return &normalized, nil
}

func (a *streamingTranscriptionAdapter) streamURL() string {
	if a.url != "" {
		return a.url
	}
	return defaultStreamingTranscriptionURL
}

func (s *streamingTranscriptionSession) Send(event modality.StreamingTranscriptionClientEvent) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return httputil.NewError(http.StatusGone, "invalid_request_error", "session_closed", "", "Streaming transcription session is closed.")
	}
	s.mu.Unlock()

	switch event.Type {
	case modality.StreamingTranscriptionClientEventSessionUpdate:
		return s.updateSession(event.Session)
	case modality.StreamingTranscriptionClientEventInputAudioAppend:
		return s.appendAudio(event.Audio)
	case modality.StreamingTranscriptionClientEventInputAudioCommit:
		return s.commitAudio()
	case modality.StreamingTranscriptionClientEventSessionClose:
		return s.Close()
	default:
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unknown_event_type", "type", "Unknown streaming transcription client event type.")
	}
}

func (s *streamingTranscriptionSession) Events() <-chan modality.StreamingTranscriptionServerEvent {
	return s.events
}

func (s *streamingTranscriptionSession) Close() error {
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

func (s *streamingTranscriptionSession) updateSession(update *modality.StreamingTranscriptionSessionConfig) error {
	if update == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(update.Model) != "" && strings.TrimSpace(update.Model) != strings.TrimSpace(s.cfg.Model) {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "model_immutable", "model", "Streaming transcription session model cannot be changed after creation.")
	}
	if update.InputAudioFormat != "" && update.InputAudioFormat != modality.AudioFormatPCM16 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_input_audio_format", "input_audio_format", "Only pcm16 input audio is supported.")
	}
	if update.SampleRateHz != 0 && update.SampleRateHz != 16000 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_sample_rate", "sample_rate_hz", "Only 16000 Hz streaming transcription sessions are supported.")
	}
	if s.started && strings.TrimSpace(update.Language) != "" && update.Language != s.cfg.Language {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "language_immutable", "language", "Streaming transcription language cannot be changed after the provider session starts.")
	}
	if strings.TrimSpace(update.Language) != "" {
		s.cfg.Language = strings.TrimSpace(update.Language)
	}
	if update.InterimResults != nil {
		value := *update.InterimResults
		s.cfg.InterimResults = &value
	}
	if update.ReturnUtterances != nil {
		value := *update.ReturnUtterances
		s.cfg.ReturnUtterances = &value
	}
	s.emit(modality.StreamingTranscriptionServerEvent{
		Type:    modality.StreamingTranscriptionServerEventSessionUpdated,
		EventID: s.nextEventID(),
	})
	return nil
}

func (s *streamingTranscriptionSession) appendAudio(encoded string) error {
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
	frame, err := encodeStreamingASRAudio(payload, false)
	if err != nil {
		return httputil.NewError(http.StatusInternalServerError, "internal_error", "internal_error", "", "Failed to encode streaming audio payload.")
	}
	if err := s.writeBinary(frame); err != nil {
		return err
	}
	s.mu.Lock()
	s.audioBytes += len(payload)
	s.mu.Unlock()
	return nil
}

func (s *streamingTranscriptionSession) commitAudio() error {
	s.mu.Lock()
	pending := s.audioBytes
	s.mu.Unlock()
	if pending == 0 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_audio", "audio", "No buffered audio is available to commit.")
	}
	if err := s.ensureStarted(); err != nil {
		return err
	}
	frame, err := encodeStreamingASRAudio(nil, true)
	if err != nil {
		return httputil.NewError(http.StatusInternalServerError, "internal_error", "internal_error", "", "Failed to encode final streaming audio payload.")
	}
	if err := s.writeBinary(frame); err != nil {
		return err
	}
	s.mu.Lock()
	s.audioBytes = 0
	s.mu.Unlock()
	s.emit(modality.StreamingTranscriptionServerEvent{
		Type:    modality.StreamingTranscriptionServerEventInputAudioCommitted,
		EventID: s.nextEventID(),
	})
	return nil
}

func (s *streamingTranscriptionSession) ensureStarted() error {
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
	case <-time.After(streamingTranscriptionReadyTimeout):
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_timeout", "", "ByteDance streaming transcription session did not start in time.")
	case err := <-s.startReady:
		return err
	}
}

func (s *streamingTranscriptionSession) start() error {
	wsURL := strings.TrimSpace(s.adapter.streamURL())
	if _, err := url.Parse(wsURL); err != nil {
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_misconfigured", "", "ByteDance streaming transcription URL is invalid.")
	}

	headers := http.Header{}
	headers.Set("X-Api-App-Key", s.adapter.client.appID)
	headers.Set("X-Api-Access-Key", s.adapter.client.speechToken)
	headers.Set("X-Api-Resource-Id", providerStreamingASRResourceID(s.cfg.Model, s.adapter.model))
	headers.Set("X-Api-Connect-Id", s.connectID)

	dialer := websocket.Dialer{HandshakeTimeout: 30 * time.Second}
	conn, resp, err := dialer.DialContext(s.ctx, wsURL, headers)
	if err != nil {
		if resp != nil && resp.StatusCode > 0 {
			return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_transport_error", "", fmt.Sprintf("ByteDance streaming transcription handshake failed with status %d.", resp.StatusCode))
		}
		return translateTransportError(err, "ByteDance")
	}

	request := s.startRequest()
	frame, err := encodeStreamingASRRequest(request)
	if err != nil {
		_ = conn.Close()
		return httputil.NewError(http.StatusInternalServerError, "internal_error", "internal_error", "", "Failed to encode streaming transcription request.")
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		_ = conn.Close()
		return translateTransportError(err, "ByteDance")
	}

	s.mu.Lock()
	s.conn = conn
	s.started = true
	s.mu.Unlock()

	go s.readLoop(conn)
	s.signalStart(nil)
	return nil
}

func (s *streamingTranscriptionSession) startRequest() streamingASRRequest {
	return streamingASRRequest{
		User: sttUserConfig{
			UID: newRequestID(),
		},
		Audio: streamingASRAudioConfig{
			Language: strings.TrimSpace(s.cfg.Language),
			Format:   "pcm",
			Codec:    "raw",
			Rate:     16000,
			Bits:     16,
			Channel:  1,
		},
		Request: streamingASRRequestConfig{
			ModelName:       "bigmodel",
			EnableITN:       true,
			EnablePunc:      true,
			ShowUtterances:  s.cfg.ReturnUtterances != nil && *s.cfg.ReturnUtterances,
			ResultType:      "full",
			EnableNonstream: strings.Contains(s.adapter.streamURL(), "bigmodel_async"),
		},
	}
}

func (s *streamingTranscriptionSession) readLoop(conn *websocket.Conn) {
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
			err := httputil.NewError(http.StatusBadGateway, "provider_error", "provider_transport_error", "", "ByteDance streaming transcription connection closed unexpectedly.")
			s.emitError(err)
			s.signalStart(err)
			return
		}
		if messageType != websocket.BinaryMessage {
			continue
		}
		frame, err := decodeStreamingASRFrame(payload)
		if err != nil {
			apiErr := httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance returned an invalid streaming transcription frame.")
			s.emitError(apiErr)
			s.signalStart(apiErr)
			return
		}
		if err := s.handleFrame(frame); err != nil {
			s.emitError(err)
			s.signalStart(err)
			return
		}
	}
}

func (s *streamingTranscriptionSession) handleFrame(frame streamingASRFrame) error {
	switch frame.MessageType {
	case streamingASRMessageTypeFullServer:
		var payload sttResponse
		if err := json.Unmarshal(frame.Payload, &payload); err != nil {
			return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance returned an invalid streaming transcription payload.")
		}
		return s.handleResponse(frame, payload)
	case streamingASRMessageTypeError:
		message := strings.TrimSpace(string(frame.Payload))
		if message == "" {
			message = "ByteDance streaming transcription returned an error."
		}
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_server_error", "", message)
	default:
		return nil
	}
}

func (s *streamingTranscriptionSession) handleResponse(frame streamingASRFrame, payload sttResponse) error {
	text := strings.TrimSpace(payload.Result.Text)
	segments := s.captureSegments(payload.Result.Utterances)
	delta := s.updateText(text)
	final := isStreamingASRFinalFrame(frame.Flags)

	if delta != "" && s.shouldEmitInterim() {
		s.emit(modality.StreamingTranscriptionServerEvent{
			Type:    modality.StreamingTranscriptionServerEventTranscriptDelta,
			EventID: s.nextEventID(),
			Text:    delta,
		})
	}
	for _, segment := range segments {
		if s.shouldReturnUtterances() {
			seg := modality.StreamingTranscriptSegment{
				ID:    segment.ID,
				Start: segment.Start,
				End:   segment.End,
				Text:  segment.Text,
				Final: true,
			}
			s.emit(modality.StreamingTranscriptionServerEvent{
				Type:    modality.StreamingTranscriptionServerEventTranscriptSegment,
				EventID: s.nextEventID(),
				Segment: &seg,
			})
		}
	}

	s.mu.Lock()
	returnUtterances := s.cfg.ReturnUtterances != nil && *s.cfg.ReturnUtterances
	s.lastDurSec = payload.AudioInfo.Duration / 1000
	transcript := modality.TranscriptResponse{
		Text:     s.lastText,
		Language: strings.TrimSpace(s.cfg.Language),
		Duration: s.lastDurSec,
		Format:   "json",
	}
	if returnUtterances {
		transcript.Segments = append([]modality.TranscriptSegment(nil), s.segments...)
	}
	s.mu.Unlock()

	if final {
		if delta != "" && !s.shouldEmitInterim() {
			s.emit(modality.StreamingTranscriptionServerEvent{
				Type:    modality.StreamingTranscriptionServerEventTranscriptDelta,
				EventID: s.nextEventID(),
				Text:    delta,
			})
		}
		s.emit(modality.StreamingTranscriptionServerEvent{
			Type:       modality.StreamingTranscriptionServerEventTranscriptCompleted,
			EventID:    s.nextEventID(),
			Text:       transcript.Text,
			Transcript: &transcript,
		})
	}
	return nil
}

func (s *streamingTranscriptionSession) captureSegments(utterances []sttUtterance) []modality.TranscriptSegment {
	if len(utterances) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var added []modality.TranscriptSegment
	for _, utterance := range utterances {
		if !utterance.Definite {
			continue
		}
		key := fmt.Sprintf("%d:%d:%s", utterance.StartTime, utterance.EndTime, strings.TrimSpace(utterance.Text))
		if _, exists := s.segmentSet[key]; exists {
			continue
		}
		segment := modality.TranscriptSegment{
			ID:    len(s.segments),
			Start: float64(utterance.StartTime) / 1000,
			End:   float64(utterance.EndTime) / 1000,
			Text:  strings.TrimSpace(utterance.Text),
		}
		s.segmentSet[key] = struct{}{}
		s.segments = append(s.segments, segment)
		added = append(added, segment)
	}
	return added
}

func (s *streamingTranscriptionSession) updateText(text string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.lastText
	if strings.HasPrefix(text, previous) {
		s.lastText = text
		return text[len(previous):]
	}
	s.lastText = text
	return text
}

func (s *streamingTranscriptionSession) shouldEmitInterim() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg.InterimResults != nil && *s.cfg.InterimResults
}

func (s *streamingTranscriptionSession) shouldReturnUtterances() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg.ReturnUtterances != nil && *s.cfg.ReturnUtterances
}

func (s *streamingTranscriptionSession) writeBinary(payload []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()
	if conn == nil {
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_transport_error", "", "ByteDance streaming transcription connection is not available.")
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
		return translateTransportError(err, "ByteDance")
	}
	return nil
}

func (s *streamingTranscriptionSession) emit(event modality.StreamingTranscriptionServerEvent) {
	select {
	case s.events <- event:
	case <-s.closeCh:
	default:
	}
}

func (s *streamingTranscriptionSession) emitError(err error) {
	var apiErr *httputil.APIError
	if !errors.As(err, &apiErr) {
		apiErr = httputil.NewError(http.StatusBadGateway, "provider_error", "provider_error", "", err.Error())
	}
	s.emit(modality.StreamingTranscriptionServerEvent{
		Type:    modality.StreamingTranscriptionServerEventError,
		EventID: s.nextEventID(),
		Error: &modality.AudioError{
			Type:    apiErr.Type,
			Code:    apiErr.Code,
			Message: apiErr.Message,
			Param:   apiErr.Param,
		},
	})
}

func (s *streamingTranscriptionSession) nextEventID() string {
	value := atomic.AddUint64(&s.counter, 1)
	return fmt.Sprintf("evt_%06d", value)
}

func (s *streamingTranscriptionSession) signalStart(err error) {
	select {
	case s.startReady <- err:
	default:
	}
}

func isStreamingASRFinalFrame(flags byte) bool {
	switch flags & 0x3 {
	case streamingASRFlagLastPacket, streamingASRFlagSequenceLast:
		return true
	default:
		return false
	}
}
