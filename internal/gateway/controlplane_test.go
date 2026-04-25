package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/JiaCheng2004/Polaris/internal/store/cache"
	"github.com/JiaCheng2004/Polaris/internal/tooling"
)

func TestVirtualKeyControlPlaneAndLocalMCP(t *testing.T) {
	cfg := testConfig(t)
	cfg.Auth.Mode = config.AuthModeVirtualKeys
	cfg.Auth.BootstrapAdminKeyHash = middleware.HashAPIKey("bootstrap-secret")
	cfg.ControlPlane.Enabled = true
	cfg.Tools.Enabled = true
	cfg.Tools.Local = map[string]config.LocalToolConfig{
		"echo": {Implementation: "echo"},
	}
	cfg.MCP.Enabled = true

	sqliteStore := testSQLiteStore(t)
	registry, warnings, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no registry warnings, got %v", warnings)
	}

	engine, err := NewEngine(Dependencies{
		Config:       cfg,
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:        sqliteStore,
		Cache:        cache.NewMemory(),
		Registry:     registry,
		ToolRegistry: tooling.NewRegistry(),
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	projectID := createProjectViaControlPlane(t, engine, "bootstrap-secret", "Acme")
	toolID := createToolViaControlPlane(t, engine, "bootstrap-secret")
	toolsetID := createToolsetViaControlPlane(t, engine, "bootstrap-secret", toolID)
	bindingID := createBindingViaControlPlane(t, engine, "bootstrap-secret", toolsetID)
	virtualKey := createVirtualKeyViaControlPlane(t, engine, "bootstrap-secret", projectID, []string{toolsetID}, []string{bindingID})
	blockedKey := createVirtualKeyViaControlPlane(t, engine, "bootstrap-secret", projectID, []string{"ts_other"}, []string{"mcp_other"})

	metaReq := httptest.NewRequest(http.MethodGet, "/mcp/"+bindingID, nil)
	metaReq.Header.Set("Authorization", "Bearer "+virtualKey)
	metaRes := httptest.NewRecorder()
	engine.ServeHTTP(metaRes, metaReq)
	if metaRes.Code != http.StatusOK {
		t.Fatalf("expected MCP metadata 200, got %d body=%s", metaRes.Code, metaRes.Body.String())
	}

	callReq := httptest.NewRequest(http.MethodPost, "/mcp/"+bindingID, bytes.NewBufferString(`{
		"jsonrpc":"2.0",
		"id":1,
		"method":"tools/call",
		"params":{"name":"echo","arguments":{"text":"hello"}}
	}`))
	callReq.Header.Set("Authorization", "Bearer "+virtualKey)
	callReq.Header.Set("Content-Type", "application/json")
	callRes := httptest.NewRecorder()
	engine.ServeHTTP(callRes, callReq)
	if callRes.Code != http.StatusOK {
		t.Fatalf("expected MCP tools/call 200, got %d body=%s", callRes.Code, callRes.Body.String())
	}
	if body := callRes.Body.String(); !bytes.Contains([]byte(body), []byte(`"hello"`)) {
		t.Fatalf("expected MCP tools/call result to contain echoed text, got %s", body)
	}

	forbiddenReq := httptest.NewRequest(http.MethodGet, "/mcp/"+bindingID, nil)
	forbiddenReq.Header.Set("Authorization", "Bearer "+blockedKey)
	forbiddenRes := httptest.NewRecorder()
	engine.ServeHTTP(forbiddenRes, forbiddenReq)
	if forbiddenRes.Code != http.StatusForbidden {
		t.Fatalf("expected MCP forbidden 403, got %d body=%s", forbiddenRes.Code, forbiddenRes.Body.String())
	}
	if !bytes.Contains(forbiddenRes.Body.Bytes(), []byte(`"mcp_binding_not_allowed"`)) {
		t.Fatalf("expected mcp_binding_not_allowed, got %s", forbiddenRes.Body.String())
	}
}

func TestVirtualKeyHardBudgetBlocksInferenceRequest(t *testing.T) {
	cfg := testConfig(t)
	cfg.Auth.Mode = config.AuthModeVirtualKeys
	cfg.Auth.BootstrapAdminKeyHash = middleware.HashAPIKey("bootstrap-secret")
	cfg.ControlPlane.Enabled = true

	sqliteStore := testSQLiteStore(t)
	project := store.Project{
		ID:        "proj_budget",
		Name:      "Budget Test",
		CreatedAt: time.Now().UTC(),
	}
	if err := sqliteStore.CreateProject(context.Background(), project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	virtualKeyRaw := "vk-budget-secret"
	key := store.VirtualKey{
		ID:            "vk_budget",
		ProjectID:     project.ID,
		Name:          "budget-key",
		KeyHash:       middleware.HashAPIKey(virtualKeyRaw),
		KeyPrefix:     "polaris-",
		AllowedModels: []string{"*"},
		CreatedAt:     time.Now().UTC(),
	}
	if err := sqliteStore.CreateVirtualKey(context.Background(), key); err != nil {
		t.Fatalf("CreateVirtualKey() error = %v", err)
	}
	if err := sqliteStore.CreateBudget(context.Background(), store.Budget{
		ID:            "bud_budget",
		ProjectID:     project.ID,
		Name:          "hard-cap",
		Mode:          store.BudgetModeHard,
		LimitRequests: 1,
		Window:        "monthly",
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateBudget() error = %v", err)
	}
	if err := sqliteStore.LogRequest(context.Background(), store.RequestLog{
		RequestID:  "req_budget",
		KeyID:      key.ID,
		ProjectID:  project.ID,
		Model:      "openai/gpt-4o",
		Modality:   modality.ModalityChat,
		StatusCode: http.StatusOK,
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("LogRequest() error = %v", err)
	}

	registry, warnings, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no registry warnings, got %v", warnings)
	}

	engine, err := NewEngine(Dependencies{
		Config:   cfg,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:    sqliteStore,
		Cache:    cache.NewMemory(),
		Registry: registry,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+virtualKeyRaw)
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected budget denial 403, got %d body=%s", res.Code, res.Body.String())
	}
	if !bytes.Contains(res.Body.Bytes(), []byte(`"budget_exceeded"`)) {
		t.Fatalf("expected budget_exceeded error, got %s", res.Body.String())
	}
}

func createProjectViaControlPlane(t *testing.T, engine http.Handler, adminKey string, name string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/projects", bytes.NewBufferString(`{"name":"`+name+`"}`))
	req.Header.Set("Authorization", "Bearer "+adminKey)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("create project: expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	var payload struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode project response: %v", err)
	}
	return payload.ID
}

func createToolViaControlPlane(t *testing.T, engine http.Handler, adminKey string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/tools", bytes.NewBufferString(`{
		"name":"Echo",
		"description":"Echo test tool",
		"implementation":"echo"
	}`))
	req.Header.Set("Authorization", "Bearer "+adminKey)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("create tool: expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	var payload struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	return payload.ID
}

func createToolsetViaControlPlane(t *testing.T, engine http.Handler, adminKey string, toolID string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"name":     "Core",
		"tool_ids": []string{toolID},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/toolsets", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminKey)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("create toolset: expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	var payload struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode toolset response: %v", err)
	}
	return payload.ID
}

func createBindingViaControlPlane(t *testing.T, engine http.Handler, adminKey string, toolsetID string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"name":       "Local Tools",
		"kind":       "local_toolset",
		"toolset_id": toolsetID,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/mcp/bindings", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminKey)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("create binding: expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	var payload struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode binding response: %v", err)
	}
	return payload.ID
}

func createVirtualKeyViaControlPlane(t *testing.T, engine http.Handler, adminKey string, projectID string, toolsetIDs []string, bindingIDs []string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"project_id":           projectID,
		"name":                 "Worker",
		"allowed_models":       []string{"*"},
		"allowed_toolsets":     toolsetIDs,
		"allowed_mcp_bindings": bindingIDs,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/virtual_keys", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminKey)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("create virtual key: expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	var payload struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode virtual key response: %v", err)
	}
	return payload.Key
}
