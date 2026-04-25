package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/JiaCheng2004/Polaris/internal/store/cache"
)

func TestMultiUserAuthAndAdminKeyLifecycle(t *testing.T) {
	cfg := testConfig(t)
	cfg.Auth.Mode = config.AuthModeMultiUser
	cfg.Cache.RateLimit.Default = "100/min"

	sqliteStore := testSQLiteStore(t)
	adminRaw := "polaris-sk-live-admin-secret"
	adminKey := store.APIKey{
		ID:            "key_admin",
		Name:          "admin",
		KeyHash:       middleware.HashAPIKey(adminRaw),
		KeyPrefix:     "polaris-",
		RateLimit:     "100/min",
		AllowedModels: []string{"*"},
		IsAdmin:       true,
	}
	if err := sqliteStore.CreateAPIKey(t.Context(), adminKey); err != nil {
		t.Fatalf("CreateAPIKey(admin) error = %v", err)
	}

	registry, _, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}

	engine, err := NewEngine(Dependencies{
		Config:   cfg,
		Store:    sqliteStore,
		Cache:    cache.NewMemory(),
		Registry: registry,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	createBody := strings.NewReader(`{
		"name":"service-key",
		"owner_id":"svc-a",
		"allowed_models":["openai/*"]
	}`)
	createReq := httptest.NewRequest(http.MethodPost, "/v1/keys", createBody)
	createReq.Header.Set("Authorization", "Bearer "+adminRaw)
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	engine.ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusOK {
		t.Fatalf("expected create key 200, got %d body=%s", createRes.Code, createRes.Body.String())
	}

	var created struct {
		ID            string   `json:"id"`
		Key           string   `json:"key"`
		KeyPrefix     string   `json:"key_prefix"`
		AllowedModels []string `json:"allowed_models"`
		RateLimit     string   `json:"rate_limit"`
	}
	if err := json.Unmarshal(createRes.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.ID == "" || created.Key == "" {
		t.Fatalf("expected created id and key, got %#v", created)
	}
	if created.KeyPrefix != "polaris-" {
		t.Fatalf("expected key_prefix polaris-, got %q", created.KeyPrefix)
	}
	if created.RateLimit != "100/min" {
		t.Fatalf("expected default rate limit 100/min, got %q", created.RateLimit)
	}

	modelsReq := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	modelsReq.Header.Set("Authorization", "Bearer "+created.Key)
	modelsRes := httptest.NewRecorder()
	engine.ServeHTTP(modelsRes, modelsReq)
	if modelsRes.Code != http.StatusOK {
		t.Fatalf("expected created key to access /v1/models, got %d body=%s", modelsRes.Code, modelsRes.Body.String())
	}

	var listReady bool
	for range 20 {
		listReq := httptest.NewRequest(http.MethodGet, "/v1/keys", nil)
		listReq.Header.Set("Authorization", "Bearer "+adminRaw)
		listRes := httptest.NewRecorder()
		engine.ServeHTTP(listRes, listReq)
		if listRes.Code != http.StatusOK {
			t.Fatalf("expected list keys 200, got %d body=%s", listRes.Code, listRes.Body.String())
		}

		var listed struct {
			Object string `json:"object"`
			Data   []struct {
				ID         string     `json:"id"`
				KeyPrefix  string     `json:"key_prefix"`
				LastUsedAt *time.Time `json:"last_used_at"`
			} `json:"data"`
		}
		if err := json.Unmarshal(listRes.Body.Bytes(), &listed); err != nil {
			t.Fatalf("decode list response: %v", err)
		}
		if listed.Object != "list" || len(listed.Data) < 2 {
			t.Fatalf("unexpected list response %#v", listed)
		}

		for i := range listed.Data {
			if listed.Data[i].ID == created.ID && listed.Data[i].LastUsedAt != nil {
				listReady = true
				break
			}
		}
		if listReady {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !listReady {
		t.Fatalf("expected last_used_at to be populated after using created key")
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/keys/"+created.ID, nil)
	deleteReq.Header.Set("Authorization", "Bearer "+adminRaw)
	deleteRes := httptest.NewRecorder()
	engine.ServeHTTP(deleteRes, deleteReq)
	if deleteRes.Code != http.StatusNoContent {
		t.Fatalf("expected revoke key 204, got %d body=%s", deleteRes.Code, deleteRes.Body.String())
	}

	modelsReq = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	modelsReq.Header.Set("Authorization", "Bearer "+created.Key)
	modelsRes = httptest.NewRecorder()
	engine.ServeHTTP(modelsRes, modelsReq)
	if modelsRes.Code != http.StatusUnauthorized {
		t.Fatalf("expected revoked key to be unauthorized, got %d body=%s", modelsRes.Code, modelsRes.Body.String())
	}
	if !strings.Contains(modelsRes.Body.String(), `"code":"key_revoked"`) {
		t.Fatalf("expected key_revoked error, got %s", modelsRes.Body.String())
	}
}

func TestMultiUserNonAdminCannotManageKeys(t *testing.T) {
	cfg := testConfig(t)
	cfg.Auth.Mode = config.AuthModeMultiUser

	sqliteStore := testSQLiteStore(t)
	userRaw := "polaris-sk-live-user-secret"
	if err := sqliteStore.CreateAPIKey(t.Context(), store.APIKey{
		ID:            "key_user",
		Name:          "user",
		KeyHash:       middleware.HashAPIKey(userRaw),
		KeyPrefix:     "polaris-",
		RateLimit:     "100/min",
		AllowedModels: []string{"*"},
		IsAdmin:       false,
	}); err != nil {
		t.Fatalf("CreateAPIKey(user) error = %v", err)
	}

	registry, _, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}

	engine, err := NewEngine(Dependencies{
		Config:   cfg,
		Store:    sqliteStore,
		Cache:    cache.NewMemory(),
		Registry: registry,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/keys", nil)
	req.Header.Set("Authorization", "Bearer "+userRaw)
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected non-admin /v1/keys request to be forbidden, got %d body=%s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `"code":"admin_required"`) {
		t.Fatalf("expected admin_required error, got %s", res.Body.String())
	}
}

func TestMultiUserExpiredKeyRejected(t *testing.T) {
	cfg := testConfig(t)
	cfg.Auth.Mode = config.AuthModeMultiUser

	sqliteStore := testSQLiteStore(t)
	expiredRaw := "polaris-sk-live-expired-secret"
	expiredAt := time.Now().UTC().Add(-time.Hour)
	if err := sqliteStore.CreateAPIKey(t.Context(), store.APIKey{
		ID:            "key_expired",
		Name:          "expired",
		KeyHash:       middleware.HashAPIKey(expiredRaw),
		KeyPrefix:     "polaris-",
		RateLimit:     "100/min",
		AllowedModels: []string{"*"},
		ExpiresAt:     &expiredAt,
	}); err != nil {
		t.Fatalf("CreateAPIKey(expired) error = %v", err)
	}

	registry, _, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}

	engine, err := NewEngine(Dependencies{
		Config:   cfg,
		Store:    sqliteStore,
		Cache:    cache.NewMemory(),
		Registry: registry,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+expiredRaw)
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected expired key to be unauthorized, got %d body=%s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `"code":"key_expired"`) {
		t.Fatalf("expected key_expired error, got %s", res.Body.String())
	}
}

func TestKeysEndpointsRejectStaticAuthRuntime(t *testing.T) {
	cfg := testConfig(t)
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{
		{
			Name:          "static-admin",
			KeyHash:       middleware.HashAPIKey("static-secret"),
			RateLimit:     "100/min",
			AllowedModels: []string{"*"},
		},
	}

	sqliteStore := testSQLiteStore(t)
	registry, _, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}

	engine, err := NewEngine(Dependencies{
		Config:   cfg,
		Store:    sqliteStore,
		Cache:    cache.NewMemory(),
		Registry: registry,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/keys", nil)
	req.Header.Set("Authorization", "Bearer static-secret")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected static auth runtime to reject /v1/keys, got %d body=%s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `"code":"admin_required"`) {
		t.Fatalf("expected admin_required error, got %s", res.Body.String())
	}
}
