package gateway

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/JiaCheng2004/Polaris/internal/store/cache"
)

func TestVideoGenerationsLifecycleAndUsage(t *testing.T) {
	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v3/contents/generations/tasks":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"task_123","status":"submitted","estimated_time":12}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/contents/generations/tasks/task_123":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"task_123","status":"succeeded","progress":100,"content":{"video_url":"` + upstream.URL + `/video.mp4","duration":8,"width":1920,"height":1080,"mime_type":"video/mp4"}}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v3/contents/generations/tasks/task_123":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/video.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("video-bytes"))
		default:
			t.Fatalf("unexpected upstream %s %s", r.Method, r.URL.Path)
		}
	}))
	defer upstream.Close()

	cfg := testVideoConfig(t, upstream.URL+"/api/v3")
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{{
		Name:          "alpha",
		KeyHash:       middleware.HashAPIKey("alpha-key"),
		RateLimit:     "10/min",
		AllowedModels: []string{"bytedance/*"},
	}}

	sqliteStore := testSQLiteStore(t)
	registry, warnings, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	requestLogger := store.NewAsyncRequestLogger(sqliteStore, slog.New(slog.NewTextHandler(io.Discard, nil)), store.NewLoggerConfig(10, 5*time.Millisecond))

	engine, err := NewEngine(Dependencies{
		Config:        cfg,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:         sqliteStore,
		Cache:         cache.NewMemory(),
		Registry:      registry,
		RequestLogger: requestLogger,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	submitReq := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(`{"model":"default-video","prompt":"A cinematic skyline","duration":8,"resolution":"720p"}`))
	submitReq.Header.Set("Authorization", "Bearer alpha-key")
	submitReq.Header.Set("Content-Type", "application/json")
	submitRes := httptest.NewRecorder()
	engine.ServeHTTP(submitRes, submitReq)
	if submitRes.Code != http.StatusOK {
		t.Fatalf("expected submit 200, got %d body=%s", submitRes.Code, submitRes.Body.String())
	}

	var submitBody struct {
		JobID  string `json:"job_id"`
		Status string `json:"status"`
		Model  string `json:"model"`
	}
	if err := json.Unmarshal(submitRes.Body.Bytes(), &submitBody); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}
	if !strings.HasPrefix(submitBody.JobID, "vid_") || submitBody.Status != "queued" || submitBody.Model != "bytedance/seedance-2.0" {
		t.Fatalf("unexpected submit response %#v", submitBody)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/video/generations/"+submitBody.JobID, nil)
	getReq.Header.Set("Authorization", "Bearer alpha-key")
	getRes := httptest.NewRecorder()
	engine.ServeHTTP(getRes, getReq)
	if getRes.Code != http.StatusOK {
		t.Fatalf("expected get 200, got %d body=%s", getRes.Code, getRes.Body.String())
	}

	var getBody struct {
		JobID  string `json:"job_id"`
		Status string `json:"status"`
		Result struct {
			VideoURL    string `json:"video_url"`
			DownloadURL string `json:"download_url"`
		} `json:"result"`
	}
	if err := json.Unmarshal(getRes.Body.Bytes(), &getBody); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if getBody.JobID != submitBody.JobID || getBody.Status != "completed" || getBody.Result.VideoURL == "" || getBody.Result.DownloadURL == "" {
		t.Fatalf("unexpected get response %#v", getBody)
	}

	contentReq := httptest.NewRequest(http.MethodGet, "/v1/video/generations/"+submitBody.JobID+"/content", nil)
	contentReq.Header.Set("Authorization", "Bearer alpha-key")
	contentRes := httptest.NewRecorder()
	engine.ServeHTTP(contentRes, contentReq)
	if contentRes.Code != http.StatusOK || contentRes.Body.String() != "video-bytes" {
		t.Fatalf("expected content 200 with bytes, got %d body=%s", contentRes.Code, contentRes.Body.String())
	}

	cancelReq := httptest.NewRequest(http.MethodDelete, "/v1/video/generations/"+submitBody.JobID, nil)
	cancelReq.Header.Set("Authorization", "Bearer alpha-key")
	cancelRes := httptest.NewRecorder()
	engine.ServeHTTP(cancelRes, cancelReq)
	if cancelRes.Code != http.StatusNoContent {
		t.Fatalf("expected cancel 204, got %d body=%s", cancelRes.Code, cancelRes.Body.String())
	}

	if err := requestLogger.Close(context.Background()); err != nil {
		t.Fatalf("requestLogger.Close() error = %v", err)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/v1/usage?group_by=model", nil)
	usageReq.Header.Set("Authorization", "Bearer alpha-key")
	usageRes := httptest.NewRecorder()
	engine.ServeHTTP(usageRes, usageReq)
	if usageRes.Code != http.StatusOK {
		t.Fatalf("expected usage 200, got %d body=%s", usageRes.Code, usageRes.Body.String())
	}

	var usageBody struct {
		TotalRequests int64 `json:"total_requests"`
		ByModel       []struct {
			Model string `json:"model"`
		} `json:"by_model"`
	}
	if err := json.Unmarshal(usageRes.Body.Bytes(), &usageBody); err != nil {
		t.Fatalf("decode usage response: %v", err)
	}
	if usageBody.TotalRequests != 1 {
		t.Fatalf("expected only submit to be logged, got %#v", usageBody)
	}
	if len(usageBody.ByModel) != 1 || usageBody.ByModel[0].Model != "bytedance/seedance-2.0" {
		t.Fatalf("unexpected usage by model %#v", usageBody.ByModel)
	}
}

func TestVideoGenerationsEnforceJobOwnershipAndTokenValidity(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v3/contents/generations/tasks" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"task_123","status":"submitted"}`))
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/v3/contents/generations/tasks/task_123" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"task_123","status":"queued"}`))
			return
		}
		t.Fatalf("unexpected upstream %s %s", r.Method, r.URL.Path)
	}))
	defer upstream.Close()

	cfg := testVideoConfig(t, upstream.URL+"/api/v3")
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{
		{
			Name:          "alpha",
			KeyHash:       middleware.HashAPIKey("alpha-key"),
			RateLimit:     "10/min",
			AllowedModels: []string{"bytedance/*"},
		},
		{
			Name:          "beta",
			KeyHash:       middleware.HashAPIKey("beta-key"),
			RateLimit:     "10/min",
			AllowedModels: []string{"bytedance/*"},
		},
	}

	engine := newTestEngine(t, cfg)

	submitReq := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(`{"model":"default-video","prompt":"A cinematic skyline"}`))
	submitReq.Header.Set("Authorization", "Bearer alpha-key")
	submitReq.Header.Set("Content-Type", "application/json")
	submitRes := httptest.NewRecorder()
	engine.ServeHTTP(submitRes, submitReq)
	if submitRes.Code != http.StatusOK {
		t.Fatalf("expected submit 200, got %d body=%s", submitRes.Code, submitRes.Body.String())
	}

	var submitBody struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(submitRes.Body.Bytes(), &submitBody); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}

	wrongOwnerReq := httptest.NewRequest(http.MethodGet, "/v1/video/generations/"+submitBody.JobID, nil)
	wrongOwnerReq.Header.Set("Authorization", "Bearer beta-key")
	wrongOwnerRes := httptest.NewRecorder()
	engine.ServeHTTP(wrongOwnerRes, wrongOwnerReq)
	if wrongOwnerRes.Code != http.StatusForbidden || !strings.Contains(wrongOwnerRes.Body.String(), "job_not_owned") {
		t.Fatalf("expected forbidden job ownership error, got %d body=%s", wrongOwnerRes.Code, wrongOwnerRes.Body.String())
	}

	badTokenReq := httptest.NewRequest(http.MethodGet, "/v1/video/generations/vid_invalid", nil)
	badTokenReq.Header.Set("Authorization", "Bearer alpha-key")
	badTokenRes := httptest.NewRecorder()
	engine.ServeHTTP(badTokenRes, badTokenReq)
	if badTokenRes.Code != http.StatusNotFound || !strings.Contains(badTokenRes.Body.String(), "job_not_found") {
		t.Fatalf("expected not found for invalid token, got %d body=%s", badTokenRes.Code, badTokenRes.Body.String())
	}
}

func TestVideoGenerationsValidateCapabilitiesAndResolution(t *testing.T) {
	cfg := testVideoConfig(t, "https://ark.cn-beijing.volces.com/api/v3")
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{{
		Name:          "alpha",
		KeyHash:       middleware.HashAPIKey("alpha-key"),
		RateLimit:     "10/min",
		AllowedModels: []string{"bytedance/*"},
	}}
	cfg.Providers["bytedance"] = config.ProviderConfig{
		APIKey:  "ark-key",
		BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
		Timeout: time.Second,
		Models: map[string]config.ModelConfig{
			"seedance-2.0": {
				Modality:     modality.ModalityVideo,
				Capabilities: []modality.Capability{modality.CapabilityTextToVideo},
				MaxDuration:  15,
				Resolutions:  []string{"720p"},
				Cancelable:   true,
				Endpoint:     "https://ark.cn-beijing.volces.com/api/v3",
			},
		},
	}

	engine := newTestEngine(t, cfg)

	cases := []struct {
		name string
		body string
		code int
		want string
	}{
		{
			name: "resolution",
			body: `{"model":"default-video","prompt":"hello","resolution":"1080p"}`,
			code: http.StatusBadRequest,
			want: "unsupported_resolution",
		},
		{
			name: "first_frame_capability",
			body: `{"model":"default-video","prompt":"hello","first_frame":"https://example.com/frame.png"}`,
			code: http.StatusBadRequest,
			want: "capability_missing",
		},
		{
			name: "audio_capability",
			body: `{"model":"default-video","prompt":"hello","with_audio":true}`,
			code: http.StatusBadRequest,
			want: "capability_missing",
		},
		{
			name: "reference_video_capability",
			body: `{"model":"default-video","prompt":"hello","reference_videos":["asset://video_123"]}`,
			code: http.StatusBadRequest,
			want: "capability_missing",
		},
		{
			name: "audio_input_capability",
			body: `{"model":"default-video","prompt":"hello","reference_images":["https://example.com/ref.png"],"audio":"asset://audio_123"}`,
			code: http.StatusBadRequest,
			want: "capability_missing",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer alpha-key")
			req.Header.Set("Content-Type", "application/json")
			res := httptest.NewRecorder()
			engine.ServeHTTP(res, req)
			if res.Code != tc.code || !strings.Contains(res.Body.String(), tc.want) {
				t.Fatalf("expected %d containing %q, got %d body=%s", tc.code, tc.want, res.Code, res.Body.String())
			}
		})
	}
}

func TestVideoGenerationsValidateParityInputs(t *testing.T) {
	cfg := testVideoConfig(t, "https://ark.cn-beijing.volces.com/api/v3")
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{{
		Name:          "alpha",
		KeyHash:       middleware.HashAPIKey("alpha-key"),
		RateLimit:     "10/min",
		AllowedModels: []string{"bytedance/*"},
	}}

	engine := newTestEngine(t, cfg)

	cases := []struct {
		name string
		body string
		code int
		want string
	}{
		{
			name: "last_frame_requires_first_frame",
			body: `{"model":"default-video","prompt":"hello","last_frame":"https://example.com/end.png"}`,
			code: http.StatusBadRequest,
			want: "missing_first_frame",
		},
		{
			name: "first_frame_conflicts_with_reference_images",
			body: `{"model":"default-video","prompt":"hello","first_frame":"https://example.com/start.png","reference_images":["https://example.com/ref.png"]}`,
			code: http.StatusBadRequest,
			want: "conflicting_inputs",
		},
		{
			name: "first_frame_conflicts_with_reference_videos",
			body: `{"model":"default-video","prompt":"hello","first_frame":"https://example.com/start.png","reference_videos":["asset://video_123"]}`,
			code: http.StatusBadRequest,
			want: "conflicting_inputs",
		},
		{
			name: "first_frame_conflicts_with_audio",
			body: `{"model":"default-video","prompt":"hello","first_frame":"https://example.com/start.png","audio":"asset://audio_123"}`,
			code: http.StatusBadRequest,
			want: "conflicting_inputs",
		},
		{
			name: "audio_requires_reference_media",
			body: `{"model":"default-video","prompt":"hello","audio":"asset://audio_123"}`,
			code: http.StatusBadRequest,
			want: "invalid_audio",
		},
		{
			name: "empty_reference_video",
			body: `{"model":"default-video","prompt":"hello","reference_videos":["  "]}`,
			code: http.StatusBadRequest,
			want: "invalid_reference_videos",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer alpha-key")
			req.Header.Set("Content-Type", "application/json")
			res := httptest.NewRecorder()
			engine.ServeHTTP(res, req)
			if res.Code != tc.code || !strings.Contains(res.Body.String(), tc.want) {
				t.Fatalf("expected %d containing %q, got %d body=%s", tc.code, tc.want, res.Code, res.Body.String())
			}
		})
	}
}

func TestVideoGenerationsRejectNonCancelableModels(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/videos" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"video_123","status":"queued","model":"sora-2"}`))
			return
		}
		t.Fatalf("unexpected upstream %s %s", r.Method, r.URL.Path)
	}))
	defer upstream.Close()

	cfg := testConfig(t)
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{{
		Name:          "alpha",
		KeyHash:       middleware.HashAPIKey("alpha-key"),
		RateLimit:     "10/min",
		AllowedModels: []string{"openai/*"},
	}}
	cfg.Providers = map[string]config.ProviderConfig{
		"openai": {
			APIKey:  "sk-openai",
			BaseURL: upstream.URL,
			Timeout: time.Second,
			Models: map[string]config.ModelConfig{
				"sora-2": {
					Modality:         modality.ModalityVideo,
					Capabilities:     []modality.Capability{modality.CapabilityTextToVideo, modality.CapabilityImageToVideo, modality.CapabilityNativeAudio},
					AllowedDurations: []int{4, 8, 12},
					AspectRatios:     []string{"16:9", "9:16"},
					Resolutions:      []string{"720p"},
					Cancelable:       false,
				},
			},
		},
	}
	cfg.Routing.Aliases = map[string]string{
		"default-video": "openai/sora-2",
	}

	engine := newTestEngine(t, cfg)

	submitReq := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(`{"model":"default-video","prompt":"hello","duration":8,"aspect_ratio":"16:9","resolution":"720p"}`))
	submitReq.Header.Set("Authorization", "Bearer alpha-key")
	submitReq.Header.Set("Content-Type", "application/json")
	submitRes := httptest.NewRecorder()
	engine.ServeHTTP(submitRes, submitReq)
	if submitRes.Code != http.StatusOK {
		t.Fatalf("expected submit 200, got %d body=%s", submitRes.Code, submitRes.Body.String())
	}

	var submitBody struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(submitRes.Body.Bytes(), &submitBody); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}

	cancelReq := httptest.NewRequest(http.MethodDelete, "/v1/video/generations/"+submitBody.JobID, nil)
	cancelReq.Header.Set("Authorization", "Bearer alpha-key")
	cancelRes := httptest.NewRecorder()
	engine.ServeHTTP(cancelRes, cancelReq)
	if cancelRes.Code != http.StatusConflict || !strings.Contains(cancelRes.Body.String(), "job_not_cancelable") {
		t.Fatalf("expected non-cancelable 409, got %d body=%s", cancelRes.Code, cancelRes.Body.String())
	}
}

func testVideoConfig(t *testing.T, baseURL string) *config.Config {
	t.Helper()
	cfg := testConfig(t)
	cfg.Providers = map[string]config.ProviderConfig{
		"bytedance": {
			APIKey:  "ark-key",
			BaseURL: baseURL,
			Timeout: time.Second,
			Models: map[string]config.ModelConfig{
				"seedance-2.0": {
					Modality:         modality.ModalityVideo,
					Capabilities:     []modality.Capability{modality.CapabilityTextToVideo, modality.CapabilityImageToVideo, modality.CapabilityLastFrame, modality.CapabilityReferenceImages, modality.CapabilityVideoInput, modality.CapabilityAudioInput, modality.CapabilityNativeAudio},
					MaxDuration:      15,
					AllowedDurations: []int{4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
					Resolutions:      []string{"720p"},
					Cancelable:       true,
					Endpoint:         baseURL,
				},
			},
		},
	}
	cfg.Routing.Aliases = map[string]string{
		"default-video": "bytedance/seedance-2.0",
	}
	return cfg
}
