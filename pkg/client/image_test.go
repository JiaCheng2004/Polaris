package client

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGenerateImage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}

		var payload ImageGenerationRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload.Model != "default-image" || payload.Prompt != "a lighthouse" {
			t.Fatalf("unexpected payload %#v", payload)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"created":1744329600,
			"data":[{"url":"https://example.com/generated.png","revised_prompt":"A lighthouse at dusk"}]
		}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	response, err := client.GenerateImage(context.Background(), &ImageGenerationRequest{
		Model:  "default-image",
		Prompt: "a lighthouse",
	})
	if err != nil {
		t.Fatalf("GenerateImage() error = %v", err)
	}
	if len(response.Data) != 1 || response.Data[0].URL != "https://example.com/generated.png" {
		t.Fatalf("unexpected response %#v", response)
	}
}

func TestEditImage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/edits" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}

		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Fatalf("ParseMediaType() error = %v", err)
		}
		if mediaType != "multipart/form-data" {
			t.Fatalf("unexpected media type %q", mediaType)
		}

		reader := multipart.NewReader(r.Body, params["boundary"])
		fields := map[string]string{}
		files := map[string][]byte{}
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
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
				continue
			}
			fields[part.FormName()] = string(data)
		}

		if fields["model"] != "default-image" || fields["prompt"] != "brighten it" {
			t.Fatalf("unexpected fields %#v", fields)
		}
		if fields["routing"] == "" || !strings.Contains(fields["routing"], "openai") {
			t.Fatalf("unexpected routing field %#v", fields["routing"])
		}
		if string(files["image"]) != "png-bytes" || string(files["mask"]) != "mask-bytes" {
			t.Fatalf("unexpected files %#v", files)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"created":1744329600,
			"data":[{"b64_json":"AQID"}]
		}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	response, err := client.EditImage(context.Background(), &ImageEditRequest{
		Model:          "default-image",
		Routing:        &RoutingOptions{Providers: []string{"openai"}},
		Prompt:         "brighten it",
		Image:          []byte("png-bytes"),
		ImageFilename:  "input.png",
		Mask:           []byte("mask-bytes"),
		MaskFilename:   "mask.png",
		ResponseFormat: "b64_json",
	})
	if err != nil {
		t.Fatalf("EditImage() error = %v", err)
	}
	if len(response.Data) != 1 || response.Data[0].B64JSON != "AQID" {
		t.Fatalf("unexpected response %#v", response)
	}
}
