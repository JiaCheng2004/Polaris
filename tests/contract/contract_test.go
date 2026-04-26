package contract_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/JiaCheng2004/Polaris/internal/store/cache"
	"github.com/JiaCheng2004/Polaris/internal/store/sqlite"
	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

var openAPIMethods = map[string]struct{}{
	http.MethodGet:     {},
	http.MethodPost:    {},
	http.MethodPut:     {},
	http.MethodPatch:   {},
	http.MethodDelete:  {},
	http.MethodHead:    {},
	http.MethodOptions: {},
	http.MethodTrace:   {},
}

type openAPIDocument struct {
	OpenAPI    string                    `yaml:"openapi"`
	Info       map[string]any            `yaml:"info"`
	Paths      map[string]map[string]any `yaml:"paths"`
	Components openAPIComponents         `yaml:"components"`
}

type openAPIComponents struct {
	Schemas map[string]any `yaml:"schemas"`
}

type contractEngine struct {
	handler http.Handler
	store   store.Store
}

type contractOptions struct {
	openAIBaseURL       string
	authMode            config.AuthMode
	bootstrapAdminKey   string
	controlPlaneEnabled bool
}

func init() {
	gin.SetMode(gin.ReleaseMode)
}

func TestOpenAPIContractParsesAndDefinesSharedSchemas(t *testing.T) {
	doc := loadOpenAPI(t)
	if !strings.HasPrefix(doc.OpenAPI, "3.1.") {
		t.Fatalf("expected OpenAPI 3.1.x, got %q", doc.OpenAPI)
	}
	if len(doc.Paths) == 0 {
		t.Fatal("expected OpenAPI paths")
	}

	requiredSchemas := []string{
		"ErrorEnvelope",
		"Usage",
		"Routing",
		"ListResponse",
		"ChatCompletion",
		"AsyncJob",
		"SessionDescriptor",
		"Project",
		"UsageReport",
	}
	for _, schema := range requiredSchemas {
		if _, ok := doc.Components.Schemas[schema]; !ok {
			t.Fatalf("missing OpenAPI schema %q", schema)
		}
	}
}

func TestRegisteredRoutesMatchOpenAPIContract(t *testing.T) {
	doc := loadOpenAPI(t)
	contract := newContractEngine(t, contractOptions{})
	engine, ok := contract.handler.(*gin.Engine)
	if !ok {
		t.Fatalf("expected gin engine, got %T", contract.handler)
	}

	specRoutes := map[string]struct{}{}
	for path, pathItem := range doc.Paths {
		for method := range pathItem {
			upper := strings.ToUpper(method)
			if _, ok := openAPIMethods[upper]; ok {
				specRoutes[upper+" "+path] = struct{}{}
			}
		}
	}

	actualRoutes := map[string]struct{}{}
	for _, route := range engine.Routes() {
		if route.Method == http.MethodConnect {
			continue
		}
		actualRoutes[route.Method+" "+normalizeGinPath(route.Path)] = struct{}{}
	}

	if missing := missingRoutes(actualRoutes, specRoutes); len(missing) > 0 {
		t.Fatalf("registered routes missing from OpenAPI contract:\n%s", strings.Join(missing, "\n"))
	}
	if missing := missingRoutes(specRoutes, actualRoutes); len(missing) > 0 {
		t.Fatalf("OpenAPI routes not registered by gateway:\n%s", strings.Join(missing, "\n"))
	}
}

func TestOpenAPIPathsRemainDocumentedInAPIReference(t *testing.T) {
	doc := loadOpenAPI(t)
	reference := readAPIReference(t)

	var missing []string
	for path := range doc.Paths {
		if !apiReferenceMentionsPath(reference, path) {
			missing = append(missing, path)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("OpenAPI paths missing from docs/API_REFERENCE.md:\n%s", strings.Join(missing, "\n"))
	}
}

func TestGoldenHTTPContracts(t *testing.T) {
	openAI := newFakeOpenAI(t)

	chatContract := newContractEngine(t, contractOptions{
		openAIBaseURL: openAI.URL + "/v1",
	})
	chatReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{
		"model":"openai/gpt-4o",
		"messages":[{"role":"user","content":"contract"}]
	}`))
	chatReq.Header.Set("Content-Type", "application/json")
	chatRes := httptest.NewRecorder()
	chatContract.handler.ServeHTTP(chatRes, chatReq)
	assertStatus(t, chatRes, http.StatusOK)
	assertJSONFixture(t, chatRes.Body.Bytes(), "chat_completion.json")

	disabledContract := newContractEngine(t, contractOptions{
		authMode:            config.AuthModeVirtualKeys,
		bootstrapAdminKey:   "bootstrap-secret",
		controlPlaneEnabled: false,
	})
	disabledReq := httptest.NewRequest(http.MethodPost, "/v1/projects", bytes.NewBufferString(`{"name":"Acme"}`))
	disabledReq.Header.Set("Authorization", "Bearer bootstrap-secret")
	disabledReq.Header.Set("Content-Type", "application/json")
	disabledRes := httptest.NewRecorder()
	disabledContract.handler.ServeHTTP(disabledRes, disabledReq)
	assertStatus(t, disabledRes, http.StatusNotFound)
	assertJSONFixture(t, disabledRes.Body.Bytes(), "control_plane_disabled.json")

	budgetContract := newContractEngine(t, contractOptions{
		authMode:            config.AuthModeVirtualKeys,
		bootstrapAdminKey:   "bootstrap-secret",
		controlPlaneEnabled: true,
	})
	budgetKey := seedHardBudget(t, budgetContract.store)
	budgetReq := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	budgetReq.Header.Set("Authorization", "Bearer "+budgetKey)
	budgetRes := httptest.NewRecorder()
	budgetContract.handler.ServeHTTP(budgetRes, budgetReq)
	assertStatus(t, budgetRes, http.StatusTooManyRequests)
	assertJSONFixture(t, budgetRes.Body.Bytes(), "budget_exceeded.json")
}

func TestStreamingContractFixture(t *testing.T) {
	openAI := newFakeOpenAI(t)
	contract := newContractEngine(t, contractOptions{
		openAIBaseURL: openAI.URL + "/v1",
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{
		"model":"openai/gpt-4o",
		"stream":true,
		"messages":[{"role":"user","content":"contract"}]
	}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	contract.handler.ServeHTTP(res, req)
	assertStatus(t, res, http.StatusOK)

	actual := normalizeSSE(res.Body.String())
	expected := normalizeSSE(loadFixture(t, "chat_stream.sse"))
	if actual != expected {
		t.Fatalf("stream fixture mismatch\nactual:\n%s\nexpected:\n%s", actual, expected)
	}
}

func TestStableErrorCodesRemainDocumented(t *testing.T) {
	docs := readPublicDocs(t)
	for _, code := range []string{
		"model_not_allowed",
		"budget_exceeded",
		"control_plane_disabled",
		"provider_timeout",
		"job_not_found",
	} {
		if !strings.Contains(docs, code) {
			t.Fatalf("stable error code %q is not documented", code)
		}
	}
}

func newContractEngine(t *testing.T, opts contractOptions) contractEngine {
	t.Helper()

	cfg := config.Default()
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = 0
	if opts.authMode != "" {
		cfg.Auth.Mode = opts.authMode
	}
	cfg.Cache.RateLimit.Enabled = false
	cfg.Cache.ResponseCache.Enabled = false
	cfg.ControlPlane.Enabled = opts.controlPlaneEnabled
	cfg.Observability.Logging.Level = "error"
	cfg.Observability.Logging.Format = "json"
	cfg.Observability.Metrics.Path = "/metrics"
	cfg.Store.DSN = filepath.Join(t.TempDir(), "polaris.db")
	cfg.Providers = map[string]config.ProviderConfig{}
	cfg.Routing.Aliases = map[string]string{}

	if opts.bootstrapAdminKey != "" {
		cfg.Auth.BootstrapAdminKeyHash = middleware.HashAPIKey(opts.bootstrapAdminKey)
	}
	if opts.openAIBaseURL != "" {
		cfg.Providers["openai"] = config.ProviderConfig{
			APIKey:  "sk-contract",
			BaseURL: opts.openAIBaseURL,
			Timeout: 5 * time.Second,
			Models: map[string]config.ModelConfig{
				"gpt-4o": {
					Modality:        modality.ModalityChat,
					Capabilities:    []modality.Capability{modality.CapabilityStreaming, modality.CapabilityJSONMode},
					ContextWindow:   128000,
					MaxOutputTokens: 4096,
				},
			},
		}
	}

	appStore := newSQLiteStore(t, cfg.Store)
	registry, warnings, err := provider.New(&cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no provider warnings, got %v", warnings)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	engine, err := gateway.NewEngine(gateway.Dependencies{
		Config:   &cfg,
		Logger:   logger,
		Store:    appStore,
		Cache:    cache.NewMemory(),
		Registry: registry,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	return contractEngine{handler: engine, store: appStore}
}

func newSQLiteStore(t *testing.T, cfg config.StoreConfig) store.Store {
	t.Helper()

	cfg.Driver = "sqlite"
	cfg.MaxConnections = 1
	cfg.LogRetentionDays = 30
	cfg.LogBufferSize = 100
	cfg.LogFlushInterval = time.Second
	appStore, err := sqlite.New(cfg)
	if err != nil {
		t.Fatalf("sqlite.New() error = %v", err)
	}
	if err := appStore.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	t.Cleanup(func() {
		_ = appStore.Close()
	})
	return appStore
}

func newFakeOpenAI(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-contract" {
			t.Fatalf("unexpected OpenAI authorization header %q", got)
		}

		var req struct {
			Model  string `json:"model"`
			Stream bool   `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode fake OpenAI request: %v", err)
		}
		if req.Model != "gpt-4o" {
			t.Fatalf("expected provider model gpt-4o, got %q", req.Model)
		}

		if req.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, strings.ReplaceAll(loadFixture(t, "chat_stream.sse"), `"model":"openai/gpt-4o"`, `"model":"gpt-4o"`))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, strings.ReplaceAll(loadFixture(t, "chat_completion.json"), `"model": "openai/gpt-4o"`, `"model": "gpt-4o"`))
	}))
	t.Cleanup(server.Close)
	return server
}

func seedHardBudget(t *testing.T, appStore store.Store) string {
	t.Helper()

	ctx := context.Background()
	project := store.Project{
		ID:        "proj_contract_budget",
		Name:      "Contract Budget",
		CreatedAt: time.Now().UTC(),
	}
	if err := appStore.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	rawKey := "vk-contract-budget"
	key := store.VirtualKey{
		ID:            "vk_contract_budget",
		ProjectID:     project.ID,
		Name:          "budget-key",
		KeyHash:       middleware.HashAPIKey(rawKey),
		KeyPrefix:     "polaris-",
		AllowedModels: []string{"*"},
		CreatedAt:     time.Now().UTC(),
	}
	if err := appStore.CreateVirtualKey(ctx, key); err != nil {
		t.Fatalf("CreateVirtualKey() error = %v", err)
	}
	if err := appStore.CreateBudget(ctx, store.Budget{
		ID:            "bud_contract_budget",
		ProjectID:     project.ID,
		Name:          "hard-cap",
		Mode:          store.BudgetModeHard,
		LimitRequests: 1,
		Window:        "monthly",
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateBudget() error = %v", err)
	}
	if err := appStore.LogRequest(ctx, store.RequestLog{
		RequestID:  "req_contract_budget",
		KeyID:      key.ID,
		ProjectID:  project.ID,
		Model:      "openai/gpt-4o",
		Modality:   modality.ModalityChat,
		StatusCode: http.StatusOK,
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("LogRequest() error = %v", err)
	}
	return rawKey
}

func loadOpenAPI(t *testing.T) openAPIDocument {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "spec", "openapi", "polaris.v1.yaml"))
	if err != nil {
		t.Fatalf("read OpenAPI contract: %v", err)
	}
	var doc openAPIDocument
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode OpenAPI contract: %v", err)
	}
	return doc
}

func loadFixture(t *testing.T, name string) string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "tests", "contract", "fixtures", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(raw)
}

func readAPIReference(t *testing.T) string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "docs", "API_REFERENCE.md"))
	if err != nil {
		t.Fatalf("read docs/API_REFERENCE.md: %v", err)
	}
	return string(raw)
}

func assertJSONFixture(t *testing.T, actual []byte, fixture string) {
	t.Helper()

	actualJSON := canonicalJSON(t, actual)
	expectedJSON := canonicalJSON(t, []byte(loadFixture(t, fixture)))
	if actualJSON != expectedJSON {
		t.Fatalf("JSON fixture %s mismatch\nactual:\n%s\nexpected:\n%s", fixture, actualJSON, expectedJSON)
	}
}

func canonicalJSON(t *testing.T, raw []byte) string {
	t.Helper()

	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode JSON: %v\nbody=%s", err, string(raw))
	}
	encoded, err := json.MarshalIndent(decoded, "", "  ")
	if err != nil {
		t.Fatalf("encode canonical JSON: %v", err)
	}
	return string(encoded)
}

func assertStatus(t *testing.T, res *httptest.ResponseRecorder, expected int) {
	t.Helper()

	if res.Code != expected {
		t.Fatalf("expected HTTP %d, got %d body=%s", expected, res.Code, res.Body.String())
	}
}

func normalizeSSE(value string) string {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	var kept []string
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		if trimmed == "" && (len(kept) == 0 || kept[len(kept)-1] == "") {
			continue
		}
		kept = append(kept, trimmed)
	}
	for len(kept) > 0 && kept[len(kept)-1] == "" {
		kept = kept[:len(kept)-1]
	}
	return strings.Join(kept, "\n")
}

func normalizeGinPath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		switch {
		case strings.HasPrefix(part, ":"):
			parts[i] = "{" + strings.TrimPrefix(part, ":") + "}"
		case strings.HasPrefix(part, "*"):
			parts[i] = "{" + strings.TrimPrefix(part, "*") + "}"
		}
	}
	return strings.Join(parts, "/")
}

func apiReferenceMentionsPath(reference, path string) bool {
	for _, candidate := range apiReferencePathCandidates(path) {
		if strings.Contains(reference, candidate) {
			return true
		}
	}
	return false
}

func apiReferencePathCandidates(path string) []string {
	colonPath := strings.NewReplacer(
		"{id}", ":id",
		"{binding_id}", ":binding_id",
		"{path}", "*path",
	).Replace(path)
	plainParamPath := strings.NewReplacer(
		"{id}", "id",
		"{binding_id}", "binding_id",
		"{path}", "path",
	).Replace(path)
	return []string{path, colonPath, plainParamPath}
}

func missingRoutes(left, right map[string]struct{}) []string {
	var missing []string
	for route := range left {
		if _, ok := right[route]; !ok {
			missing = append(missing, route)
		}
	}
	sort.Strings(missing)
	return missing
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func readPublicDocs(t *testing.T) string {
	t.Helper()

	var builder strings.Builder
	for _, name := range []string{
		"README.md",
		filepath.Join("docs", "ARCHITECTURE.md"),
		filepath.Join("docs", "API_REFERENCE.md"),
	} {
		raw, err := os.ReadFile(filepath.Join(repoRoot(t), name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		builder.Write(raw)
		builder.WriteByte('\n')
	}
	return builder.String()
}
