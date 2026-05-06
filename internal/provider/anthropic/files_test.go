package anthropic

import (
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func TestFilesAdapterMaterializePreservesMultipartContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/files" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("anthropic-beta"); !strings.Contains(got, anthropicFilesBeta) {
			t.Fatalf("expected files beta header, got %q", got)
		}
		reader, err := r.MultipartReader()
		if err != nil {
			t.Fatalf("multipart reader: %v", err)
		}
		part, err := reader.NextPart()
		if err != nil {
			t.Fatalf("next part: %v", err)
		}
		defer func() {
			_ = part.Close()
		}()
		if part.FormName() != "file" {
			t.Fatalf("unexpected form part %q", part.FormName())
		}
		if got := part.Header.Get("Content-Type"); got != "application/pdf" {
			t.Fatalf("expected file Content-Type application/pdf, got %q", got)
		}
		data, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("read part: %v", err)
		}
		if string(data) != "%PDF-1.4" {
			t.Fatalf("unexpected body %q", string(data))
		}
		if _, err := reader.NextPart(); err != io.EOF && err != multipart.ErrMessageTooLarge {
			t.Fatalf("unexpected extra part err %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"file_abc","type":"file","filename":"eval.pdf","size_bytes":8,"mime_type":"application/pdf","created_at":"2026-05-06T00:00:00Z"}`))
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		APIKey:  "sk-anthropic",
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	adapter := NewFilesAdapter(client, "anthropic")

	handle, err := adapter.Materialize(context.Background(), &modality.FileMaterializeRequest{
		MimeType:  "application/pdf",
		Filename:  "eval.pdf",
		Purpose:   modality.FilePurposeUserData,
		InlineSrc: []byte("%PDF-1.4"),
		Size:      8,
	})
	if err != nil {
		t.Fatalf("Materialize() error = %v", err)
	}
	if handle.ProviderFileID != "file_abc" || handle.MimeType != "application/pdf" {
		t.Fatalf("unexpected handle %#v", handle)
	}
}
