package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/JiaCheng2004/Polaris/internal/tooling"
	"github.com/gin-gonic/gin"
)

type ControlPlaneHandler struct {
	runtime     *gwruntime.Holder
	store       store.Store
	keyCache    *middleware.VirtualKeyCache
	auditLogger *store.AsyncAuditLogger
	tools       *tooling.Registry
}

func NewControlPlaneHandler(runtime *gwruntime.Holder, appStore store.Store, keyCache *middleware.VirtualKeyCache, auditLogger *store.AsyncAuditLogger, tools *tooling.Registry) *ControlPlaneHandler {
	return &ControlPlaneHandler{
		runtime:     runtime,
		store:       appStore,
		keyCache:    keyCache,
		auditLogger: auditLogger,
		tools:       tools,
	}
}

type createProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type projectResponse struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	ArchivedAt  *time.Time `json:"archived_at,omitempty"`
}

func (h *ControlPlaneHandler) CreateProject(c *gin.Context) {
	var req createProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_name", "name", "Field 'name' is required."))
		return
	}

	projectID, err := newKeyID()
	if err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusInternalServerError, "internal_error", "id_generation_failed", "", "Unable to generate project identifier."))
		return
	}

	project := store.Project{
		ID:          "proj_" + projectID,
		Name:        req.Name,
		Description: strings.TrimSpace(req.Description),
		CreatedAt:   time.Now().UTC(),
	}
	if err := h.store.CreateProject(c.Request.Context(), project); err != nil {
		httputil.WriteError(c, err)
		return
	}
	h.logAudit(c, "project.created", "project", project.ID, map[string]any{"name": project.Name})
	c.JSON(http.StatusOK, projectResponse{
		ID:          project.ID,
		Name:        project.Name,
		Description: project.Description,
		CreatedAt:   project.CreatedAt,
	})
}

func (h *ControlPlaneHandler) ListProjects(c *gin.Context) {
	includeArchived, err := parseBoolQuery(c, "include_archived")
	if err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_include_archived", "include_archived", "Query parameter 'include_archived' must be a boolean."))
		return
	}
	projects, err := h.store.ListProjects(c.Request.Context(), includeArchived)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	data := make([]projectResponse, 0, len(projects))
	for _, project := range projects {
		data = append(data, projectResponse{
			ID:          project.ID,
			Name:        project.Name,
			Description: project.Description,
			CreatedAt:   project.CreatedAt,
			ArchivedAt:  project.ArchivedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": data})
}

type createVirtualKeyRequest struct {
	ProjectID         string   `json:"project_id"`
	OwnerID           string   `json:"owner_id"`
	Name              string   `json:"name"`
	RateLimit         string   `json:"rate_limit"`
	AllowedModels     []string `json:"allowed_models"`
	AllowedModalities []string `json:"allowed_modalities"`
	AllowedToolsets   []string `json:"allowed_toolsets"`
	AllowedMCP        []string `json:"allowed_mcp_bindings"`
	IsAdmin           bool     `json:"is_admin"`
	ExpiresAt         string   `json:"expires_at"`
}

type virtualKeyResponse struct {
	ID                string     `json:"id"`
	ProjectID         string     `json:"project_id,omitempty"`
	Name              string     `json:"name"`
	Key               string     `json:"key,omitempty"`
	KeyPrefix         string     `json:"key_prefix"`
	RateLimit         string     `json:"rate_limit,omitempty"`
	AllowedModels     []string   `json:"allowed_models"`
	AllowedModalities []string   `json:"allowed_modalities,omitempty"`
	AllowedToolsets   []string   `json:"allowed_toolsets,omitempty"`
	AllowedMCP        []string   `json:"allowed_mcp_bindings,omitempty"`
	IsAdmin           bool       `json:"is_admin"`
	CreatedAt         time.Time  `json:"created_at"`
	LastUsedAt        *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
	IsRevoked         bool       `json:"is_revoked,omitempty"`
}

func (h *ControlPlaneHandler) CreateVirtualKey(c *gin.Context) {
	req, rawKey, key, ok := h.bindVirtualKeyCreate(c, false)
	if !ok {
		return
	}
	if err := h.store.CreateVirtualKey(c.Request.Context(), key); err != nil {
		httputil.WriteError(c, err)
		return
	}
	if h.keyCache != nil {
		h.keyCache.Clear()
	}
	h.logAudit(c, "virtual_key.created", "virtual_key", key.ID, map[string]any{"project_id": req.ProjectID, "name": key.Name})
	c.JSON(http.StatusOK, virtualKeyToResponse(key, rawKey))
}

func (h *ControlPlaneHandler) ListVirtualKeys(c *gin.Context) {
	includeRevoked, err := parseBoolQuery(c, "include_revoked")
	if err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_include_revoked", "include_revoked", "Query parameter 'include_revoked' must be a boolean."))
		return
	}
	keys, err := h.store.ListVirtualKeys(c.Request.Context(), strings.TrimSpace(c.Query("project_id")), includeRevoked)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	data := make([]virtualKeyResponse, 0, len(keys))
	for _, key := range keys {
		data = append(data, virtualKeyToResponse(key, ""))
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": data})
}

func (h *ControlPlaneHandler) DeleteVirtualKey(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_key_id", "id", "Key id is required."))
		return
	}
	if err := h.store.DeleteVirtualKey(c.Request.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httputil.WriteError(c, httputil.NewError(http.StatusNotFound, "invalid_request_error", "key_not_found", "id", "API key was not found."))
			return
		}
		httputil.WriteError(c, err)
		return
	}
	if h.keyCache != nil {
		h.keyCache.Clear()
	}
	h.logAudit(c, "virtual_key.revoked", "virtual_key", id, nil)
	c.Status(http.StatusNoContent)
}

type createPolicyRequest struct {
	ProjectID         string   `json:"project_id"`
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	AllowedModels     []string `json:"allowed_models"`
	AllowedModalities []string `json:"allowed_modalities"`
	AllowedToolsets   []string `json:"allowed_toolsets"`
	AllowedMCP        []string `json:"allowed_mcp_bindings"`
}

func (h *ControlPlaneHandler) CreatePolicy(c *gin.Context) {
	var req createPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}
	req.ProjectID = strings.TrimSpace(req.ProjectID)
	req.Name = strings.TrimSpace(req.Name)
	if req.ProjectID == "" || req.Name == "" {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_fields", "", "Fields 'project_id' and 'name' are required."))
		return
	}
	policyID, err := newKeyID()
	if err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusInternalServerError, "internal_error", "id_generation_failed", "", "Unable to generate policy identifier."))
		return
	}

	policy := store.Policy{
		ID:                "pol_" + policyID,
		ProjectID:         req.ProjectID,
		Name:              req.Name,
		Description:       strings.TrimSpace(req.Description),
		AllowedModels:     defaultPatterns(req.AllowedModels),
		AllowedModalities: parseAllowedModalities(req.AllowedModalities),
		AllowedToolsets:   normalizeStringList(req.AllowedToolsets),
		AllowedMCP:        normalizeStringList(req.AllowedMCP),
		CreatedAt:         time.Now().UTC(),
	}
	if err := h.store.CreatePolicy(c.Request.Context(), policy); err != nil {
		httputil.WriteError(c, err)
		return
	}
	h.logAudit(c, "policy.created", "policy", policy.ID, map[string]any{"project_id": policy.ProjectID})
	c.JSON(http.StatusOK, policy)
}

func (h *ControlPlaneHandler) ListPolicies(c *gin.Context) {
	policies, err := h.store.ListPolicies(c.Request.Context(), strings.TrimSpace(c.Query("project_id")))
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": policies})
}

type createBudgetRequest struct {
	ProjectID     string  `json:"project_id"`
	Name          string  `json:"name"`
	Mode          string  `json:"mode"`
	LimitUSD      float64 `json:"limit_usd"`
	LimitRequests int64   `json:"limit_requests"`
	Window        string  `json:"window"`
}

func (h *ControlPlaneHandler) CreateBudget(c *gin.Context) {
	var req createBudgetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}
	req.ProjectID = strings.TrimSpace(req.ProjectID)
	req.Name = strings.TrimSpace(req.Name)
	mode := store.BudgetMode(strings.TrimSpace(req.Mode))
	if req.ProjectID == "" || req.Name == "" || !mode.Valid() {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_budget", "", "Fields 'project_id', 'name', and a valid 'mode' are required."))
		return
	}
	budgetID, err := newKeyID()
	if err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusInternalServerError, "internal_error", "id_generation_failed", "", "Unable to generate budget identifier."))
		return
	}

	budget := store.Budget{
		ID:            "bud_" + budgetID,
		ProjectID:     req.ProjectID,
		Name:          req.Name,
		Mode:          mode,
		LimitUSD:      req.LimitUSD,
		LimitRequests: req.LimitRequests,
		Window:        strings.TrimSpace(req.Window),
		CreatedAt:     time.Now().UTC(),
	}
	if budget.Window == "" {
		budget.Window = "monthly"
	}
	if err := h.store.CreateBudget(c.Request.Context(), budget); err != nil {
		httputil.WriteError(c, err)
		return
	}
	h.logAudit(c, "budget.created", "budget", budget.ID, map[string]any{"project_id": budget.ProjectID, "mode": budget.Mode})
	c.JSON(http.StatusOK, budget)
}

func (h *ControlPlaneHandler) ListBudgets(c *gin.Context) {
	budgets, err := h.store.ListBudgets(c.Request.Context(), strings.TrimSpace(c.Query("project_id")))
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": budgets})
}

type createToolRequest struct {
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	Implementation string          `json:"implementation"`
	InputSchema    json.RawMessage `json:"input_schema"`
	Enabled        *bool           `json:"enabled"`
}

func (h *ControlPlaneHandler) CreateTool(c *gin.Context) {
	var req createToolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Implementation = strings.TrimSpace(req.Implementation)
	if req.Name == "" || req.Implementation == "" {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_fields", "", "Fields 'name' and 'implementation' are required."))
		return
	}
	if h.tools != nil && !h.tools.Has(req.Implementation) {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unknown_tool_implementation", "implementation", "Tool implementation is not registered in this Polaris runtime."))
		return
	}
	inputSchema := strings.TrimSpace(string(req.InputSchema))
	if inputSchema == "" && h.tools != nil {
		inputSchema = h.tools.Schema(req.Implementation)
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	toolID, err := newKeyID()
	if err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusInternalServerError, "internal_error", "id_generation_failed", "", "Unable to generate tool identifier."))
		return
	}

	tool := store.ToolDefinition{
		ID:             "tool_" + toolID,
		Name:           req.Name,
		Description:    strings.TrimSpace(req.Description),
		Implementation: req.Implementation,
		InputSchema:    inputSchema,
		Enabled:        enabled,
		CreatedAt:      time.Now().UTC(),
	}
	if err := h.store.CreateToolDefinition(c.Request.Context(), tool); err != nil {
		httputil.WriteError(c, err)
		return
	}
	h.logAudit(c, "tool.created", "tool", tool.ID, map[string]any{"name": tool.Name, "implementation": tool.Implementation})
	c.JSON(http.StatusOK, tool)
}

func (h *ControlPlaneHandler) ListTools(c *gin.Context) {
	tools, err := h.store.ListToolDefinitions(c.Request.Context())
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": tools})
}

type createToolsetRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	ToolIDs     []string `json:"tool_ids"`
}

func (h *ControlPlaneHandler) CreateToolset(c *gin.Context) {
	var req createToolsetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.ToolIDs) == 0 {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_toolset", "", "Fields 'name' and at least one 'tool_id' are required."))
		return
	}
	for _, toolID := range req.ToolIDs {
		if _, err := h.store.GetToolDefinition(c.Request.Context(), strings.TrimSpace(toolID)); err != nil {
			httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unknown_tool", "tool_ids", "Toolset references an unknown tool definition."))
			return
		}
	}
	toolsetID, err := newKeyID()
	if err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusInternalServerError, "internal_error", "id_generation_failed", "", "Unable to generate toolset identifier."))
		return
	}

	toolset := store.Toolset{
		ID:          "ts_" + toolsetID,
		Name:        req.Name,
		Description: strings.TrimSpace(req.Description),
		ToolIDs:     normalizeStringList(req.ToolIDs),
		CreatedAt:   time.Now().UTC(),
	}
	if err := h.store.CreateToolset(c.Request.Context(), toolset); err != nil {
		httputil.WriteError(c, err)
		return
	}
	h.logAudit(c, "toolset.created", "toolset", toolset.ID, map[string]any{"name": toolset.Name})
	c.JSON(http.StatusOK, toolset)
}

func (h *ControlPlaneHandler) ListToolsets(c *gin.Context) {
	toolsets, err := h.store.ListToolsets(c.Request.Context())
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": toolsets})
}

type createBindingRequest struct {
	Name        string            `json:"name"`
	Kind        string            `json:"kind"`
	UpstreamURL string            `json:"upstream_url"`
	ToolsetID   string            `json:"toolset_id"`
	Headers     map[string]string `json:"headers"`
	Enabled     *bool             `json:"enabled"`
}

func (h *ControlPlaneHandler) CreateMCPBinding(c *gin.Context) {
	var req createBindingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	kind := store.MCPBindingKind(strings.TrimSpace(req.Kind))
	if req.Name == "" || !kind.Valid() {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_binding", "", "Fields 'name' and valid 'kind' are required."))
		return
	}
	if kind == store.MCPBindingKindUpstreamProxy && strings.TrimSpace(req.UpstreamURL) == "" {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_upstream_url", "upstream_url", "Field 'upstream_url' is required for upstream_proxy bindings."))
		return
	}
	if kind == store.MCPBindingKindLocalToolset {
		req.ToolsetID = strings.TrimSpace(req.ToolsetID)
		if req.ToolsetID == "" {
			httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_toolset_id", "toolset_id", "Field 'toolset_id' is required for local_toolset bindings."))
			return
		}
		if _, err := h.store.GetToolset(c.Request.Context(), req.ToolsetID); err != nil {
			httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unknown_toolset", "toolset_id", "Referenced toolset was not found."))
			return
		}
	}
	headersJSON, _ := json.Marshal(req.Headers)
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	bindingID, err := newKeyID()
	if err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusInternalServerError, "internal_error", "id_generation_failed", "", "Unable to generate MCP binding identifier."))
		return
	}

	binding := store.MCPBinding{
		ID:          "mcp_" + bindingID,
		Name:        req.Name,
		Kind:        kind,
		UpstreamURL: strings.TrimSpace(req.UpstreamURL),
		ToolsetID:   strings.TrimSpace(req.ToolsetID),
		HeadersJSON: string(headersJSON),
		Enabled:     enabled,
		CreatedAt:   time.Now().UTC(),
	}
	if err := h.store.CreateMCPBinding(c.Request.Context(), binding); err != nil {
		httputil.WriteError(c, err)
		return
	}
	h.logAudit(c, "mcp_binding.created", "mcp_binding", binding.ID, map[string]any{"kind": binding.Kind, "name": binding.Name})
	c.JSON(http.StatusOK, binding)
}

func (h *ControlPlaneHandler) ListMCPBindings(c *gin.Context) {
	bindings, err := h.store.ListMCPBindings(c.Request.Context())
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": bindings})
}

func (h *ControlPlaneHandler) CreateLegacyKey(c *gin.Context) {
	req, rawKey, key, ok := h.bindVirtualKeyCreate(c, true)
	if !ok {
		return
	}
	if err := h.store.CreateVirtualKey(c.Request.Context(), key); err != nil {
		httputil.WriteError(c, err)
		return
	}
	if h.keyCache != nil {
		h.keyCache.Clear()
	}
	h.logAudit(c, "virtual_key.created", "virtual_key", key.ID, map[string]any{"project_id": req.ProjectID, "name": key.Name, "compatibility": true})
	response := virtualKeyToResponse(key, rawKey)
	c.JSON(http.StatusOK, gin.H{
		"id":             response.ID,
		"name":           response.Name,
		"key":            response.Key,
		"key_prefix":     response.KeyPrefix,
		"owner_id":       response.ProjectID,
		"rate_limit":     response.RateLimit,
		"allowed_models": response.AllowedModels,
		"is_admin":       response.IsAdmin,
		"created_at":     response.CreatedAt,
		"expires_at":     response.ExpiresAt,
	})
}

func (h *ControlPlaneHandler) ListLegacyKeys(c *gin.Context) {
	includeRevoked, err := parseBoolQuery(c, "include_revoked")
	if err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_include_revoked", "include_revoked", "Query parameter 'include_revoked' must be a boolean."))
		return
	}
	projectID := strings.TrimSpace(c.Query("project_id"))
	if projectID == "" {
		projectID = strings.TrimSpace(c.Query("owner_id"))
	}
	keys, err := h.store.ListVirtualKeys(c.Request.Context(), projectID, includeRevoked)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	data := make([]gin.H, 0, len(keys))
	for _, key := range keys {
		resp := virtualKeyToResponse(key, "")
		data = append(data, gin.H{
			"id":             resp.ID,
			"name":           resp.Name,
			"key_prefix":     resp.KeyPrefix,
			"owner_id":       resp.ProjectID,
			"rate_limit":     resp.RateLimit,
			"allowed_models": resp.AllowedModels,
			"is_admin":       resp.IsAdmin,
			"created_at":     resp.CreatedAt,
			"last_used_at":   resp.LastUsedAt,
			"expires_at":     resp.ExpiresAt,
			"is_revoked":     resp.IsRevoked,
		})
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": data})
}

func (h *ControlPlaneHandler) DeleteLegacyKey(c *gin.Context) {
	h.DeleteVirtualKey(c)
}

func (h *ControlPlaneHandler) bindVirtualKeyCreate(c *gin.Context, compatibility bool) (createVirtualKeyRequest, string, store.VirtualKey, bool) {
	var req createVirtualKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return createVirtualKeyRequest{}, "", store.VirtualKey{}, false
	}
	if compatibility && strings.TrimSpace(req.ProjectID) == "" {
		req.ProjectID = strings.TrimSpace(c.Query("project_id"))
	}
	if compatibility && strings.TrimSpace(req.ProjectID) == "" {
		req.ProjectID = strings.TrimSpace(req.OwnerID)
	}
	req.ProjectID = strings.TrimSpace(req.ProjectID)
	req.Name = strings.TrimSpace(req.Name)
	if compatibility && req.ProjectID == "" {
		req.ProjectID = "legacy-default"
	}
	if req.ProjectID == "" || req.Name == "" {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_fields", "", "Fields 'project_id' and 'name' are required."))
		return createVirtualKeyRequest{}, "", store.VirtualKey{}, false
	}
	if _, err := h.store.GetProject(c.Request.Context(), req.ProjectID); err != nil {
		if compatibility && errors.Is(err, store.ErrNotFound) {
			project := store.Project{
				ID:          req.ProjectID,
				Name:        req.ProjectID,
				Description: "Compatibility project created from legacy /v1/keys flow.",
				CreatedAt:   time.Now().UTC(),
			}
			if createErr := h.store.CreateProject(c.Request.Context(), project); createErr != nil {
				httputil.WriteError(c, createErr)
				return createVirtualKeyRequest{}, "", store.VirtualKey{}, false
			}
		} else {
			httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unknown_project", "project_id", "Project was not found."))
			return createVirtualKeyRequest{}, "", store.VirtualKey{}, false
		}
	}

	rawKey, err := newAPIKeyValue()
	if err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusInternalServerError, "internal_error", "key_generation_failed", "", "Unable to generate API key."))
		return createVirtualKeyRequest{}, "", store.VirtualKey{}, false
	}
	var expiresAt *time.Time
	if strings.TrimSpace(req.ExpiresAt) != "" {
		parsed, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err != nil {
			httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_expires_at", "expires_at", "Field 'expires_at' must be RFC3339."))
			return createVirtualKeyRequest{}, "", store.VirtualKey{}, false
		}
		expiresAt = &parsed
	}

	keyID, err := newKeyID()
	if err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusInternalServerError, "internal_error", "key_generation_failed", "", "Unable to generate API key identifier."))
		return createVirtualKeyRequest{}, "", store.VirtualKey{}, false
	}

	key := store.VirtualKey{
		ID:                "vk_" + keyID,
		ProjectID:         req.ProjectID,
		Name:              req.Name,
		KeyHash:           middleware.HashAPIKey(rawKey),
		KeyPrefix:         prefixForDisplay(rawKey),
		RateLimit:         strings.TrimSpace(req.RateLimit),
		AllowedModels:     defaultPatterns(req.AllowedModels),
		AllowedModalities: parseAllowedModalities(req.AllowedModalities),
		AllowedToolsets:   normalizeStringList(req.AllowedToolsets),
		AllowedMCP:        normalizeStringList(req.AllowedMCP),
		IsAdmin:           req.IsAdmin,
		CreatedAt:         time.Now().UTC(),
		ExpiresAt:         expiresAt,
	}
	return req, rawKey, key, true
}

func virtualKeyToResponse(key store.VirtualKey, rawKey string) virtualKeyResponse {
	return virtualKeyResponse{
		ID:                key.ID,
		ProjectID:         key.ProjectID,
		Name:              key.Name,
		Key:               rawKey,
		KeyPrefix:         key.KeyPrefix,
		RateLimit:         key.RateLimit,
		AllowedModels:     key.AllowedModels,
		AllowedModalities: modalitiesToStrings(key.AllowedModalities),
		AllowedToolsets:   key.AllowedToolsets,
		AllowedMCP:        key.AllowedMCP,
		IsAdmin:           key.IsAdmin,
		CreatedAt:         key.CreatedAt,
		LastUsedAt:        key.LastUsedAt,
		ExpiresAt:         key.ExpiresAt,
		IsRevoked:         key.IsRevoked,
	}
}

func parseAllowedModalities(values []string) []modality.Modality {
	if len(values) == 0 {
		return nil
	}
	out := make([]modality.Modality, 0, len(values))
	for _, value := range values {
		candidate := modality.Modality(strings.TrimSpace(value))
		if candidate.Valid() {
			out = append(out, candidate)
		}
	}
	return out
}

func modalitiesToStrings(values []modality.Modality) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func defaultPatterns(values []string) []string {
	normalized := normalizeStringList(values)
	if len(normalized) == 0 {
		return []string{"*"}
	}
	return normalized
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func parseBoolQuery(c *gin.Context, name string) (bool, error) {
	raw := c.Query(name)
	if raw == "" {
		return false, nil
	}
	return strconv.ParseBool(raw)
}

func (h *ControlPlaneHandler) logAudit(c *gin.Context, kind string, resourceType string, resourceID string, payload map[string]any) {
	if h.auditLogger == nil {
		return
	}
	auth := middleware.GetAuthContext(c)
	if payload == nil {
		payload = map[string]any{}
	}
	payload["request_path"] = c.FullPath()
	payload["request_method"] = c.Request.Method
	raw, _ := json.Marshal(payload)
	h.auditLogger.Log(store.AuditEvent{
		ProjectID:    auth.ProjectID,
		ActorKeyID:   auth.KeyID,
		Kind:         kind,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		MetadataJSON: string(raw),
		CreatedAt:    time.Now().UTC(),
	})
}
