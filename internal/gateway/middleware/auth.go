package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/gin-gonic/gin"
)

const anonymousKeyID = "anonymous"

func Auth(cfg *config.Config, logger *slog.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}

	staticKeys := map[string]config.StaticKeyConfig{}
	for _, key := range cfg.Auth.StaticKeys {
		staticKeys[key.KeyHash] = key
	}

	return func(c *gin.Context) {
		switch cfg.Auth.Mode {
		case config.AuthModeNone:
			SetAuthContext(c, AuthContext{
				KeyID:         anonymousKeyID,
				AllowedModels: []string{"*"},
				Mode:          string(config.AuthModeNone),
			})
			c.Next()
			return
		case config.AuthModeStatic:
			rawKey, ok := bearerToken(c.GetHeader("Authorization"))
			if !ok {
				httputil.WriteError(c, httputil.NewError(http.StatusUnauthorized, "authentication_error", "missing_api_key", "", "Missing Authorization bearer token."))
				return
			}

			keyHash := HashAPIKey(rawKey)
			key, ok := staticKeys[keyHash]
			if !ok {
				httputil.WriteError(c, httputil.NewError(http.StatusUnauthorized, "authentication_error", "invalid_api_key", "", "Invalid API key."))
				return
			}

			allowedModels := key.AllowedModels
			if len(allowedModels) == 0 {
				allowedModels = []string{"*"}
			}

			SetAuthContext(c, AuthContext{
				KeyID:         key.Name,
				KeyPrefix:     keyPrefix(rawKey),
				RateLimit:     key.RateLimit,
				AllowedModels: allowedModels,
				Mode:          string(config.AuthModeStatic),
			})
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
