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

var audioUpgrader = websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool { return true },
}

type AudioHandler struct {
	runtime *gwruntime.Holder
}

func NewAudioHandler(runtime *gwruntime.Holder) *AudioHandler {
	return &AudioHandler{runtime: runtime}
}

func (h *AudioHandler) Create(c *gin.Context) {
	var req modality.AudioSessionConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}
	if err := validateAudioSessionRequest(&req); err != nil {
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
	resolved, err := resolveEndpointModel(c.Request.Context(), registry, auth, req.Model, req.Routing, modality.ModalityAudio, modality.CapabilityAudioInput, modality.CapabilityAudioOutput)
	if err != nil {
		writeModalityTargetError(c, err, "audio sessions")
		return
	}
	applyResolvedRoutingHeaders(c, resolved)
	model := resolved.Model
	if len(model.Voices) > 0 && strings.TrimSpace(req.Voice) == "" {
		req.Voice = model.Voices[0]
	}
	if strings.TrimSpace(req.Voice) != "" && len(model.Voices) > 0 && !containsString(model.Voices, req.Voice) {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unknown_voice", "voice", "Requested voice is not supported by this model."))
		return
	}

	adapter, _, err := registry.GetAudioAdapter(model.ID)
	if err != nil {
		writeModalityTargetError(c, err, "audio sessions")
		return
	}
	testSession, err := adapter.Connect(c.Request.Context(), &req)
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
	session, err := issueAudioSession(snapshot, model, auth.KeyID, req, ttl)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	session.WebSocketURL = audioWebSocketURL(c, session.ID)
	c.JSON(http.StatusOK, session)
}

func (h *AudioHandler) WebSocket(c *gin.Context) {
	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	registry := h.registry(c)
	if snapshot == nil || registry == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "", "Model registry is unavailable."))
		return
	}

	clientSecret := bearerToken(c.GetHeader("Authorization"))
	if clientSecret == "" {
		httputil.WriteError(c, invalidAudioSessionError("authorization", "Missing client secret."))
		return
	}

	issued, err := parseAudioSession(snapshot, c.Param("id"), clientSecret)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	middleware.SetAuthContext(c, middleware.AuthContext{
		KeyID: issued.KeyID,
		Mode:  "audio_session",
	})

	model, err := registry.RequireModel(issued.Model, modality.ModalityAudio)
	if err != nil {
		writeModalityTargetError(c, err, "audio sessions")
		return
	}
	adapter, _, err := registry.GetAudioAdapter(issued.Model)
	if err != nil {
		writeModalityTargetError(c, err, "audio sessions")
		return
	}
	session, err := adapter.Connect(c.Request.Context(), &issued.Config)
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
		outcome      = middleware.RequestOutcome{Model: model.ID, Provider: model.Provider, Modality: modality.ModalityAudio, StatusCode: http.StatusSwitchingProtocols}
		usage        modality.AudioUsage
		lastErrorTyp string
	)

	writeEvent := func(event modality.AudioServerEvent) error {
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
			case event, ok := <-events:
				if !ok {
					return
				}
				if event.Usage != nil {
					normalizedUsage := normalizeAudioUsage(*event.Usage)
					event.Usage = &normalizedUsage
					usage.InputAudioSeconds += event.Usage.InputAudioSeconds
					usage.OutputAudioSeconds += event.Usage.OutputAudioSeconds
					usage.InputTextTokens += event.Usage.InputTextTokens
					usage.OutputTextTokens += event.Usage.OutputTextTokens
					usage.TotalTokens += event.Usage.TotalTokens
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

	created := modality.AudioServerEvent{
		Type:    modality.AudioServerEventSessionCreated,
		EventID: "evt_000000",
		Session: &modality.AudioSessionDescriptor{
			ID:           c.Param("id"),
			Object:       "audio.session",
			Model:        issued.Model,
			ExpiresAt:    issued.ExpiresAt,
			WebSocketURL: audioWebSocketURL(c, c.Param("id")),
		},
	}
	if err := writeEvent(created); err != nil {
		close(done)
		writerDone.Wait()
		return
	}

	for {
		var event modality.AudioClientEvent
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
			_ = writeEvent(modality.AudioServerEvent{
				Type:    modality.AudioServerEventError,
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
		if event.Type == modality.AudioClientEventSessionClose {
			break
		}
	}

	close(done)
	writerDone.Wait()
	outcome.ErrorType = lastErrorTyp
	outcome.PromptTokens = usage.InputTextTokens
	outcome.CompletionTokens = usage.OutputTextTokens
	outcome.TotalTokens = usage.TotalTokens
	outcome.TokenSource = countsTokenSource(usage.InputTextTokens, usage.OutputTextTokens, usage.TotalTokens)
	middleware.SetRequestOutcome(c, outcome)
}

func (h *AudioHandler) registry(c *gin.Context) *provider.Registry {
	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	if snapshot == nil {
		return nil
	}
	return snapshot.Registry
}

func validateAudioSessionRequest(req *modality.AudioSessionConfig) error {
	if strings.TrimSpace(req.Model) == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_model", "model", "Field 'model' is required.")
	}
	if req.InputAudioFormat == "" {
		req.InputAudioFormat = modality.AudioFormatPCM16
	}
	if req.OutputAudioFormat == "" {
		req.OutputAudioFormat = modality.AudioFormatPCM16
	}
	if req.SampleRateHz == 0 {
		req.SampleRateHz = 16000
	}
	if req.SampleRateHz != 16000 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_sample_rate", "sample_rate_hz", "Only 16000 Hz audio sessions are supported.")
	}
	if req.InputAudioFormat != modality.AudioFormatPCM16 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_input_audio_format", "input_audio_format", "Only pcm16 input audio is supported.")
	}
	if req.OutputAudioFormat != modality.AudioFormatPCM16 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_output_audio_format", "output_audio_format", "Only pcm16 output audio is supported.")
	}
	if req.TurnDetection == nil {
		req.TurnDetection = &modality.TurnDetectionConfig{Mode: modality.TurnDetectionManual}
	}
	if strings.TrimSpace(req.TurnDetection.Mode) == "" {
		req.TurnDetection.Mode = modality.TurnDetectionManual
	}
	return nil
}

func audioWebSocketURL(c *gin.Context, sessionID string) string {
	scheme := "ws"
	if forwarded := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")); strings.EqualFold(forwarded, "https") || strings.EqualFold(forwarded, "wss") {
		scheme = "wss"
	} else if c.Request.TLS != nil {
		scheme = "wss"
	}
	return scheme + "://" + c.Request.Host + "/v1/audio/sessions/" + sessionID + "/ws"
}

func bearerToken(header string) string {
	parts := strings.SplitN(strings.TrimSpace(header), " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
