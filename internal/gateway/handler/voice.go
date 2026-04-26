package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	cachepkg "github.com/JiaCheng2004/Polaris/internal/store/cache"
	"github.com/gin-gonic/gin"
)

type VoiceHandler struct {
	runtime *gwruntime.Holder
	cache   cachepkg.Cache
}

func NewVoiceHandler(runtime *gwruntime.Holder, cache cachepkg.Cache) *VoiceHandler {
	return &VoiceHandler{runtime: runtime, cache: cache}
}

func (h *VoiceHandler) Speech(c *gin.Context) {
	var req modality.TTSRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}
	if err := validateTTSRequest(&req); err != nil {
		httputil.WriteError(c, err)
		return
	}

	registry := h.registry(c)
	if registry == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "model", "Model registry is unavailable."))
		return
	}

	auth := middleware.GetAuthContext(c)
	resolved, err := resolveEndpointModel(c.Request.Context(), registry, auth, req.Model, req.Routing, modality.ModalityVoice, modality.CapabilityTTS)
	if err != nil {
		writeModalityTargetError(c, err, "audio speech")
		return
	}
	applyResolvedRoutingHeaders(c, resolved)
	model := resolved.Model
	if len(model.Voices) > 0 && !containsString(model.Voices, req.Voice) {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unknown_voice", "voice", "Requested voice is not supported by this model."))
		return
	}

	adapter, _, err := registry.GetVoiceAdapter(model.ID)
	if err != nil {
		writeModalityTargetError(c, err, "audio speech")
		return
	}

	cacheCtl := newResponseCache(c, h.runtime, h.cache)
	cacheKey := exactCacheKey("tts", model.ID, req)
	if cacheCtl != nil && cacheCtl.tryExact(c, cacheKey, model, modality.ModalityVoice) {
		return
	}
	if cacheCtl == nil {
		c.Header(cacheHeader, "bypass")
	}

	req.Model = model.ID
	response, err := adapter.TextToSpeech(c.Request.Context(), &req)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	middleware.SetRequestOutcome(c, middleware.RequestOutcome{
		Model:      model.ID,
		Provider:   model.Provider,
		Modality:   modality.ModalityVoice,
		StatusCode: http.StatusOK,
	})
	if cacheCtl != nil && response != nil {
		cacheCtl.storeRaw(c, cacheKey, http.StatusOK, response.ContentType, response.Data)
	}
	writeAudioResponse(c, req.ResponseFormat, response)
}

func (h *VoiceHandler) Transcribe(c *gin.Context) {
	req, err := parseSTTRequest(c)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	if err := validateSTTRequest(req); err != nil {
		httputil.WriteError(c, err)
		return
	}

	registry := h.registry(c)
	if registry == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "model", "Model registry is unavailable."))
		return
	}

	auth := middleware.GetAuthContext(c)
	resolved, err := resolveEndpointModel(c.Request.Context(), registry, auth, req.Model, req.Routing, modality.ModalityVoice, modality.CapabilitySTT)
	if err != nil {
		writeModalityTargetError(c, err, "audio transcription")
		return
	}
	applyResolvedRoutingHeaders(c, resolved)
	model := resolved.Model
	if len(model.Formats) > 0 {
		format := fileFormatFromName(req.Filename)
		if format == "" || !containsString(model.Formats, format) {
			httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_audio_format", "file", "Uploaded audio format is not supported by this model."))
			return
		}
	}

	adapter, _, err := registry.GetVoiceAdapter(model.ID)
	if err != nil {
		writeModalityTargetError(c, err, "audio transcription")
		return
	}

	cacheCtl := newResponseCache(c, h.runtime, h.cache)
	cacheKey := exactCacheKey("stt", model.ID, map[string]any{
		"file":            hashBytes(req.File),
		"language":        req.Language,
		"response_format": req.ResponseFormat,
		"temperature":     req.Temperature,
	})
	if cacheCtl != nil && cacheCtl.tryExact(c, cacheKey, model, modality.ModalityVoice) {
		return
	}
	if cacheCtl == nil {
		c.Header(cacheHeader, "bypass")
	}

	req.Model = model.ID
	response, err := adapter.SpeechToText(c.Request.Context(), req)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	middleware.SetRequestOutcome(c, middleware.RequestOutcome{
		Model:      model.ID,
		Provider:   model.Provider,
		Modality:   modality.ModalityVoice,
		StatusCode: http.StatusOK,
	})
	if cacheCtl != nil && response != nil {
		if req.ResponseFormat == "json" {
			cacheCtl.storeJSON(c, cacheKey, http.StatusOK, response)
		} else {
			body := response.Raw
			if len(body) == 0 {
				body = []byte(response.Text)
			}
			cacheCtl.storeRaw(c, cacheKey, http.StatusOK, response.ContentType, body)
		}
	}
	writeTranscriptResponse(c, req.ResponseFormat, response)
}

func (h *VoiceHandler) registry(c *gin.Context) *provider.Registry {
	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	if snapshot == nil {
		return nil
	}
	return snapshot.Registry
}

func parseSTTRequest(c *gin.Context) (*modality.STTRequest, error) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		if httputil.IsRequestBodyTooLarge(err) {
			return nil, httputil.RequestBodyTooLargeError(0)
		}
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_file", "file", "Form field 'file' is required.")
	}
	data, contentType, err := readMultipartFile(fileHeader)
	if err != nil {
		if httputil.IsRequestBodyTooLarge(err) {
			return nil, httputil.RequestBodyTooLargeError(0)
		}
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_file", "file", "Unable to read uploaded audio file.")
	}

	req := &modality.STTRequest{
		Model:          c.PostForm("model"),
		File:           data,
		Filename:       fileHeader.Filename,
		ContentType:    contentType,
		Language:       c.PostForm("language"),
		ResponseFormat: c.DefaultPostForm("response_format", "json"),
	}
	req.Routing, err = parseRoutingFormValue(c.PostForm("routing"))
	if err != nil {
		return nil, err
	}
	if tempValue := strings.TrimSpace(c.PostForm("temperature")); tempValue != "" {
		value, err := strconv.ParseFloat(tempValue, 64)
		if err != nil {
			return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_temperature", "temperature", "Field 'temperature' must be a number between 0 and 1.")
		}
		req.Temperature = &value
	}
	return req, nil
}

func validateTTSRequest(req *modality.TTSRequest) error {
	if strings.TrimSpace(req.Model) == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_model", "model", "Field 'model' is required.")
	}
	if strings.TrimSpace(req.Input) == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_input", "input", "Field 'input' is required.")
	}
	if strings.TrimSpace(req.Voice) == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_voice", "voice", "Field 'voice' is required.")
	}
	switch req.ResponseFormat {
	case "", "mp3", "opus", "aac", "flac", "wav", "pcm":
	default:
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_response_format", "response_format", "Field 'response_format' must be one of mp3, opus, aac, flac, wav, or pcm.")
	}
	if req.ResponseFormat == "" {
		req.ResponseFormat = "mp3"
	}
	if req.Speed != nil && (*req.Speed < 0.25 || *req.Speed > 4.0) {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_speed", "speed", "Field 'speed' must be between 0.25 and 4.0.")
	}
	return nil
}

func validateSTTRequest(req *modality.STTRequest) error {
	if strings.TrimSpace(req.Model) == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_model", "model", "Field 'model' is required.")
	}
	if len(req.File) == 0 {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_file", "file", "Form field 'file' is required.")
	}
	switch req.ResponseFormat {
	case "", "json", "text", "srt", "vtt":
	default:
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_response_format", "response_format", "Field 'response_format' must be one of json, text, srt, or vtt.")
	}
	if req.ResponseFormat == "" {
		req.ResponseFormat = "json"
	}
	if req.Temperature != nil && (*req.Temperature < 0 || *req.Temperature > 1) {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_temperature", "temperature", "Field 'temperature' must be between 0 and 1.")
	}
	return nil
}

func writeAudioResponse(c *gin.Context, requestedFormat string, response *modality.AudioResponse) {
	if response == nil {
		c.Data(http.StatusOK, audioContentType(requestedFormat), nil)
		return
	}
	contentType := audioContentType(requestedFormat)
	if response != nil && strings.TrimSpace(response.ContentType) != "" {
		contentType = response.ContentType
	}
	c.Data(http.StatusOK, contentType, response.Data)
}

func writeTranscriptResponse(c *gin.Context, requestedFormat string, response *modality.TranscriptResponse) {
	if requestedFormat == "" {
		requestedFormat = "json"
	}
	if response == nil {
		c.Status(http.StatusOK)
		return
	}
	switch requestedFormat {
	case "json":
		c.JSON(http.StatusOK, response)
	default:
		contentType := transcriptionContentType(requestedFormat)
		if strings.TrimSpace(response.ContentType) != "" {
			contentType = response.ContentType
		}
		body := response.Raw
		if len(body) == 0 {
			body = []byte(response.Text)
		}
		c.Data(http.StatusOK, contentType, body)
	}
}
