package qwen

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func TestImageAdapterGenerate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", r.Method)
		}
		if r.URL.Path != "/api/v1/services/aigc/multimodal-generation/generation" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer qwen-key" {
			t.Fatalf("unexpected Authorization header %q", got)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["model"] != "qwen-image-2.0" {
			t.Fatalf("unexpected model %#v", payload["model"])
		}
		messages := payload["input"].(map[string]any)["messages"].([]any)
		content := messages[0].(map[string]any)["content"].([]any)
		if len(content) != 2 {
			t.Fatalf("expected prompt + reference image, got %#v", content)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"output":{
				"choices":[{
					"message":{
						"content":[
							{"image":"https://example.com/generated.png"}
						]
					}
				}]
			}
		}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "qwen-key",
		BaseURL: server.URL + "/compatible-mode/v1",
		Timeout: time.Second,
	})
	adapter := NewImageAdapter(client, "qwen/qwen-image-2.0")

	response, err := adapter.Generate(context.Background(), &modality.ImageRequest{
		Model:           "qwen/qwen-image-2.0",
		Prompt:          "A cinematic street scene",
		ResponseFormat:  "url",
		ReferenceImages: []string{"ZmFrZS1pbWFnZQ=="},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(response.Data) != 1 || response.Data[0].URL != "https://example.com/generated.png" {
		t.Fatalf("unexpected image response %#v", response)
	}
}

func TestImageAdapterEditB64JSON(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/services/aigc/multimodal-generation/generation":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			messages := payload["input"].(map[string]any)["messages"].([]any)
			content := messages[0].(map[string]any)["content"].([]any)
			if len(content) != 3 {
				t.Fatalf("expected image + text + mask content, got %#v", content)
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"output":{
					"choices":[{
						"message":{
							"content":[
								{"image":"` + server.URL + `/result.png"}
							]
						}
					}]
				}
			}`))
		case "/result.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte{0x89, 'P', 'N', 'G'})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "qwen-key",
		BaseURL: server.URL + "/compatible-mode/v1",
		Timeout: time.Second,
	})
	adapter := NewImageAdapter(client, "qwen/qwen-image-2.0")

	response, err := adapter.Edit(context.Background(), &modality.ImageEditRequest{
		Model:          "qwen/qwen-image-2.0",
		Prompt:         "Replace the background",
		Image:          []byte("image-bytes"),
		ImageFilename:  "input.png",
		ImageType:      "image/png",
		Mask:           []byte("mask-bytes"),
		MaskFilename:   "mask.png",
		MaskType:       "image/png",
		ResponseFormat: "b64_json",
	})
	if err != nil {
		t.Fatalf("Edit() error = %v", err)
	}
	if len(response.Data) != 1 || response.Data[0].B64JSON != base64.StdEncoding.EncodeToString([]byte{0x89, 'P', 'N', 'G'}) {
		t.Fatalf("unexpected image response %#v", response)
	}
}
