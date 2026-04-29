package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/gin-gonic/gin"
)

type NotesHandler struct {
	runtime *gwruntime.Holder
}

func NewNotesHandler(runtime *gwruntime.Holder) *NotesHandler {
	return &NotesHandler{runtime: runtime}
}

func (h *NotesHandler) Create(c *gin.Context) {
	var req modality.AudioNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}
	if err := validateAudioNoteRequest(&req); err != nil {
		httputil.WriteError(c, err)
		return
	}

	registry, snapshot := h.runtimeDeps(c)
	if registry == nil || snapshot == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "model", "Model registry is unavailable."))
		return
	}
	auth := middleware.GetAuthContext(c)
	resolved, err := resolveEndpointModel(c, registry, auth, req.Model, req.Routing, modality.ModalityNotes, modality.CapabilityAudioNotes)
	if err != nil {
		writeModalityTargetError(c, err, "audio notes")
		return
	}
	applyResolvedRoutingHeaders(c, resolved)
	model := resolved.Model
	adapter, _, err := registry.GetAudioNotesAdapter(model.ID)
	if err != nil {
		writeModalityTargetError(c, err, "audio notes")
		return
	}

	req.Model = model.ID
	job, err := adapter.SubmitNotes(c.Request.Context(), &req)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	if job == nil || strings.TrimSpace(job.ID) == "" {
		httputil.WriteError(c, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Audio notes provider did not return a task id."))
		return
	}
	signedID, err := signAudioNoteID(snapshot, model, job.ID, auth.KeyID, time.Now().Add(audioNoteTTL).Unix())
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	job.ID = signedID
	job.Model = model.ID
	if strings.TrimSpace(job.Status) == "" {
		job.Status = modality.AudioNoteStatusQueued
	}
	c.JSON(http.StatusOK, job)
}

func (h *NotesHandler) Get(c *gin.Context) {
	registry, snapshot := h.runtimeDeps(c)
	if registry == nil || snapshot == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "model", "Model registry is unavailable."))
		return
	}
	token, _, adapter, err := h.resolveAuthorizedNote(c, registry, snapshot)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}

	job, err := adapter.GetAudioNote(c.Request.Context(), &modality.AudioNoteStatusRequest{
		Model:  token.Model,
		TaskID: token.ProviderTaskID,
	})
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	if job == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Audio notes provider returned an empty job status."))
		return
	}
	job.ID = c.Param("id")
	if strings.TrimSpace(job.Model) == "" {
		job.Model = token.Model
	}
	if strings.TrimSpace(job.Status) == "" {
		job.Status = modality.AudioNoteStatusQueued
	}
	if job.Result != nil && job.Result.Metadata == nil {
		job.Result.Metadata = map[string]any{}
	}
	c.JSON(http.StatusOK, job)
}

func (h *NotesHandler) Delete(c *gin.Context) {
	registry, snapshot := h.runtimeDeps(c)
	if registry == nil || snapshot == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "model", "Model registry is unavailable."))
		return
	}
	_, _, _, err := h.resolveAuthorizedNote(c, registry, snapshot)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "capability_not_supported", "notes_delete_not_supported", "id", "ByteDance does not expose note deletion through the current Polaris runtime."))
}

func (h *NotesHandler) runtimeDeps(c *gin.Context) (*provider.Registry, *gwruntime.Snapshot) {
	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	if snapshot == nil {
		return nil, nil
	}
	return snapshot.Registry, snapshot
}

func (h *NotesHandler) resolveAuthorizedNote(c *gin.Context, registry *provider.Registry, snapshot *gwruntime.Snapshot) (audioNoteToken, provider.Model, modality.AudioNotesAdapter, error) {
	token, err := parseAudioNoteID(snapshot, c.Param("id"))
	if err != nil {
		return audioNoteToken{}, provider.Model{}, nil, err
	}
	auth := middleware.GetAuthContext(c)
	if token.KeyID != "" && auth.KeyID != "" && token.KeyID != auth.KeyID {
		return audioNoteToken{}, provider.Model{}, nil, invalidAudioNoteIDError()
	}
	resolved, err := resolveEndpointModel(c, registry, auth, token.Model, nil, modality.ModalityNotes, modality.CapabilityAudioNotes)
	if err != nil {
		return audioNoteToken{}, provider.Model{}, nil, translateNoteTargetError(err)
	}
	model := resolved.Model
	adapter, _, err := registry.GetAudioNotesAdapter(model.ID)
	if err != nil {
		return audioNoteToken{}, provider.Model{}, nil, translateNoteTargetError(err)
	}
	return token, model, adapter, nil
}

func validateAudioNoteRequest(req *modality.AudioNoteRequest) error {
	if strings.TrimSpace(req.Model) == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_model", "model", "Field 'model' is required.")
	}
	if strings.TrimSpace(req.SourceURL) == "" {
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_source_url", "source_url", "Field 'source_url' is required.")
	}
	return nil
}

func translateNoteTargetError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, provider.ErrUnknownAlias):
		return httputil.NewError(http.StatusNotFound, "model_not_found", "unknown_alias", "model", "Model alias is not defined.")
	case errors.Is(err, provider.ErrUnknownModel):
		return httputil.NewError(http.StatusNotFound, "model_not_found", "unknown_model", "model", "Requested model is not registered.")
	case errors.Is(err, provider.ErrModalityMismatch):
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "modality_mismatch", "model", "Requested model does not support audio notes.")
	case errors.Is(err, provider.ErrCapabilityMissing):
		return httputil.NewError(http.StatusBadRequest, "capability_not_supported", "capability_missing", "model", "Requested model does not support audio notes.")
	case errors.Is(err, provider.ErrAdapterMissing):
		return httputil.NewError(http.StatusBadGateway, "provider_error", "adapter_unavailable", "model", "Requested model is configured but not available in this runtime build.")
	default:
		return err
	}
}
