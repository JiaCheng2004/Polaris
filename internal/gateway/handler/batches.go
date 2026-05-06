package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	commonfiles "github.com/JiaCheng2004/Polaris/internal/provider/common/files"
	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/gin-gonic/gin"
)

const batchJobTTL = 7 * 24 * time.Hour

type BatchesHandler struct {
	runtime *gwruntime.Holder
	store   store.Store
}

func NewBatchesHandler(runtime *gwruntime.Holder, appStore store.Store) *BatchesHandler {
	return &BatchesHandler{runtime: runtime, store: appStore}
}

func (h *BatchesHandler) Create(c *gin.Context) {
	var req modality.BatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}
	if strings.TrimSpace(req.Model) == "" {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_model", "model", "Field 'model' is required."))
		return
	}
	if strings.TrimSpace(req.InputFileID) == "" {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_input_file_id", "input_file_id", "Field 'input_file_id' is required."))
		return
	}
	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	if snapshot == nil || snapshot.Registry == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "model", "Model registry is unavailable."))
		return
	}
	auth := middleware.GetAuthContext(c)
	if !middleware.ModalityScopeAllowed(auth.AllowedModalities, auth.PolicyModalities, modality.ModalityBatch) {
		httputil.WriteError(c, httputil.NewError(http.StatusForbidden, "permission_error", "modality_not_allowed", "batch", "API key is not permitted to use the batch modality."))
		return
	}
	resolution, err := snapshot.Registry.RequireResolvedModel(req.Model, modality.ModalityChat, req.Routing)
	if err != nil {
		writeChatTargetError(c, err)
		return
	}
	if !middleware.ScopeAllowed(auth.AllowedModels, auth.PolicyModels, resolution.Model.ID) {
		httputil.WriteError(c, httputil.NewError(http.StatusForbidden, "permission_error", "model_not_allowed", "model", "API key is not permitted to use this model."))
		return
	}
	adapter, err := snapshot.Registry.GetBatchAdapter(resolution.Model.Provider)
	if err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "capability_not_supported", "provider_lacks_batch_api", "model", "Selected provider does not expose a batch API."))
		return
	}
	providerReq := req
	providerReq.Model = resolution.Model.ID
	providerFileID, err := h.resolveBatchInputFile(c, snapshot, resolution.Model.Provider, req.InputFileID)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	providerReq.InputFileID = providerFileID
	job, err := adapter.Create(c.Request.Context(), &providerReq)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	publicID, err := signBatchJobID(snapshot, resolution.Model, job.ProviderJobID, auth.KeyID, time.Now().Add(batchJobTTL).Unix())
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	job.ID = publicID
	job.Model = resolution.Model.ID
	job.ProviderJobID = ""
	c.JSON(http.StatusOK, job)
}

func (h *BatchesHandler) Get(c *gin.Context) {
	token, adapter, modelID, err := h.adapterForToken(c)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	status, err := adapter.Get(c.Request.Context(), token.ProviderJobID)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	status.ID = c.Param("id")
	status.Model = modelID
	status.ProviderJobID = ""
	c.JSON(http.StatusOK, status)
}

func (h *BatchesHandler) Delete(c *gin.Context) {
	token, adapter, _, err := h.adapterForToken(c)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	if err := adapter.Cancel(c.Request.Context(), token.ProviderJobID); err != nil {
		httputil.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": c.Param("id"), "object": "batch", "cancelled": true})
}

func (h *BatchesHandler) Output(c *gin.Context) {
	token, adapter, _, err := h.adapterForToken(c)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	body, err := adapter.Output(c.Request.Context(), token.ProviderJobID)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	defer func() {
		_ = body.Close()
	}()
	c.DataFromReader(http.StatusOK, -1, "application/jsonl", body, nil)
}

func (h *BatchesHandler) adapterForToken(c *gin.Context) (batchJobToken, modality.BatchAdapter, string, error) {
	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	token, err := parseBatchJobID(snapshot, c.Param("id"))
	if err != nil {
		return batchJobToken{}, nil, "", err
	}
	auth := middleware.GetAuthContext(c)
	if token.KeyID != auth.KeyID {
		return batchJobToken{}, nil, "", invalidBatchJobIDError()
	}
	adapter, err := snapshot.Registry.GetBatchAdapter(token.Provider)
	if err != nil {
		return batchJobToken{}, nil, "", httputil.NewError(http.StatusBadRequest, "capability_not_supported", "provider_lacks_batch_api", "id", "Selected provider does not expose a batch API.")
	}
	return token, adapter, token.Model, nil
}

func (h *BatchesHandler) resolveBatchInputFile(c *gin.Context, snapshot *gwruntime.Snapshot, providerName string, inputFileID string) (string, error) {
	if !strings.HasPrefix(strings.TrimSpace(inputFileID), "pl_file_") {
		return strings.TrimSpace(inputFileID), nil
	}
	resolver := commonfiles.Resolver{
		Store:    h.store,
		Registry: snapshot.Registry,
		Config:   snapshot.Config.Files,
	}
	resolved, err := resolver.Resolve(c.Request.Context(), modality.FileSource{
		Kind:      modality.FileSourcePolarisRef,
		PolarisID: strings.TrimSpace(inputFileID),
	}, chatProjectID(middleware.GetAuthContext(c)), providerName)
	if err != nil {
		return "", err
	}
	if resolved.Kind != commonfiles.ResolvedProviderRef || strings.TrimSpace(resolved.ProviderID) == "" {
		return "", httputil.NewError(http.StatusBadRequest, "capability_not_supported", "provider_lacks_files_api", "input_file_id", "Batch input files must materialize to a provider file id.")
	}
	return resolved.ProviderID, nil
}
