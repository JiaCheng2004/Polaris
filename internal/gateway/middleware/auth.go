package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/gateway/telemetry"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
)

const (
	anonymousKeyID = "anonymous"
	authCacheTTL   = 60 * time.Second
)

func Auth(holder *gwruntime.Holder, appStore store.Store, keyCache *APIKeyCache, virtualKeyCache *VirtualKeyCache, logger *slog.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}
	if keyCache == nil {
		keyCache = NewAPIKeyCache(authCacheTTL)
	}
	if virtualKeyCache == nil {
		virtualKeyCache = NewVirtualKeyCache(authCacheTTL)
	}
	externalAuthCache := newExternalAuthCache()

	return func(c *gin.Context) {
		snapshot := RuntimeSnapshot(c, holder)
		if snapshot == nil || snapshot.Config == nil {
			httputil.WriteError(c, httputil.NewError(http.StatusInternalServerError, "internal_error", "runtime_unavailable", "", "Runtime configuration is unavailable."))
			return
		}
		cfg := snapshot.Config
		ctx, span := telemetry.StartInternalSpan(c.Request.Context(), "auth.lookup", attribute.String("polaris.auth_mode", string(cfg.Auth.Mode)))
		defer span.End()
		c.Request = c.Request.WithContext(ctx)

		switch cfg.Auth.Mode {
		case config.AuthModeNone:
			SetAuthContext(c, AuthContext{
				KeyID:         anonymousKeyID,
				AllowedModels: []string{"*"},
				Mode:          string(config.AuthModeNone),
				TokenSource:   "anonymous",
			})
			span.SetAttributes(attribute.String("polaris.auth_source", "anonymous"))
			c.Next()
			return
		case config.AuthModeStatic:
			rawKey, ok := bearerToken(c.GetHeader("Authorization"))
			if !ok {
				httputil.WriteError(c, httputil.NewError(http.StatusUnauthorized, "authentication_error", "missing_api_key", "", "Missing Authorization bearer token."))
				return
			}

			keyHash := HashAPIKey(rawKey)
			key, ok := snapshot.StaticKeys[keyHash]
			if !ok {
				httputil.WriteError(c, httputil.NewError(http.StatusUnauthorized, "authentication_error", "invalid_api_key", "", "Invalid API key."))
				return
			}

			allowedModels := key.AllowedModels
			if len(allowedModels) == 0 {
				allowedModels = []string{"*"}
			}

			SetAuthContext(c, AuthContext{
				KeyID:             key.Name,
				KeyPrefix:         keyPrefix(rawKey),
				RateLimit:         key.RateLimit,
				AllowedModels:     allowedModels,
				AllowedModalities: allModalities(),
				Mode:              string(config.AuthModeStatic),
				TokenSource:       "static",
			})
			span.SetAttributes(attribute.String("polaris.auth_source", "static"))
			c.Next()
			return
		case config.AuthModeExternal:
			auth, err := authenticateExternalSignedHeaders(c.Request, cfg.Auth.External, time.Now().UTC(), externalAuthCache)
			if err != nil {
				writeExternalAuthError(c, err)
				return
			}

			SetAuthContext(c, auth)
			span.SetAttributes(
				attribute.String("polaris.auth_source", "external"),
				attribute.String("polaris.project_id", auth.ProjectID),
				attribute.String("polaris.key_id", auth.KeyID),
			)
			c.Next()
			return
		case config.AuthModeVirtualKeys:
			rawKey, ok := bearerToken(c.GetHeader("Authorization"))
			if !ok {
				httputil.WriteError(c, httputil.NewError(http.StatusUnauthorized, "authentication_error", "missing_api_key", "", "Missing Authorization bearer token."))
				return
			}
			if appStore == nil {
				httputil.WriteError(c, httputil.NewError(http.StatusInternalServerError, "internal_error", "store_unavailable", "", "Virtual key store is unavailable."))
				return
			}
			if isControlPlaneRequest(c) && matchesStoredKeyHash(cfg.Auth.BootstrapAdminKeyHash, rawKey) {
				SetAuthContext(c, AuthContext{
					KeyID:             "bootstrap-admin",
					VirtualKeyID:      "bootstrap-admin",
					KeyPrefix:         keyPrefix(rawKey),
					AllowedModels:     []string{"*"},
					AllowedModalities: allModalities(),
					IsAdmin:           true,
					Mode:              string(config.AuthModeVirtualKeys),
					TokenSource:       "bootstrap_admin",
				})
				span.SetAttributes(attribute.String("polaris.auth_source", "bootstrap_admin"))
				c.Next()
				return
			}

			keyHash := HashAPIKey(rawKey)
			virtualKey, err := loadVirtualKey(c.Request.Context(), appStore, virtualKeyCache, keyHash)
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					httputil.WriteError(c, httputil.NewError(http.StatusUnauthorized, "authentication_error", "invalid_api_key", "", "Invalid API key."))
					return
				}
				logger.Error("virtual-key auth lookup failed", "request_id", GetRequestID(c), "error", err)
				httputil.WriteError(c, httputil.NewError(http.StatusInternalServerError, "internal_error", "auth_lookup_failed", "", "Unable to validate API key."))
				return
			}
			if virtualKey.IsRevoked {
				virtualKeyCache.Delete(keyHash)
				httputil.WriteError(c, httputil.NewError(http.StatusUnauthorized, "authentication_error", "key_revoked", "", "API key has been revoked."))
				return
			}
			if virtualKey.ExpiresAt != nil && time.Now().UTC().After(virtualKey.ExpiresAt.UTC()) {
				virtualKeyCache.Delete(keyHash)
				httputil.WriteError(c, httputil.NewError(http.StatusUnauthorized, "authentication_error", "key_expired", "", "API key has expired."))
				return
			}

			allowedModels := virtualKey.AllowedModels
			if len(allowedModels) == 0 {
				allowedModels = []string{"*"}
			}
			allowedModalities := virtualKey.AllowedModalities
			if len(allowedModalities) == 0 {
				allowedModalities = allModalities()
			}
			policyModels, policyModalities, policyToolsets, policyBindings, err := aggregateProjectPolicies(c.Request.Context(), appStore, virtualKey.ProjectID)
			if err != nil {
				logger.Warn("project policy lookup failed", "project_id", virtualKey.ProjectID, "error", err)
			}

			SetAuthContext(c, AuthContext{
				ProjectID:          virtualKey.ProjectID,
				VirtualKeyID:       virtualKey.ID,
				KeyID:              virtualKey.ID,
				KeyPrefix:          virtualKey.KeyPrefix,
				RateLimit:          virtualKey.RateLimit,
				AllowedModels:      allowedModels,
				AllowedModalities:  allowedModalities,
				AllowedToolsets:    append([]string(nil), virtualKey.AllowedToolsets...),
				AllowedMCPBindings: append([]string(nil), virtualKey.AllowedMCP...),
				PolicyModels:       policyModels,
				PolicyModalities:   policyModalities,
				PolicyToolsets:     policyToolsets,
				PolicyMCPBindings:  policyBindings,
				IsAdmin:            virtualKey.IsAdmin,
				Mode:               string(config.AuthModeVirtualKeys),
				TokenSource:        "virtual_key",
			})
			span.SetAttributes(
				attribute.String("polaris.auth_source", "virtual_key"),
				attribute.String("polaris.project_id", virtualKey.ProjectID),
				attribute.String("polaris.virtual_key_id", virtualKey.ID),
			)
			touchVirtualKeyLastUsed(appStore, logger, virtualKey.ID)
			c.Next()
			return
		case config.AuthModeMultiUser:
			rawKey, ok := bearerToken(c.GetHeader("Authorization"))
			if !ok {
				httputil.WriteError(c, httputil.NewError(http.StatusUnauthorized, "authentication_error", "missing_api_key", "", "Missing Authorization bearer token."))
				return
			}
			if appStore == nil {
				httputil.WriteError(c, httputil.NewError(http.StatusInternalServerError, "internal_error", "store_unavailable", "", "API key store is unavailable."))
				return
			}

			keyHash := HashAPIKey(rawKey)
			key, err := loadAPIKey(c.Request.Context(), appStore, keyCache, keyHash)
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					httputil.WriteError(c, httputil.NewError(http.StatusUnauthorized, "authentication_error", "invalid_api_key", "", "Invalid API key."))
					return
				}
				logger.Error("multi-user auth lookup failed", "request_id", GetRequestID(c), "error", err)
				httputil.WriteError(c, httputil.NewError(http.StatusInternalServerError, "internal_error", "auth_lookup_failed", "", "Unable to validate API key."))
				return
			}

			if key.IsRevoked {
				keyCache.Delete(keyHash)
				httputil.WriteError(c, httputil.NewError(http.StatusUnauthorized, "authentication_error", "key_revoked", "", "API key has been revoked."))
				return
			}
			if key.ExpiresAt != nil && time.Now().UTC().After(key.ExpiresAt.UTC()) {
				keyCache.Delete(keyHash)
				httputil.WriteError(c, httputil.NewError(http.StatusUnauthorized, "authentication_error", "key_expired", "", "API key has expired."))
				return
			}

			allowedModels := key.AllowedModels
			if len(allowedModels) == 0 {
				allowedModels = []string{"*"}
			}
			prefix := key.KeyPrefix
			if prefix == "" {
				prefix = keyPrefix(rawKey)
			}

			SetAuthContext(c, AuthContext{
				KeyID:             key.ID,
				OwnerID:           key.OwnerID,
				KeyPrefix:         prefix,
				RateLimit:         key.RateLimit,
				AllowedModels:     allowedModels,
				AllowedModalities: allModalities(),
				IsAdmin:           key.IsAdmin,
				Mode:              string(config.AuthModeMultiUser),
				TokenSource:       "api_key",
			})
			span.SetAttributes(attribute.String("polaris.auth_source", "api_key"))

			touchLastUsed(appStore, logger, key.ID)
			c.Next()
			return
		default:
			logger.Error("unsupported auth mode in runtime kernel", "mode", cfg.Auth.Mode)
			httputil.WriteError(c, httputil.NewError(http.StatusInternalServerError, "internal_error", "unsupported_auth_mode", "", "Configured auth mode is not supported by this runtime build."))
			return
		}
	}
}

func HashAPIKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func ModelAllowed(patterns []string, candidate string) bool {
	if len(patterns) == 0 {
		return false
	}
	for _, pattern := range patterns {
		if pattern == "*" {
			return true
		}
		regex := "^" + regexp.QuoteMeta(pattern) + "$"
		regex = strings.ReplaceAll(regex, `\*`, ".*")
		if matched, _ := regexp.MatchString(regex, candidate); matched {
			return true
		}
	}
	return false
}

func ScopeAllowed(primary []string, policy []string, candidate string) bool {
	if candidate == "" {
		return true
	}
	if len(primary) > 0 && !ModelAllowed(primary, candidate) {
		return false
	}
	if len(policy) > 0 && !ModelAllowed(policy, candidate) {
		return false
	}
	return true
}

func ModalityScopeAllowed(primary []modality.Modality, policy []modality.Modality, candidate modality.Modality) bool {
	if !ModalityAllowed(primary, candidate) {
		return false
	}
	if len(policy) > 0 && !ModalityAllowed(policy, candidate) {
		return false
	}
	return true
}

func StringAllowed(values []string, candidate string) bool {
	if candidate == "" {
		return true
	}
	if len(values) == 0 {
		return true
	}
	for _, value := range values {
		if value == "*" || strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(candidate)) {
			return true
		}
	}
	return false
}

func StringScopeAllowed(primary []string, policy []string, candidate string) bool {
	if !StringAllowed(primary, candidate) {
		return false
	}
	if len(policy) > 0 && !StringAllowed(policy, candidate) {
		return false
	}
	return true
}

func bearerToken(header string) (string, bool) {
	if header == "" {
		return "", false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if token == "" {
		return "", false
	}
	return token, true
}

func keyPrefix(raw string) string {
	if len(raw) <= 8 {
		return raw
	}
	return raw[:8]
}

func loadAPIKey(ctx context.Context, appStore store.Store, keyCache *APIKeyCache, keyHash string) (*store.APIKey, error) {
	if cached, ok := keyCache.Get(keyHash); ok {
		return cached, nil
	}

	key, err := appStore.GetAPIKeyByHash(ctx, keyHash)
	if err != nil {
		return nil, err
	}
	keyCache.Set(keyHash, key)
	return key, nil
}

func loadVirtualKey(ctx context.Context, appStore store.Store, keyCache *VirtualKeyCache, keyHash string) (*store.VirtualKey, error) {
	if cached, ok := keyCache.Get(keyHash); ok {
		return cached, nil
	}

	key, err := appStore.GetVirtualKeyByHash(ctx, keyHash)
	if err != nil {
		return nil, err
	}
	keyCache.Set(keyHash, key)
	return key, nil
}

func touchLastUsed(appStore store.Store, logger *slog.Logger, keyID string) {
	if appStore == nil || keyID == "" {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		if err := appStore.UpdateAPIKeyLastUsed(ctx, keyID, time.Now().UTC()); err != nil && !errors.Is(err, store.ErrNotFound) {
			logger.Warn("update api key last_used_at failed", "key_id", keyID, "error", err)
		}
	}()
}

func touchVirtualKeyLastUsed(appStore store.Store, logger *slog.Logger, keyID string) {
	if appStore == nil || keyID == "" {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		if err := appStore.UpdateVirtualKeyLastUsed(ctx, keyID, time.Now().UTC()); err != nil && !errors.Is(err, store.ErrNotFound) {
			logger.Warn("update virtual key last_used_at failed", "key_id", keyID, "error", err)
		}
	}()
}

func ModalityAllowed(allowed []modality.Modality, candidate modality.Modality) bool {
	if candidate == "" {
		return true
	}
	if len(allowed) == 0 {
		return true
	}
	for _, value := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func allModalities() []modality.Modality {
	return []modality.Modality{
		modality.ModalityChat,
		modality.ModalityEmbed,
		modality.ModalityImage,
		modality.ModalityVoice,
		modality.ModalityVideo,
		modality.ModalityAudio,
		modality.ModalityInterpreting,
		modality.ModalityMusic,
		modality.ModalityTranslation,
		modality.ModalityNotes,
		modality.ModalityPodcast,
	}
}

func aggregateProjectPolicies(ctx context.Context, appStore store.Store, projectID string) ([]string, []modality.Modality, []string, []string, error) {
	if appStore == nil || projectID == "" {
		return nil, nil, nil, nil, nil
	}
	policies, err := appStore.ListPolicies(ctx, projectID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if len(policies) == 0 {
		return nil, nil, nil, nil, nil
	}
	models := make(map[string]struct{})
	modalitiesSet := make(map[modality.Modality]struct{})
	toolsets := make(map[string]struct{})
	bindings := make(map[string]struct{})
	for _, policy := range policies {
		for _, value := range policy.AllowedModels {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				models[trimmed] = struct{}{}
			}
		}
		for _, value := range policy.AllowedModalities {
			if value.Valid() {
				modalitiesSet[value] = struct{}{}
			}
		}
		for _, value := range policy.AllowedToolsets {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				toolsets[trimmed] = struct{}{}
			}
		}
		for _, value := range policy.AllowedMCP {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				bindings[trimmed] = struct{}{}
			}
		}
	}

	return mapKeys(models), mapModalityKeys(modalitiesSet), mapKeys(toolsets), mapKeys(bindings), nil
}

func mapKeys(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	return out
}

func mapModalityKeys(values map[modality.Modality]struct{}) []modality.Modality {
	if len(values) == 0 {
		return nil
	}
	out := make([]modality.Modality, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	return out
}

func matchesStoredKeyHash(stored string, raw string) bool {
	stored = strings.TrimSpace(stored)
	raw = strings.TrimSpace(raw)
	if stored == "" || raw == "" {
		return false
	}
	if !strings.HasPrefix(stored, "sha256:") {
		return false
	}
	return HashAPIKey(raw) == stored
}

func isControlPlaneRequest(c *gin.Context) bool {
	path := c.FullPath()
	if path == "" {
		path = c.Request.URL.Path
	}
	switch {
	case strings.HasPrefix(path, "/v1/projects"),
		strings.HasPrefix(path, "/v1/virtual_keys"),
		strings.HasPrefix(path, "/v1/policies"),
		strings.HasPrefix(path, "/v1/budgets"),
		strings.HasPrefix(path, "/v1/tools"),
		strings.HasPrefix(path, "/v1/toolsets"),
		strings.HasPrefix(path, "/v1/mcp/bindings"),
		strings.HasPrefix(path, "/v1/keys"):
		return true
	default:
		return false
	}
}
