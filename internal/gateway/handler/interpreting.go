package handler

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type InterpretingHandler struct {
	runtime *gwruntime.Holder
}

func NewInterpretingHandler(runtime *gwruntime.Holder) *InterpretingHandler {
	return &InterpretingHandler{runtime: runtime}
}

func (h *InterpretingHandler) Create(c *gin.Context) {
	var req modality.InterpretingSessionConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}
	if err := validateInterpretingRequest(&req); err != nil {
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
	resolved, err := resolveEndpointModel(c.Request.Context(), registry, auth, req.Model, req.Routing, modality.ModalityInterpreting, modality.CapabilityAudioInput)
	if err != nil {
		writeModalityTargetError(c, err, "interpreting sessions")
		return
	}
	applyResolvedRoutingHeaders(c, resolved)
	model := resolved.Model
	if req.Mode == modality.InterpretingModeSpeechToSpeech && len(model.Voices) > 0 && strings.TrimSpace(req.Voice) == "" {
		req.Voice = model.Voices[0]
	}
	if strings.TrimSpace(req.Voice) != "" && len(model.Voices) > 0 && !containsString(model.Voices, req.Voice) {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unknown_voice", "voice", "Requested voice is not supported by this model."))
		return
	}

	adapter, _, err := registry.GetInterpretingAdapter(model.ID)
	if err != nil {
		writeModalityTargetError(c, err, "interpreting sessions")
		return
	}
	testSession, err := adapter.ConnectInterpreting(c.Request.Context(), &req)
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
	session, err := issueInterpretingSession(snapshot, model, auth.KeyID, req, ttl)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	session.WebSocketURL = interpretingWebSocketURL(c, session.ID)
	c.JSON(http.StatusOK, session)
}

func (h *InterpretingHandler) WebSocket(c *gin.Context) {
	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	registry := h.registry(c)
	if snapshot == nil || registry == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "", "Model registry is unavailable."))
		return
	}

	clientSecret := bearerToken(c.GetHeader("Authorization"))
	if clientSecret == "" {
		httputil.WriteError(c, invalidInterpretingSessionError("authorization", "Missing client secret."))
		return
	}

	issued, err := parseInterpretingSession(snapshot, c.Param("id"), clientSecret)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	middleware.SetAuthContext(c, middleware.AuthContext{
		KeyID: issued.KeyID,
		Mode:  "interpreting_session",
	})

	model, err := registry.RequireModel(issued.Model, modality.ModalityInterpreting)
	if err != nil {
		writeModalityTargetError(c, err, "interpreting sessions")
		return
	}
	adapter, _, err := registry.GetInterpretingAdapter(issued.Model)
	if err != nil {
		writeModalityTargetError(c, err, "interpreting sessions")
		return
	}
	session, err := adapter.ConnectInterpreting(c.Request.Context(), &issued.Config)
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
		writeMu      sync.Mutex
		done         = make(chan struct{})
		writerDone   sync.WaitGroup
		outcome      = middleware.RequestOutcome{Model: model.ID, Provider: model.Provider, Modality: modality.ModalityInterpreting, InterfaceFamily: "audio_interpreting", StatusCode: http.StatusSwitchingProtocols, TokenSource: modality.TokenCountSourceUnavailable}
		usage        modality.InterpretingUsage
		lastErrorTyp string
	)

	writeEvent := func(event modality.InterpretingServerEvent) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteJSON(event)
	}

	writerDone.Add(1)
	go func() {
		defer writerDone.Done()
		events := session.Events()
		for {
			select {
			case <-done:
				return
			case event := <-events:
				if event.Usage != nil {
					if event.Usage.InputAudioSeconds > 0 {
						usage.InputAudioSeconds = event.Usage.InputAudioSeconds
					}
					if event.Usage.OutputAudioSeconds > 0 {
						usage.OutputAudioSeconds = event.Usage.OutputAudioSeconds
					}
					if event.Usage.TotalTokens > 0 {
						usage.TotalTokens = event.Usage.TotalTokens
						usage.Source = event.Usage.Source
					}
				}
				if event.Error != nil && strings.TrimSpace(event.Error.Type) != "" {
					lastErrorTyp = event.Error.Type
					outcome.StatusCode = http.StatusBadGateway
				}
				if err := writeEvent(event); err != nil {
					return
				}
			}
		}
	}()

	created := modality.InterpretingServerEvent{
		Type:    modality.InterpretingServerEventSessionCreated,
		EventID: "evt_000000",
		Session: &modality.InterpretingSessionDescriptor{
			ID:           c.Param("id"),
			Object:       "audio.interpreting.session",
			Model:        issued.Model,
			ExpiresAt:    issued.ExpiresAt,
			WebSocketURL: interpretingWebSocketURL(c, c.Param("id")),
		},
	}
	if err := writeEvent(created); err != nil {
		close(done)
		writerDone.Wait()
		return
	}

	for {
		var event modality.InterpretingClientEvent
		if err := conn.ReadJSON(&event); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				lastErrorTyp = "provider_transport_error"
				outcome.StatusCode = http.StatusBadGateway
			}
			break
		}
		if err := session.Send(event); err != nil {
			var apiErr *httputil.APIError
			if !errors.As(err, &apiErr) {
				apiErr = httputil.NewError(http.StatusInternalServerError, "internal_error", "internal_error", "", "An internal error occurred.")
			}
			_ = writeEvent(modality.InterpretingServerEvent{
				Type:    modality.InterpretingServerEventError,
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
		if event.Type == modality.InterpretingClientEventSessionClose {
			break
		}
	}

	close(done)
	writerDone.Wait()
	outcome.ErrorType = lastErrorTyp
	outcome.TotalTokens = usage.TotalTokens
	outcome.TokenSource = countsTokenSource(0, 0, usage.TotalTokens)
	middleware.SetRequestOutcome(c, outcome)
}

func (h *InterpretingHandler) registry(c *gin.Context) *provider.Registry {
	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	if snapshot == nil {
		return nil
	}
	return snapshot.Registry
}

func validateInterpretingRequest(req *modality.InterpretingSessionConfig) error {
	if strings.TrimSpace(req.Model) == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_model", "model", "Field 'model' is required.")
	}
	switch strings.TrimSpace(req.Mode) {
	case "", modality.InterpretingModeSpeechToSpeech, modality.InterpretingModeSpeechToText:
	default:
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_mode", "mode", "Supported interpreting modes are speech_to_speech and speech_to_text.")
	}
	if strings.TrimSpace(req.SourceLanguage) == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_source_language", "source_language", "Field 'source_language' is required.")
	}
	if strings.TrimSpace(req.TargetLanguage) == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_target_language", "target_language", "Field 'target_language' is required.")
	}
	if req.InputAudioFormat == "" {
		req.InputAudioFormat = modality.InterpretingAudioFormatPCM16
	}
	switch req.InputAudioFormat {
	case modality.InterpretingAudioFormatPCM16, modality.InterpretingAudioFormatWAV:
	default:
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_input_audio_format", "input_audio_format", "Supported input audio formats are pcm16 and wav.")
	}
	if req.InputSampleRateHz == 0 {
		req.InputSampleRateHz = 16000
	}
	if req.InputSampleRateHz != 16000 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_input_sample_rate", "input_sample_rate_hz", "ByteDance simultaneous interpretation input must be 16000 Hz.")
	}
	if strings.TrimSpace(req.Mode) == modality.InterpretingModeSpeechToSpeech || strings.TrimSpace(req.Mode) == "" {
		if req.OutputAudioFormat == "" {
			req.OutputAudioFormat = modality.InterpretingAudioFormatOpus
		}
		switch req.OutputAudioFormat {
		case modality.InterpretingAudioFormatPCM16, modality.InterpretingAudioFormatOpus:
		default:
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_output_audio_format", "output_audio_format", "Supported output audio formats are pcm16 and ogg_opus.")
		}
		if req.OutputSampleRateHz == 0 {
			if req.OutputAudioFormat == modality.InterpretingAudioFormatPCM16 {
				req.OutputSampleRateHz = 16000
			} else {
				req.OutputSampleRateHz = 48000
			}
		}
		if req.OutputAudioFormat == modality.InterpretingAudioFormatPCM16 && req.OutputSampleRateHz != 16000 {
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_output_sample_rate", "output_sample_rate_hz", "PCM16 interpreting output only supports 16000 Hz.")
		}
		if req.OutputAudioFormat == modality.InterpretingAudioFormatOpus && req.OutputSampleRateHz != 48000 {
			return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_output_sample_rate", "output_sample_rate_hz", "Ogg Opus interpreting output only supports 48000 Hz.")
		}
	} else {
		req.OutputAudioFormat = ""
		req.OutputSampleRateHz = 0
		req.Voice = ""
	}
	return nil
}

func interpretingWebSocketURL(c *gin.Context, sessionID string) string {
	scheme := "ws"
	if forwarded := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")); strings.EqualFold(forwarded, "https") || strings.EqualFold(forwarded, "wss") {
		scheme = "wss"
	} else if c.Request.TLS != nil {
		scheme = "wss"
	}
	return scheme + "://" + c.Request.Host + "/v1/audio/interpreting/sessions/" + sessionID + "/ws"
}
