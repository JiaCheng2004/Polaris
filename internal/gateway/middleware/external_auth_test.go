package middleware

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/gin-gonic/gin"
)

func TestAuthenticateExternalSignedHeadersValidClaims(t *testing.T) {
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	secret := "external-test-secret"
	req := signedExternalAuthRequest(t, secret, now, map[string]any{
		"sub":                  "user_123",
		"project_id":           "proj_123",
		"key_id":               "key_123",
		"is_admin":             true,
		"rate_limit":           "12/min",
		"allowed_models":       []string{"openai/*"},
		"allowed_modalities":   []string{"chat"},
		"allowed_toolsets":     []string{"tools_public"},
		"allowed_mcp_bindings": []string{"mcp_public"},
		"policy_models":        []string{"openai/gpt-5.5"},
		"policy_modalities":    []string{"chat"},
		"policy_toolsets":      []string{"tools_public"},
		"policy_mcp_bindings":  []string{"mcp_public"},
		"expires_at":           now.Add(time.Minute).Format(time.RFC3339),
	})

	auth, err := authenticateExternalSignedHeaders(req, externalAuthTestConfig(secret), now, newExternalAuthCache())
	if err != nil {
		t.Fatalf("authenticateExternalSignedHeaders() error = %v", err)
	}

	if auth.Mode != string(config.AuthModeExternal) || auth.TokenSource != "external" {
		t.Fatalf("unexpected auth source: %#v", auth)
	}
	if auth.OwnerID != "user_123" || auth.ProjectID != "proj_123" || auth.KeyID != "key_123" || !auth.IsAdmin {
		t.Fatalf("unexpected auth identity: %#v", auth)
	}
	if !ScopeAllowed(auth.AllowedModels, auth.PolicyModels, "openai/gpt-5.5") {
		t.Fatalf("expected signed claims to allow openai/gpt-5.5")
	}
	if ScopeAllowed(auth.AllowedModels, auth.PolicyModels, "anthropic/claude-sonnet-4-6") {
		t.Fatalf("expected signed claims to block anthropic model")
	}
	if !ModalityScopeAllowed(auth.AllowedModalities, auth.PolicyModalities, modality.ModalityChat) {
		t.Fatalf("expected signed claims to allow chat modality")
	}
	if ModalityScopeAllowed(auth.AllowedModalities, auth.PolicyModalities, modality.ModalityImage) {
		t.Fatalf("expected signed claims to block image modality")
	}
}

func TestAuthenticateExternalSignedHeadersDefaults(t *testing.T) {
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	secret := "external-test-secret"
	req := signedExternalAuthRequest(t, secret, now, map[string]any{
		"sub": "user_123",
	})

	auth, err := authenticateExternalSignedHeaders(req, externalAuthTestConfig(secret), now, newExternalAuthCache())
	if err != nil {
		t.Fatalf("authenticateExternalSignedHeaders() error = %v", err)
	}

	if auth.KeyID != "external:user_123" {
		t.Fatalf("expected derived key ID, got %q", auth.KeyID)
	}
	if !ScopeAllowed(auth.AllowedModels, nil, "any/provider-model") {
		t.Fatalf("expected default wildcard model scope")
	}
	if !ModalityScopeAllowed(auth.AllowedModalities, nil, modality.ModalityVideo) {
		t.Fatalf("expected default all-modality scope")
	}
}

func TestAuthenticateExternalSignedHeadersRejectsMissingHeaders(t *testing.T) {
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)

	_, err := authenticateExternalSignedHeaders(req, externalAuthTestConfig("secret"), now, newExternalAuthCache())
	assertExternalAuthError(t, err, "missing_external_auth")
}

func TestAuthenticateExternalSignedHeadersRejectsBadSignature(t *testing.T) {
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	req := signedExternalAuthRequest(t, "secret", now, map[string]any{
		"sub": "user_123",
	})
	req.Header.Set(externalAuthSignatureHeader, "v1="+hex.EncodeToString([]byte("bad-signature")))

	_, err := authenticateExternalSignedHeaders(req, externalAuthTestConfig("secret"), now, newExternalAuthCache())
	assertExternalAuthError(t, err, "invalid_external_auth_signature")
}

func TestAuthenticateExternalSignedHeadersRejectsExpiredTimestamp(t *testing.T) {
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	secret := "secret"
	req := signedExternalAuthRequest(t, secret, now.Add(-2*time.Minute), map[string]any{
		"sub": "user_123",
	})

	_, err := authenticateExternalSignedHeaders(req, externalAuthTestConfig(secret), now, newExternalAuthCache())
	assertExternalAuthError(t, err, "external_auth_timestamp_expired")
}

func TestAuthenticateExternalSignedHeadersRejectsExpiredClaims(t *testing.T) {
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	secret := "secret"
	req := signedExternalAuthRequest(t, secret, now, map[string]any{
		"sub":        "user_123",
		"expires_at": now.Add(-time.Second).Format(time.RFC3339),
	})

	_, err := authenticateExternalSignedHeaders(req, externalAuthTestConfig(secret), now, newExternalAuthCache())
	assertExternalAuthError(t, err, "external_auth_claims_expired")
}

func TestExternalAuthMiddlewareSetsContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	now := time.Now().UTC()
	secret := "external-test-secret"
	cfg := config.Default()
	cfg.Auth.Mode = config.AuthModeExternal
	cfg.Auth.External = externalAuthTestConfig(secret)
	holder := gwruntime.NewHolder(&cfg, nil)

	router := gin.New()
	router.Use(Auth(holder, nil, nil, nil, nil))
	router.GET("/protected", func(c *gin.Context) {
		auth := GetAuthContext(c)
		c.JSON(http.StatusOK, gin.H{
			"key_id":       auth.KeyID,
			"project_id":   auth.ProjectID,
			"is_admin":     auth.IsAdmin,
			"token_source": auth.TokenSource,
		})
	})

	req := signedExternalAuthRequest(t, secret, now, map[string]any{
		"sub":        "user_123",
		"project_id": "proj_123",
		"is_admin":   true,
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["key_id"] != "external:user_123" || body["project_id"] != "proj_123" || body["token_source"] != "external" || body["is_admin"] != true {
		t.Fatalf("unexpected middleware auth context response: %v", body)
	}
}

func externalAuthTestConfig(secret string) config.ExternalAuthConfig {
	return config.ExternalAuthConfig{
		Provider:     externalAuthProviderSigned,
		SharedSecret: secret,
		MaxClockSkew: time.Minute,
		CacheTTL:     time.Minute,
	}
}

func signedExternalAuthRequest(t *testing.T, secret string, now time.Time, claims map[string]any) *http.Request {
	t.Helper()

	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	encodedClaims := base64.RawURLEncoding.EncodeToString(claimsJSON)
	timestamp := strconv.FormatInt(now.Unix(), 10)
	signature := hex.EncodeToString(signExternalAuth(secret, timestamp, encodedClaims))

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set(externalAuthClaimsHeader, encodedClaims)
	req.Header.Set(externalAuthTimestampHeader, timestamp)
	req.Header.Set(externalAuthSignatureHeader, "v1="+signature)
	return req
}

func assertExternalAuthError(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s error, got nil", code)
	}
	var authErr externalAuthError
	if !errorsAsExternalAuth(err, &authErr) {
		t.Fatalf("expected externalAuthError, got %T: %v", err, err)
	}
	if authErr.code != code {
		t.Fatalf("expected error code %q, got %q", code, authErr.code)
	}
}

func errorsAsExternalAuth(err error, target *externalAuthError) bool {
	authErr, ok := err.(externalAuthError)
	if !ok {
		return false
	}
	*target = authErr
	return true
}
