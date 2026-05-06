package openai

import (
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func TestFilesAdapterMaterializeUploadsMultipartFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/files" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Fatalf("unexpected Authorization header %q", got)
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
				if part.FileName() != "report.pdf" {
					t.Fatalf("unexpected file name %q", part.FileName())
				}
			} else {
				fields[part.FormName()] = string(data)
			}
		}
		if fields["purpose"] != "user_data" {
			t.Fatalf("unexpected purpose %q", fields["purpose"])
		}
		if string(files["file"]) != "%PDF bytes" {
			t.Fatalf("unexpected file bytes %q", string(files["file"]))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"file_openai_123",
			"object":"file",
			"bytes":10,
			"filename":"report.pdf",
			"purpose":"user_data",
			"created_at":1714857600
		}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "sk-test",
		BaseURL: server.URL + "/v1",
		Timeout: time.Second,
	})
	adapter := NewFilesAdapter(client, "openai")
	handle, err := adapter.Materialize(context.Background(), &modality.FileMaterializeRequest{
		PolarisID: "pl_file_01J7P9V3YMZQK7E0X2QF5N6BHD",
		MimeType:  "application/pdf",
		Filename:  "report.pdf",
		Purpose:   modality.FilePurposeUserData,
		InlineSrc: []byte("%PDF bytes"),
		Size:      int64(len("%PDF bytes")),
	})
	if err != nil {
		t.Fatalf("Materialize() error = %v", err)
	}
	if handle.Provider != "openai" || handle.ProviderFileID != "file_openai_123" || handle.SizeBytes != 10 {
		t.Fatalf("unexpected handle %#v", handle)
	}
	if handle.CreatedAt.Unix() != 1714857600 {
		t.Fatalf("unexpected created_at %s", handle.CreatedAt)
	}
}
