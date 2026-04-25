package googlevertex

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JiaCheng2004/Polaris/internal/modality"
	"golang.org/x/oauth2"
)

func TestVideoAdapterGenerateStatusDownloadAndCancel(t *testing.T) {
	videoBytes := base64.StdEncoding.EncodeToString([]byte("video-bytes"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/frame.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("png-bytes"))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/projects/test-project/locations/us-central1/publishers/google/models/veo-3.1-generate-001@default:predictLongRunning":
			if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
				t.Fatalf("unexpected auth header %q", got)
			}
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode generate request: %v", err)
			}
			parameters := payload["parameters"].(map[string]any)
			if parameters["durationSeconds"] != float64(8) || parameters["generateAudio"] != true || parameters["resolution"] != "720p" {
				t.Fatalf("unexpected parameters %#v", parameters)
			}
			instances := payload["instances"].([]any)
			instance := instances[0].(map[string]any)
			if _, ok := instance["image"].(map[string]any); !ok {
				t.Fatalf("missing image payload %#v", instance)
			}
			if _, ok := instance["lastFrame"].(map[string]any); !ok {
				t.Fatalf("missing lastFrame payload %#v", instance)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"projects/test-project/locations/us-central1/publishers/google/models/veo-3.1-generate-001@default/operations/op_123"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/projects/test-project/locations/us-central1/publishers/google/models/veo-3.1-generate-001@default:fetchPredictOperation":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"projects/test-project/locations/us-central1/publishers/google/models/veo-3.1-generate-001@default/operations/op_123","done":true,"response":{"videos":[{"bytesBase64Encoded":"` + videoBytes + `","mimeType":"video/mp4"}]}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/projects/test-project/locations/us-central1/publishers/google/models/veo-3.1-generate-001@default/operations/op_123:cancel":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := &Client{
		baseURL:     server.URL,
		projectID:   "test-project",
		location:    "us-central1",
		httpClient:  server.Client(),
		tokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test-token"}),
	}
	adapter := NewVideoAdapter(client, "google-vertex/veo-3.1-generate-001")

	job, err := adapter.Generate(context.Background(), &modality.VideoRequest{
		Model:       "google-vertex/veo-3.1-generate-001",
		Prompt:      "A drifting paper lantern over water",
		Duration:    8,
		AspectRatio: "16:9",
		Resolution:  "720p",
		FirstFrame:  server.URL + "/frame.png",
		LastFrame:   "data:image/png;base64,Zm9v",
		WithAudio:   true,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if job.JobID == "" || job.Status != "queued" {
		t.Fatalf("unexpected job %#v", job)
	}

	status, err := adapter.GetStatus(context.Background(), job.JobID)
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if status.Status != "completed" || status.Result == nil || status.Result.ContentType != "video/mp4" {
		t.Fatalf("unexpected status %#v", status)
	}

	asset, err := adapter.Download(context.Background(), job.JobID, status)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if string(asset.Data) != "video-bytes" || asset.ContentType != "video/mp4" {
		t.Fatalf("unexpected asset %#v", asset)
	}

	if err := adapter.Cancel(context.Background(), job.JobID); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
}
