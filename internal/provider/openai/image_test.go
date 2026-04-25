package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func TestImageAdapterGenerate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Fatalf("unexpected Authorization header %q", got)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["model"] != "gpt-image-1" {
			t.Fatalf("expected stripped provider model, got %#v", payload["model"])
		}
		if _, ok := payload["response_format"]; ok {
			t.Fatalf("expected gpt-image request to omit response_format, got %#v", payload["response_format"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"created":1744329600,
			"data":[{"b64_json":"AQID","revised_prompt":"A lighthouse at dusk"}]
		}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "sk-test",
		BaseURL: server.URL + "/v1",
		Timeout: time.Second,
	})
	adapter := NewImageAdapter(client, "openai/gpt-image-1")

	response, err := adapter.Generate(context.Background(), &modality.ImageRequest{
		Model:          "openai/gpt-image-1",
		Prompt:         "A lighthouse at dusk",
		N:              1,
		ResponseFormat: "b64_json",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(response.Data) != 1 || response.Data[0].B64JSON != "AQID" {
		t.Fatalf("unexpected image response %#v", response)
	}
}

func TestImageAdapterGenerateNormalizesInlineImageToDataURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if _, ok := payload["response_format"]; ok {
			t.Fatalf("expected gpt-image request to omit response_format, got %#v", payload["response_format"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"created":1744329600,
			"data":[{"b64_json":"AQID","revised_prompt":"A lighthouse at dusk"}]
		}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "sk-test",
		BaseURL: server.URL + "/v1",
		Timeout: time.Second,
	})
	adapter := NewImageAdapter(client, "openai/gpt-image-1")

	response, err := adapter.Generate(context.Background(), &modality.ImageRequest{
		Model:          "openai/gpt-image-1",
		Prompt:         "A lighthouse at dusk",
		N:              1,
		ResponseFormat: "url",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(response.Data) != 1 || response.Data[0].URL != "data:image/png;base64,AQID" {
		t.Fatalf("unexpected image response %#v", response)
	}
}

func TestImageAdapterGenerateGPTImage2UsesGPTImageContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["model"] != "gpt-image-2" {
			t.Fatalf("expected stripped provider model, got %#v", payload["model"])
		}
		if _, ok := payload["response_format"]; ok {
			t.Fatalf("expected gpt-image request to omit response_format, got %#v", payload["response_format"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":1744329600,"data":[{"b64_json":"AQID"}]}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "sk-test",
		BaseURL: server.URL + "/v1",
		Timeout: time.Second,
	})
	adapter := NewImageAdapter(client, "openai/gpt-image-2")

	response, err := adapter.Generate(context.Background(), &modality.ImageRequest{
		Model:          "openai/gpt-image-2",
		Prompt:         "A clean product render",
		ResponseFormat: "url",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(response.Data) != 1 || response.Data[0].URL != "data:image/png;base64,AQID" {
		t.Fatalf("unexpected image response %#v", response)
	}
}

func TestImageAdapterEdit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/edits" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}

		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Fatalf("ParseMediaType() error = %v", err)
		}
		if mediaType != "multipart/form-data" {
			t.Fatalf("expected multipart/form-data, got %q", mediaType)
		}
		reader := multipart.NewReader(r.Body, params["boundary"])

		fields := map[string]string{}
		files := map[string][]byte{}
		for {
			part, err := reader.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatalf("NextPart() error = %v", err)
			}
			data, err := io.ReadAll(part)
			if err != nil {
				t.Fatalf("ReadAll() error = %v", err)
			}
			if part.FileName() != "" {
				files[part.FormName()] = data
			} else {
				fields[part.FormName()] = string(data)
			}
		}
		if fields["model"] != "gpt-image-1" {
			t.Fatalf("unexpected model field %q", fields["model"])
		}
		if _, ok := fields["response_format"]; ok {
			t.Fatalf("expected gpt-image edit request to omit response_format, got %#v", fields["response_format"])
		}
		if string(files["image"]) != "image-bytes" {
			t.Fatalf("unexpected image file %q", string(files["image"]))
		}
		if string(files["mask"]) != "mask-bytes" {
			t.Fatalf("unexpected mask file %q", string(files["mask"]))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data":[{"b64_json":"AQID","revised_prompt":"edited"}]
		}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "sk-test",
		BaseURL: server.URL + "/v1",
		Timeout: time.Second,
	})
	adapter := NewImageAdapter(client, "openai/gpt-image-1")

	response, err := adapter.Edit(context.Background(), &modality.ImageEditRequest{
		Model:          "openai/gpt-image-1",
		Prompt:         "edit this",
		Image:          []byte("image-bytes"),
		ImageFilename:  "input.png",
		ImageType:      "image/png",
		Mask:           []byte("mask-bytes"),
		MaskFilename:   "mask.png",
		MaskType:       "image/png",
		N:              1,
		ResponseFormat: "b64_json",
	})
	if err != nil {
		t.Fatalf("Edit() error = %v", err)
	}
	if len(response.Data) != 1 || response.Data[0].B64JSON != "AQID" {
		t.Fatalf("unexpected image response %#v", response)
	}
}

func TestImageAdapterGenerateLegacyModelForwardsResponseFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["model"] != "dall-e-3" {
			t.Fatalf("expected stripped provider model, got %#v", payload["model"])
		}
		if payload["response_format"] != "b64_json" {
			t.Fatalf("expected legacy response_format passthrough, got %#v", payload["response_format"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"created":1744329600,
			"data":[{"b64_json":"AQID"}]
		}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "sk-test",
		BaseURL: server.URL + "/v1",
		Timeout: time.Second,
	})
	adapter := NewImageAdapter(client, "openai/dall-e-3")

	response, err := adapter.Generate(context.Background(), &modality.ImageRequest{
		Model:          "openai/dall-e-3",
		Prompt:         "A lighthouse at dusk",
		ResponseFormat: "b64_json",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(response.Data) != 1 || response.Data[0].B64JSON != "AQID" {
		t.Fatalf("unexpected image response %#v", response)
	}
}

func TestImageAdapterMapsRateLimitError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"too many requests","type":"rate_limit_error","code":"rate_limit"}}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "sk-test",
		BaseURL: server.URL + "/v1",
		Timeout: time.Second,
	})
	adapter := NewImageAdapter(client, "openai/gpt-image-1")

	_, err := adapter.Generate(context.Background(), &modality.ImageRequest{
		Model:  "openai/gpt-image-1",
		Prompt: "hello",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	var apiErr *httputil.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.Status != http.StatusTooManyRequests || apiErr.Type != "rate_limit_error" {
		t.Fatalf("unexpected api error %#v", apiErr)
	}
}
