package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/gin-gonic/gin"
)

const (
	externalAuthClaimsHeader    = "X-Polaris-External-Auth"
	externalAuthTimestampHeader = "X-Polaris-External-Auth-Timestamp"
	externalAuthSignatureHeader = "X-Polaris-External-Auth-Signature"
	externalAuthProviderSigned  = "signed_headers"
)

type externalAuthError struct {
	code    string
	message string
}

func (e externalAuthError) Error() string {
	return e.code
}

type externalAuthClaims struct {
	Subject            string               `json:"sub"`
	ProjectID          string               `json:"project_id"`
	KeyID              string               `json:"key_id"`
	KeyPrefix          string               `json:"key_prefix"`
	IsAdmin            bool                 `json:"is_admin"`
	RateLimit          string               `json:"rate_limit"`
	AllowedModels      []string             `json:"allowed_models"`
	AllowedModalities  []modality.Modality  `json:"allowed_modalities"`
	AllowedToolsets    []string             `json:"allowed_toolsets"`
	AllowedMCPBindings []string             `json:"allowed_mcp_bindings"`
	PolicyModels       []string             `json:"policy_models"`
	PolicyModalities   []modality.Modality  `json:"policy_modalities"`
	PolicyToolsets     []string             `json:"policy_toolsets"`
	PolicyMCPBindings  []string             `json:"policy_mcp_bindings"`
	ExpiresAt          *externalClaimExpiry `json:"expires_at"`
}

type externalClaimExpiry struct {
	time.Time
}

func (e *externalClaimExpiry) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}

	var rawString string
	if err := json.Unmarshal(data, &rawString); err == nil {
		parsed, err := time.Parse(time.RFC3339, rawString)
		if err != nil {
			return fmt.Errorf("parse expires_at: %w", err)
		}
		e.Time = parsed
		return nil
	}

	var unixSeconds int64
	if err := json.Unmarshal(data, &unixSeconds); err != nil {
		return fmt.Errorf("parse expires_at: %w", err)
	}
	e.Time = time.Unix(unixSeconds, 0).UTC()
	return nil
}

type externalAuthCache struct {
	mu      sync.Mutex
	entries map[string]externalAuthCacheEntry
}

type externalAuthCacheEntry struct {
	auth      AuthContext
	expiresAt time.Time
}

func newExternalAuthCache() *externalAuthCache {
	return &externalAuthCache{entries: map[string]externalAuthCacheEntry{}}
}

func (c *externalAuthCache) Get(key string, now time.Time) (AuthContext, bool) {
	if c == nil {
		return AuthContext{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		return AuthContext{}, false
	}
	if !entry.expiresAt.After(now) {
		delete(c.entries, key)
		return AuthContext{}, false
	}
	return entry.auth, true
}

func (c *externalAuthCache) Set(key string, auth AuthContext, expiresAt time.Time) {
	if c == nil || key == "" || expiresAt.IsZero() {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = externalAuthCacheEntry{auth: auth, expiresAt: expiresAt}
}

func authenticateExternalSignedHeaders(req *http.Request, cfg config.ExternalAuthConfig, now time.Time, cache *externalAuthCache) (AuthContext, error) {
	if req == nil {
		return AuthContext{}, newExternalAuthError("invalid_external_auth", "External auth request is unavailable.")
	}
	if strings.TrimSpace(cfg.Provider) != externalAuthProviderSigned {
		return AuthContext{}, newExternalAuthError("unsupported_external_auth_provider", "Configured external auth provider is not supported.")
	}
	secret := strings.TrimSpace(cfg.SharedSecret)
	if secret == "" {
		return AuthContext{}, newExternalAuthError("external_auth_secret_missing", "External auth shared secret is not configured.")
	}

	encodedClaims := strings.TrimSpace(req.Header.Get(externalAuthClaimsHeader))
	timestampValue := strings.TrimSpace(req.Header.Get(externalAuthTimestampHeader))
	signatureValue := strings.TrimSpace(req.Header.Get(externalAuthSignatureHeader))
	if encodedClaims == "" || timestampValue == "" || signatureValue == "" {
		return AuthContext{}, newExternalAuthError("missing_external_auth", "Missing external auth headers.")
	}

	timestamp, err := strconv.ParseInt(timestampValue, 10, 64)
	if err != nil {
		return AuthContext{}, newExternalAuthError("invalid_external_auth_timestamp", "External auth timestamp must be Unix seconds.")
	}
	signedAt := time.Unix(timestamp, 0).UTC()
	if outsideClockSkew(now.UTC(), signedAt, cfg.MaxClockSkew) {
		return AuthContext{}, newExternalAuthError("external_auth_timestamp_expired", "External auth timestamp is outside the allowed clock skew.")
	}

	expectedSignature := signExternalAuth(secret, timestampValue, encodedClaims)
	actualSignature, err := decodeExternalSignature(signatureValue)
	if err != nil || !hmac.Equal(actualSignature, expectedSignature) {
		return AuthContext{}, newExternalAuthError("invalid_external_auth_signature", "External auth signature is invalid.")
	}

	cacheKey := externalAuthCacheKey(secret, timestampValue, encodedClaims, signatureValue)
	if auth, ok := cache.Get(cacheKey, now.UTC()); ok {
		return auth, nil
	}

	decodedClaims, err := decodeExternalClaims(encodedClaims)
	if err != nil {
		return AuthContext{}, newExternalAuthError("invalid_external_auth_claims", "External auth claims are invalid.")
	}
	var claims externalAuthClaims
	if err := json.Unmarshal(decodedClaims, &claims); err != nil {
		return AuthContext{}, newExternalAuthError("invalid_external_auth_claims", "External auth claims must be valid JSON.")
	}

	auth, claimsExpiresAt, err := authContextFromExternalClaims(claims, now.UTC())
	if err != nil {
		return AuthContext{}, err
	}
	if cfg.CacheTTL > 0 {
		cacheExpiresAt := now.UTC().Add(cfg.CacheTTL)
		if claimsExpiresAt != nil && claimsExpiresAt.Before(cacheExpiresAt) {
			cacheExpiresAt = *claimsExpiresAt
		}
		cache.Set(cacheKey, auth, cacheExpiresAt)
	}
	return auth, nil
}

func writeExternalAuthError(c *gin.Context, err error) {
	var authErr externalAuthError
	if errors.As(err, &authErr) {
		httputil.WriteError(c, httputil.NewError(http.StatusUnauthorized, "authentication_error", authErr.code, "", authErr.message))
		return
	}
	httputil.WriteError(c, httputil.NewError(http.StatusUnauthorized, "authentication_error", "invalid_external_auth", "", "External authentication failed."))
}

func newExternalAuthError(code string, message string) externalAuthError {
	return externalAuthError{code: code, message: message}
}

func signExternalAuth(secret string, timestamp string, encodedClaims string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("\n"))
	mac.Write([]byte(encodedClaims))
	return mac.Sum(nil)
}

func decodeExternalSignature(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "v1=")
	return hex.DecodeString(value)
}

func decodeExternalClaims(value string) ([]byte, error) {
	if decoded, err := base64.RawURLEncoding.DecodeString(value); err == nil {
		return decoded, nil
	}
	return base64.URLEncoding.DecodeString(value)
}

func outsideClockSkew(now time.Time, signedAt time.Time, maxSkew time.Duration) bool {
	if maxSkew <= 0 {
		return true
	}
	if signedAt.After(now) {
		return signedAt.Sub(now) > maxSkew
	}
	return now.Sub(signedAt) > maxSkew
}

func externalAuthCacheKey(secret string, timestamp string, encodedClaims string, signature string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:]) + ":" + timestamp + ":" + encodedClaims + ":" + signature
}

func authContextFromExternalClaims(claims externalAuthClaims, now time.Time) (AuthContext, *time.Time, error) {
	subject := strings.TrimSpace(claims.Subject)
	if subject == "" {
		return AuthContext{}, nil, newExternalAuthError("invalid_external_auth_claims", "External auth claims must include sub.")
	}
	if claims.ExpiresAt != nil {
		expiresAt := claims.ExpiresAt.UTC()
		if !expiresAt.After(now) {
			return AuthContext{}, nil, newExternalAuthError("external_auth_claims_expired", "External auth claims have expired.")
		}
	}

	allowedModels := normalizeExternalStringSlice(claims.AllowedModels)
	if len(allowedModels) == 0 {
		allowedModels = []string{"*"}
	}
	allowedModalities, err := normalizeExternalModalities(claims.AllowedModalities, true)
	if err != nil {
		return AuthContext{}, nil, err
	}
	policyModalities, err := normalizeExternalModalities(claims.PolicyModalities, false)
	if err != nil {
		return AuthContext{}, nil, err
	}

	keyID := strings.TrimSpace(claims.KeyID)
	if keyID == "" {
		keyID = "external:" + subject
	}

	var expiresAt *time.Time
	if claims.ExpiresAt != nil {
		value := claims.ExpiresAt.UTC()
		expiresAt = &value
	}
	return AuthContext{
		ProjectID:          strings.TrimSpace(claims.ProjectID),
		KeyID:              keyID,
		OwnerID:            subject,
		KeyPrefix:          strings.TrimSpace(claims.KeyPrefix),
		RateLimit:          strings.TrimSpace(claims.RateLimit),
		AllowedModels:      allowedModels,
		AllowedModalities:  allowedModalities,
		AllowedToolsets:    normalizeExternalStringSlice(claims.AllowedToolsets),
		AllowedMCPBindings: normalizeExternalStringSlice(claims.AllowedMCPBindings),
		PolicyModels:       normalizeExternalStringSlice(claims.PolicyModels),
		PolicyModalities:   policyModalities,
		PolicyToolsets:     normalizeExternalStringSlice(claims.PolicyToolsets),
		PolicyMCPBindings:  normalizeExternalStringSlice(claims.PolicyMCPBindings),
		IsAdmin:            claims.IsAdmin,
		Mode:               string(config.AuthModeExternal),
		TokenSource:        "external",
	}, expiresAt, nil
}

func normalizeExternalStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func normalizeExternalModalities(values []modality.Modality, defaultAll bool) ([]modality.Modality, error) {
	if len(values) == 0 {
		if defaultAll {
			return allModalities(), nil
		}
		return nil, nil
	}
	out := make([]modality.Modality, 0, len(values))
	for _, value := range values {
		if !value.Valid() {
			return nil, newExternalAuthError("invalid_external_auth_claims", "External auth claims include an invalid modality.")
		}
		out = append(out, value)
	}
	return out, nil
}
