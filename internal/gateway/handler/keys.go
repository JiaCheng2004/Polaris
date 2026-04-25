package handler

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/gin-gonic/gin"
)

type KeysHandler struct {
	runtime         *gwruntime.Holder
	store           store.Store
	keyCache        *middleware.APIKeyCache
	virtualKeyCache *middleware.VirtualKeyCache
}

type createKeyRequest struct {
	Name          string   `json:"name"`
	OwnerID       string   `json:"owner_id"`
	ProjectID     string   `json:"project_id"`
	RateLimit     string   `json:"rate_limit"`
	AllowedModels []string `json:"allowed_models"`
	IsAdmin       bool     `json:"is_admin"`
	ExpiresAt     string   `json:"expires_at"`
}

type apiKeyResponse struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Key           string     `json:"key,omitempty"`
	KeyPrefix     string     `json:"key_prefix"`
	OwnerID       string     `json:"owner_id,omitempty"`
	RateLimit     string     `json:"rate_limit,omitempty"`
	AllowedModels []string   `json:"allowed_models"`
	IsAdmin       bool       `json:"is_admin"`
	CreatedAt     time.Time  `json:"created_at"`
	LastUsedAt    *time.Time `json:"last_used_at"`
	ExpiresAt     *time.Time `json:"expires_at"`
	IsRevoked     bool       `json:"is_revoked,omitempty"`
}

func NewKeysHandler(runtime *gwruntime.Holder, appStore store.Store, keyCache *middleware.APIKeyCache, virtualKeyCache *middleware.VirtualKeyCache) *KeysHandler {
	return &KeysHandler{
		runtime:         runtime,
		store:           appStore,
		keyCache:        keyCache,
		virtualKeyCache: virtualKeyCache,
	}
}

func (h *KeysHandler) Create(c *gin.Context) {
	cfg, _, ok := h.requireAdmin(c)
	if !ok {
		return
	}

	var req createKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_name", "name", "Field 'name' is required."))
		return
	}

	rateLimit := strings.TrimSpace(req.RateLimit)
	if rateLimit == "" && cfg != nil {
		rateLimit = cfg.Cache.RateLimit.Default
	}
	if rateLimit != "" {
		if _, _, err := parseRateLimitSpec(rateLimit); err != nil {
			httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_rate_limit", "rate_limit", "Field 'rate_limit' must use count/window format such as 100/min."))
			return
		}
	}

	allowedModels := req.AllowedModels
	if len(allowedModels) == 0 {
		allowedModels = []string{"*"}
	}
	for _, pattern := range allowedModels {
		if strings.TrimSpace(pattern) == "" {
			httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_allowed_models", "allowed_models", "Field 'allowed_models' must not contain empty entries."))
			return
		}
	}

	var expiresAt *time.Time
	if strings.TrimSpace(req.ExpiresAt) != "" {
		parsed, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err != nil {
			httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_expires_at", "expires_at", "Field 'expires_at' must be RFC3339."))
			return
		}
		expiresAt = &parsed
	}

	rawKey, err := newAPIKeyValue()
	if err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusInternalServerError, "internal_error", "key_generation_failed", "", "Unable to generate API key."))
		return
	}

	if cfg.Auth.Mode == config.AuthModeVirtualKeys {
		projectID := strings.TrimSpace(req.ProjectID)
		if projectID == "" {
			projectID = strings.TrimSpace(req.OwnerID)
		}
		if projectID == "" {
			projectID = "legacy-default"
		}
		if _, err := h.store.GetProject(c.Request.Context(), projectID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				project := store.Project{
					ID:          projectID,
					Name:        projectID,
					Description: "Compatibility project created from /v1/keys.",
					CreatedAt:   time.Now().UTC(),
				}
				if createErr := h.store.CreateProject(c.Request.Context(), project); createErr != nil {
					httputil.WriteError(c, createErr)
					return
				}
			} else {
				httputil.WriteError(c, err)
				return
			}
		}

		id, err := newKeyID()
		if err != nil {
			httputil.WriteError(c, httputil.NewError(http.StatusInternalServerError, "internal_error", "key_generation_failed", "", "Unable to generate API key identifier."))
			return
		}

		key := store.VirtualKey{
			ID:            "vk_" + id,
			ProjectID:     projectID,
			Name:          req.Name,
			KeyHash:       middleware.HashAPIKey(rawKey),
			KeyPrefix:     prefixForDisplay(rawKey),
			RateLimit:     rateLimit,
			AllowedModels: allowedModels,
			IsAdmin:       req.IsAdmin,
			ExpiresAt:     expiresAt,
			CreatedAt:     time.Now().UTC(),
		}
		if err := h.store.CreateVirtualKey(c.Request.Context(), key); err != nil {
			httputil.WriteError(c, err)
			return
		}
		if h.virtualKeyCache != nil {
			h.virtualKeyCache.Clear()
		}
		c.JSON(http.StatusOK, apiKeyResponse{
			ID:            key.ID,
			Name:          key.Name,
			Key:           rawKey,
			KeyPrefix:     key.KeyPrefix,
			OwnerID:       key.ProjectID,
			RateLimit:     key.RateLimit,
			AllowedModels: key.AllowedModels,
			IsAdmin:       key.IsAdmin,
			CreatedAt:     key.CreatedAt,
			ExpiresAt:     key.ExpiresAt,
		})
		return
	}

	id, err := newKeyID()
	if err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusInternalServerError, "internal_error", "key_generation_failed", "", "Unable to generate API key identifier."))
		return
	}

	key := store.APIKey{
		ID:            id,
		Name:          req.Name,
		KeyHash:       middleware.HashAPIKey(rawKey),
		KeyPrefix:     prefixForDisplay(rawKey),
		OwnerID:       strings.TrimSpace(req.OwnerID),
		RateLimit:     rateLimit,
		AllowedModels: allowedModels,
		IsAdmin:       req.IsAdmin,
		ExpiresAt:     expiresAt,
	}
	if err := h.store.CreateAPIKey(c.Request.Context(), key); err != nil {
		httputil.WriteError(c, err)
		return
	}
	if h.keyCache != nil {
		h.keyCache.Clear()
	}

	c.JSON(http.StatusOK, apiKeyResponse{
		ID:            key.ID,
		Name:          key.Name,
		Key:           rawKey,
		KeyPrefix:     key.KeyPrefix,
		OwnerID:       key.OwnerID,
		RateLimit:     key.RateLimit,
		AllowedModels: key.AllowedModels,
		IsAdmin:       key.IsAdmin,
		CreatedAt:     time.Now().UTC(),
		ExpiresAt:     key.ExpiresAt,
	})
}

func (h *KeysHandler) List(c *gin.Context) {
	_, _, ok := h.requireAdmin(c)
	if !ok {
		return
	}

	includeRevoked := false
	if raw := c.Query("include_revoked"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_include_revoked", "include_revoked", "Query parameter 'include_revoked' must be a boolean."))
			return
		}
		includeRevoked = parsed
	}

	data := make([]apiKeyResponse, 0)
	if snapshot, _, _ := h.requireAdmin(c); snapshot != nil && snapshot.Auth.Mode == config.AuthModeVirtualKeys {
		projectID := strings.TrimSpace(c.Query("project_id"))
		if projectID == "" {
			projectID = strings.TrimSpace(c.Query("owner_id"))
		}
		keys, err := h.store.ListVirtualKeys(c.Request.Context(), projectID, includeRevoked)
		if err != nil {
			httputil.WriteError(c, err)
			return
		}
		for _, key := range keys {
			data = append(data, apiKeyResponse{
				ID:            key.ID,
				Name:          key.Name,
				KeyPrefix:     key.KeyPrefix,
				OwnerID:       key.ProjectID,
				RateLimit:     key.RateLimit,
				AllowedModels: key.AllowedModels,
				IsAdmin:       key.IsAdmin,
				CreatedAt:     key.CreatedAt,
				LastUsedAt:    key.LastUsedAt,
				ExpiresAt:     key.ExpiresAt,
				IsRevoked:     key.IsRevoked,
			})
		}
	} else {
		keys, err := h.store.ListAPIKeys(c.Request.Context(), c.Query("owner_id"), includeRevoked)
		if err != nil {
			httputil.WriteError(c, err)
			return
		}

		for _, key := range keys {
			data = append(data, apiKeyResponse{
				ID:            key.ID,
				Name:          key.Name,
				KeyPrefix:     key.KeyPrefix,
				OwnerID:       key.OwnerID,
				RateLimit:     key.RateLimit,
				AllowedModels: key.AllowedModels,
				IsAdmin:       key.IsAdmin,
				CreatedAt:     key.CreatedAt,
				LastUsedAt:    key.LastUsedAt,
				ExpiresAt:     key.ExpiresAt,
				IsRevoked:     key.IsRevoked,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   data,
	})
}

func (h *KeysHandler) Delete(c *gin.Context) {
	cfg, _, ok := h.requireAdmin(c)
	if !ok {
		return
	}

	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_key_id", "id", "Key id is required."))
		return
	}

	if cfg.Auth.Mode == config.AuthModeVirtualKeys {
		if err := h.store.DeleteVirtualKey(c.Request.Context(), id); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				httputil.WriteError(c, httputil.NewError(http.StatusNotFound, "invalid_request_error", "key_not_found", "id", "API key was not found."))
				return
			}
			httputil.WriteError(c, err)
			return
		}
		if h.virtualKeyCache != nil {
			h.virtualKeyCache.Clear()
		}
	} else {
		if err := h.store.DeleteAPIKey(c.Request.Context(), id); err != nil {
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
	}

	c.Status(http.StatusNoContent)
}

func (h *KeysHandler) requireAdmin(c *gin.Context) (*config.Config, middleware.AuthContext, bool) {
	if h.store == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusInternalServerError, "internal_error", "store_unavailable", "", "API key store is unavailable."))
		return nil, middleware.AuthContext{}, false
	}

	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	if snapshot == nil || snapshot.Config == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusInternalServerError, "internal_error", "runtime_unavailable", "", "Runtime configuration is unavailable."))
		return nil, middleware.AuthContext{}, false
	}

	auth := middleware.GetAuthContext(c)
	if (snapshot.Config.Auth.Mode != config.AuthModeMultiUser && snapshot.Config.Auth.Mode != config.AuthModeVirtualKeys) || !auth.IsAdmin {
		httputil.WriteError(c, httputil.NewError(http.StatusForbidden, "permission_error", "admin_required", "", "Admin access is required for this endpoint."))
		return snapshot.Config, auth, false
	}

	return snapshot.Config, auth, true
}

func newKeyID() (string, error) {
	var bytes [12]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return "key_" + hex.EncodeToString(bytes[:]), nil
}

func newAPIKeyValue() (string, error) {
	var bytes [24]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return "polaris-sk-live-" + hex.EncodeToString(bytes[:]), nil
}

func prefixForDisplay(raw string) string {
	if len(raw) <= 8 {
		return raw
	}
	return raw[:8]
}

func parseRateLimitSpec(raw string) (int64, string, error) {
	parts := strings.Split(strings.TrimSpace(raw), "/")
	if len(parts) != 2 {
		return 0, "", errors.New("invalid rate limit")
	}

	limit, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || limit <= 0 {
		return 0, "", errors.New("invalid rate limit")
	}

	switch strings.ToLower(parts[1]) {
	case "s", "sec", "second", "m", "min", "minute", "h", "hour", "d", "day":
		return limit, parts[1], nil
	default:
		return 0, "", errors.New("invalid rate limit")
	}
}
