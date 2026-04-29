package handler

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func (h *VoiceHandler) CreateStreamingTranscriptionSession(c *gin.Context) {
	var req modality.StreamingTranscriptionSessionConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}
	if err := validateStreamingTranscriptionRequest(&req); err != nil {
		httputil.WriteError(c, err)
		return
	}

	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	registry := h.registry(c)
	if snapshot == nil || registry == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "model", "Model registry is unavailable."))
		return
	}

	auth := middleware.GetAuthContext(c)
	resolved, err := resolveEndpointModel(c, registry, auth, req.Model, req.Routing, modality.ModalityVoice, modality.CapabilityStreaming)
	if err != nil {
		writeModalityTargetError(c, err, "streaming transcription")
		return
	}
	applyResolvedRoutingHeaders(c, resolved)
	model := resolved.Model

	adapter, _, err := registry.GetStreamingTranscriptionAdapter(model.ID)
	if err != nil {
		writeModalityTargetError(c, err, "streaming transcription")
		return
	}
	testSession, err := adapter.ConnectStreamingTranscription(c.Request.Context(), &req)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	_ = testSession.Close()

	req.Model = model.ID
	ttl := 10 * time.Minute
	if model.SessionTTL > 0 {
		ttl = time.Duration(model.SessionTTL) * time.Second
	}
	session, err := issueStreamingTranscriptionSession(snapshot, model, auth.KeyID, req, ttl)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	session.WebSocketURL = streamingTranscriptionWebSocketURL(c, session.ID)
	c.JSON(http.StatusOK, session)
}

func (h *VoiceHandler) StreamingTranscriptionWebSocket(c *gin.Context) {
	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	registry := h.registry(c)
	if snapshot == nil || registry == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "", "Model registry is unavailable."))
		return
	}

	clientSecret := bearerToken(c.GetHeader("Authorization"))
	if clientSecret == "" {
		httputil.WriteError(c, invalidStreamingTranscriptionSessionError("authorization", "Missing client secret."))
		return
	}

	issued, err := parseStreamingTranscriptionSession(snapshot, c.Param("id"), clientSecret)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	middleware.SetAuthContext(c, middleware.AuthContext{
		KeyID: issued.KeyID,
		Mode:  "streaming_transcription_session",
	})

	model, err := registry.RequireModel(issued.Model, modality.ModalityVoice, modality.CapabilityStreaming)
	if err != nil {
		writeModalityTargetError(c, err, "streaming transcription")
		return
	}
	adapter, _, err := registry.GetStreamingTranscriptionAdapter(issued.Model)
	if err != nil {
		writeModalityTargetError(c, err, "streaming transcription")
		return
	}
	session, err := adapter.ConnectStreamingTranscription(c.Request.Context(), &issued.Config)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}

	conn, err := audioUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		_ = session.Close()
		return
	}
	defer func() {
		_ = conn.Close()
	}()
	defer func() {
		_ = session.Close()
	}()

	var (
		writeMu    sync.Mutex
		outcomeMu  sync.Mutex
		done       = make(chan struct{})
		writerDone sync.WaitGroup
		outcome    = middleware.RequestOutcome{
			Model:           model.ID,
			Provider:        model.Provider,
			Modality:        modality.ModalityVoice,
			InterfaceFamily: "voice_streaming",
			StatusCode:      http.StatusSwitchingProtocols,
			TokenSource:     modality.TokenCountSourceUnavailable,
		}
	)

	writeEvent := func(event modality.StreamingTranscriptionServerEvent) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteJSON(event)
	}
	recordOutcomeError := func(errorType string) {
		outcomeMu.Lock()
		defer outcomeMu.Unlock()
		outcome.ErrorType = errorType
		outcome.StatusCode = http.StatusBadGateway
	}
	snapshotOutcome := func() middleware.RequestOutcome {
		outcomeMu.Lock()
		defer outcomeMu.Unlock()
		return outcome
	}

	writerDone.Add(1)
	go func() {
		defer writerDone.Done()
		events := session.Events()
		for {
			select {
			case <-done:
				return
			case event, ok := <-events:
				if !ok {
					return
				}
				if event.Error != nil && strings.TrimSpace(event.Error.Type) != "" {
					recordOutcomeError(event.Error.Type)
				}
				if err := writeEvent(event); err != nil {
					return
				}
			}
		}
	}()

	created := modality.StreamingTranscriptionServerEvent{
		Type:    modality.StreamingTranscriptionServerEventSessionCreated,
		EventID: "evt_000000",
		Session: &modality.StreamingTranscriptionSessionDescriptor{
			ID:           c.Param("id"),
			Object:       "audio.transcription.session",
			Model:        issued.Model,
			ExpiresAt:    issued.ExpiresAt,
			WebSocketURL: streamingTranscriptionWebSocketURL(c, c.Param("id")),
		},
	}
	if err := writeEvent(created); err != nil {
		close(done)
		writerDone.Wait()
		return
	}

	for {
		var event modality.StreamingTranscriptionClientEvent
		if err := conn.ReadJSON(&event); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				recordOutcomeError("provider_transport_error")
			}
			break
		}
		if err := session.Send(event); err != nil {
			var apiErr *httputil.APIError
			if !errors.As(err, &apiErr) {
				apiErr = httputil.NewError(http.StatusInternalServerError, "internal_error", "internal_error", "", "An internal error occurred.")
			}
			_ = writeEvent(modality.StreamingTranscriptionServerEvent{
				Type:    modality.StreamingTranscriptionServerEventError,
				EventID: "evt_error",
				Error: &modality.AudioError{
					Type:    apiErr.Type,
					Code:    apiErr.Code,
					Message: apiErr.Message,
					Param:   apiErr.Param,
				},
			})
			continue
		}
		if event.Type == modality.StreamingTranscriptionClientEventSessionClose {
			break
		}
	}

	close(done)
	writerDone.Wait()
	middleware.SetRequestOutcome(c, snapshotOutcome())
}

func validateStreamingTranscriptionRequest(req *modality.StreamingTranscriptionSessionConfig) error {
	if strings.TrimSpace(req.Model) == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_model", "model", "Field 'model' is required.")
	}
	if req.InputAudioFormat == "" {
		req.InputAudioFormat = modality.AudioFormatPCM16
	}
	if req.SampleRateHz == 0 {
		req.SampleRateHz = 16000
	}
	if req.SampleRateHz != 16000 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_sample_rate", "sample_rate_hz", "Only 16000 Hz streaming transcription sessions are supported.")
	}
	if req.InputAudioFormat != modality.AudioFormatPCM16 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_input_audio_format", "input_audio_format", "Only pcm16 input audio is supported.")
	}
	if req.InterimResults == nil {
		value := true
		req.InterimResults = &value
	}
	if req.ReturnUtterances == nil {
		value := true
		req.ReturnUtterances = &value
	}
	return nil
}

func streamingTranscriptionWebSocketURL(c *gin.Context, sessionID string) string {
	scheme := "ws"
	if forwarded := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")); strings.EqualFold(forwarded, "https") || strings.EqualFold(forwarded, "wss") {
		scheme = "wss"
	} else if c.Request.TLS != nil {
		scheme = "wss"
	}
	return scheme + "://" + c.Request.Host + "/v1/audio/transcriptions/stream/" + sessionID + "/ws"
}
