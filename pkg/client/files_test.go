package client

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUploadFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/files" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		mediaType := r.Header.Get("Content-Type")
		if !strings.Contains(mediaType, "multipart/form-data") {
			t.Fatalf("expected multipart content type, got %q", mediaType)
		}
		reader, err := r.MultipartReader()
		if err != nil {
			t.Fatalf("MultipartReader() error = %v", err)
		}
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
				t.Fatalf("read part: %v", err)
			}
			if part.FileName() != "" {
				files[part.FormName()] = data
				continue
			}
			fields[part.FormName()] = string(data)
		}
		if string(files["file"]) != "file-bytes" || fields["purpose"] != "user_data" {
			t.Fatalf("unexpected multipart payload fields=%#v files=%#v", fields, files)
		}
		if !strings.Contains(fields["metadata"], `"case":"sdk"`) {
			t.Fatalf("unexpected metadata field %q", fields["metadata"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"pl_file_test",
			"object":"file",
			"bytes":10,
			"created_at":1744329600,
			"filename":"note.txt",
			"purpose":"user_data",
			"polaris":{"sha256":"sha","mime_type":"text/plain","project_id":"proj","has_inline_bytes":true,"has_blob_backing":false,"content_url":"http://example.test/content"}
		}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, WithAPIKey("secret"))
	resp, err := client.UploadFile(context.Background(), &FileUploadRequest{
		File:        []byte("file-bytes"),
		Filename:    "note.txt",
		ContentType: "text/plain",
		Purpose:     "user_data",
		Metadata:    map[string]string{"case": "sdk"},
	})
	if err != nil {
		t.Fatalf("UploadFile() error = %v", err)
	}
	if resp.ID != "pl_file_test" || resp.Polaris.ContentURL == "" {
		t.Fatalf("unexpected file response %#v", resp)
	}
}

func TestFileCRUDMethods(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files":
			if r.URL.Query().Get("limit") != "1" || r.URL.Query().Get("purpose") != "user_data" {
				t.Fatalf("unexpected list query %q", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"object":"list","data":[],"has_more":false}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/pl_file_test":
			_, _ = w.Write([]byte(`{"id":"pl_file_test","object":"file","bytes":1,"created_at":1744329600,"filename":"a.txt","purpose":"user_data","polaris":{"sha256":"sha","mime_type":"text/plain","project_id":"proj","has_inline_bytes":true,"has_blob_backing":false}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/files/pl_file_test/materialize":
			if r.URL.Query().Get("provider") != "openai" {
				t.Fatalf("unexpected provider query %q", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"object":"file.materialization","file_id":"pl_file_test","provider":"openai","provider_file_id":"file_123","purpose":"user_data","bytes":1,"mime_type":"text/plain","created_at":1744329600,"cached":false}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/files/pl_file_test":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/files":
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r.Body)
			if !strings.Contains(buf.String(), `"url":"https://example.test/file.txt"`) {
				t.Fatalf("unexpected URL upload payload %q", buf.String())
			}
			_, _ = w.Write([]byte(`{"id":"pl_file_url","object":"file","bytes":1,"created_at":1744329600,"filename":"file.txt","purpose":"user_data","polaris":{"sha256":"sha","mime_type":"text/plain","project_id":"proj","has_inline_bytes":true,"has_blob_backing":false}}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	if _, err := client.ListFiles(context.Background(), &ListFilesParams{Limit: 1, Purpose: "user_data"}); err != nil {
		t.Fatalf("ListFiles() error = %v", err)
	}
	if _, err := client.GetFile(context.Background(), "pl_file_test"); err != nil {
		t.Fatalf("GetFile() error = %v", err)
	}
	materialized, err := client.MaterializeFile(context.Background(), "pl_file_test", "openai")
	if err != nil {
		t.Fatalf("MaterializeFile() error = %v", err)
	}
	if materialized.ProviderFileID != "file_123" {
		t.Fatalf("unexpected materialization %#v", materialized)
	}
	if _, err := client.UploadFileURL(context.Background(), &FileURLUploadRequest{URL: "https://example.test/file.txt", Purpose: "user_data"}); err != nil {
		t.Fatalf("UploadFileURL() error = %v", err)
	}
	if err := client.DeleteFile(context.Background(), "pl_file_test"); err != nil {
		t.Fatalf("DeleteFile() error = %v", err)
	}
}

func TestDownloadFileContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("unexpected Authorization header %q", got)
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("content"))
	}))
	defer server.Close()

	client := newTestClient(t, "https://example.test", WithAPIKey("secret"))
	data, contentType, err := client.DownloadFileContent(context.Background(), server.URL+"/content")
	if err != nil {
		t.Fatalf("DownloadFileContent() error = %v", err)
	}
	if string(data) != "content" || !strings.HasPrefix(contentType, "text/plain") {
		t.Fatalf("unexpected content data=%q contentType=%q", data, contentType)
	}
}
