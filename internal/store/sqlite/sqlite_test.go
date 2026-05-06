package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/store"
)

func TestSQLiteStoreCRUDAndUsage(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "polaris.db")
	sqliteStore, err := New(config.StoreConfig{
		Driver:           "sqlite",
		DSN:              dbPath,
		MaxConnections:   1,
		LogRetentionDays: 30,
		LogBufferSize:    10,
		LogFlushInterval: time.Second,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		_ = sqliteStore.Close()
	}()

	if err := sqliteStore.Migrate(ctx); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	key := store.APIKey{
		Name:          "test-key",
		KeyHash:       "sha256:test",
		KeyPrefix:     "polaris-",
		RateLimit:     "10/min",
		AllowedModels: []string{"*"},
	}
	if err := sqliteStore.CreateAPIKey(ctx, key); err != nil {
		t.Fatalf("CreateAPIKey() error = %v", err)
	}

	gotKey, err := sqliteStore.GetAPIKeyByHash(ctx, "sha256:test")
	if err != nil {
		t.Fatalf("GetAPIKeyByHash() error = %v", err)
	}
	if gotKey.Name != "test-key" {
		t.Fatalf("expected key name test-key, got %q", gotKey.Name)
	}

	usedAt := time.Now().UTC().Truncate(time.Second)
	if err := sqliteStore.UpdateAPIKeyLastUsed(ctx, gotKey.ID, usedAt); err != nil {
		t.Fatalf("UpdateAPIKeyLastUsed() error = %v", err)
	}
	gotKey, err = sqliteStore.GetAPIKeyByHash(ctx, "sha256:test")
	if err != nil {
		t.Fatalf("GetAPIKeyByHash() after last_used_at update error = %v", err)
	}
	if gotKey.LastUsedAt == nil || !gotKey.LastUsedAt.UTC().Equal(usedAt) {
		t.Fatalf("expected last_used_at %s, got %#v", usedAt, gotKey.LastUsedAt)
	}

	now := time.Now().UTC()
	logs := []store.RequestLog{
		{
			RequestID:     "req-1",
			KeyID:         gotKey.ID,
			Model:         "openai/gpt-4o",
			Modality:      modality.ModalityChat,
			TotalTokens:   42,
			StatusCode:    200,
			EstimatedCost: 0.12,
			CreatedAt:     now,
		},
		{
			RequestID:     "req-2",
			KeyID:         gotKey.ID,
			Model:         "openai/gpt-4o-mini",
			Modality:      modality.ModalityChat,
			TotalTokens:   10,
			StatusCode:    200,
			EstimatedCost: 0.03,
			CreatedAt:     now,
		},
		{
			RequestID:     "req-3",
			KeyID:         gotKey.ID,
			Model:         "openai/gpt-4o-audio",
			Modality:      modality.ModalityAudio,
			TotalTokens:   18,
			StatusCode:    101,
			EstimatedCost: 0.000145,
			CreatedAt:     now,
		},
	}
	if err := sqliteStore.LogRequestBatch(ctx, logs); err != nil {
		t.Fatalf("LogRequestBatch() error = %v", err)
	}

	report, err := sqliteStore.GetUsage(ctx, store.UsageFilter{KeyID: gotKey.ID})
	if err != nil {
		t.Fatalf("GetUsage() error = %v", err)
	}
	if report.TotalRequests != 3 {
		t.Fatalf("expected 3 requests, got %d", report.TotalRequests)
	}
	if report.TotalTokens != 70 {
		t.Fatalf("expected 70 tokens, got %d", report.TotalTokens)
	}

	modelReport, err := sqliteStore.GetUsageByModel(ctx, store.UsageFilter{KeyID: gotKey.ID})
	if err != nil {
		t.Fatalf("GetUsageByModel() error = %v", err)
	}
	if len(modelReport.ByModel) != 3 {
		t.Fatalf("expected 3 model groups, got %d", len(modelReport.ByModel))
	}

	audioReport, err := sqliteStore.GetUsageByModel(ctx, store.UsageFilter{KeyID: gotKey.ID, Modality: modality.ModalityAudio})
	if err != nil {
		t.Fatalf("GetUsageByModel(audio) error = %v", err)
	}
	if audioReport.TotalRequests != 1 || audioReport.TotalTokens != 18 {
		t.Fatalf("unexpected audio usage report %#v", audioReport)
	}
	if len(audioReport.ByModel) != 1 || audioReport.ByModel[0].Model != "openai/gpt-4o-audio" {
		t.Fatalf("unexpected audio usage by model %#v", audioReport.ByModel)
	}

	if err := sqliteStore.DeleteAPIKey(ctx, gotKey.ID); err != nil {
		t.Fatalf("DeleteAPIKey() error = %v", err)
	}
	revoked, err := sqliteStore.GetAPIKeyByHash(ctx, "sha256:test")
	if err != nil {
		t.Fatalf("GetAPIKeyByHash() after revoke error = %v", err)
	}
	if !revoked.IsRevoked {
		t.Fatalf("expected key to be revoked")
	}
}

func TestSQLiteMigrateUpgradesExistingRequestLogsSchema(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	sqliteStore, err := New(config.StoreConfig{
		Driver:           "sqlite",
		DSN:              dbPath,
		MaxConnections:   1,
		LogRetentionDays: 30,
		LogBufferSize:    10,
		LogFlushInterval: time.Second,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		_ = sqliteStore.Close()
	}()

	_, err = sqliteStore.db.ExecContext(ctx, `
		CREATE TABLE request_logs (
			id TEXT PRIMARY KEY,
			request_id TEXT NOT NULL,
			key_id TEXT NOT NULL,
			project_id TEXT,
			model TEXT NOT NULL,
			modality TEXT NOT NULL,
			provider_latency_ms INTEGER,
			total_latency_ms INTEGER,
			input_tokens INTEGER,
			output_tokens INTEGER,
			total_tokens INTEGER,
			estimated_cost REAL,
			status_code INTEGER NOT NULL,
			error_type TEXT,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		t.Fatalf("create legacy request_logs: %v", err)
	}

	if err := sqliteStore.Migrate(ctx); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	for _, column := range []string{"interface_family", "token_source", "cache_status", "fallback_model", "trace_id", "toolset", "mcp_binding", "cost_source"} {
		exists, err := sqliteStore.sqliteColumnExists(ctx, "request_logs", column)
		if err != nil {
			t.Fatalf("sqliteColumnExists(%s) error = %v", column, err)
		}
		if !exists {
			t.Fatalf("expected migrated request_logs column %s", column)
		}
	}
}

func TestSQLiteStoreFiles(t *testing.T) {
	ctx := context.Background()
	sqliteStore, err := New(config.StoreConfig{
		Driver:           "sqlite",
		DSN:              filepath.Join(t.TempDir(), "files.db"),
		MaxConnections:   1,
		LogRetentionDays: 30,
		LogBufferSize:    10,
		LogFlushInterval: time.Second,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() {
		_ = sqliteStore.Close()
	}()
	if err := sqliteStore.Migrate(ctx); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	for _, projectID := range []string{"proj_a", "proj_b"} {
		if err := sqliteStore.CreateProject(ctx, store.Project{ID: projectID, Name: projectID, CreatedAt: time.Now().UTC()}); err != nil {
			t.Fatalf("CreateProject(%s) error = %v", projectID, err)
		}
	}

	expiresAt := time.Now().UTC().Add(time.Hour)
	file := store.File{
		PolarisID:        "pl_file_01J7P9V3YMZQK7E0X2QF5N6BHD",
		ProjectID:        "proj_a",
		KeyID:            "vk_a",
		Sha256:           "abc123",
		Size:             5,
		MimeType:         "text/plain",
		OriginalFilename: "note.txt",
		Purpose:          modality.FilePurposeUserData,
		InlineBytes:      []byte("hello"),
		BlobKey:          "sha256/ab/abc123",
		Metadata:         map[string]string{"source": "test"},
		CreatedAt:        time.Now().UTC(),
		ExpiresAt:        &expiresAt,
	}
	if err := sqliteStore.CreateFile(ctx, file); err != nil {
		t.Fatalf("CreateFile() error = %v", err)
	}
	got, err := sqliteStore.GetFileForProject(ctx, file.PolarisID, "proj_a")
	if err != nil {
		t.Fatalf("GetFileForProject() error = %v", err)
	}
	if got.Sha256 != file.Sha256 || string(got.InlineBytes) != "hello" || got.Metadata["source"] != "test" {
		t.Fatalf("unexpected file %#v", got)
	}
	if _, err := sqliteStore.GetFileForProject(ctx, file.PolarisID, "proj_b"); err != store.ErrNotFound {
		t.Fatalf("expected cross-project ErrNotFound, got %v", err)
	}
	inline, err := sqliteStore.GetFileInline(ctx, file.PolarisID)
	if err != nil || string(inline) != "hello" {
		t.Fatalf("GetFileInline() = %q, %v", inline, err)
	}
	total, err := sqliteStore.SumProjectFileBytes(ctx, "proj_a")
	if err != nil || total != 5 {
		t.Fatalf("SumProjectFileBytes() = %d, %v", total, err)
	}
	count, err := sqliteStore.CountProjectFiles(ctx, "proj_a")
	if err != nil || count != 1 {
		t.Fatalf("CountProjectFiles() = %d, %v", count, err)
	}
	blobRefs, err := sqliteStore.CountFilesByBlobKey(ctx, "sha256/ab/abc123")
	if err != nil || blobRefs != 1 {
		t.Fatalf("CountFilesByBlobKey() = %d, %v", blobRefs, err)
	}

	artifact := store.FileUnderstandingArtifact{
		Sha256:       file.Sha256,
		Processor:    "polaris_text_extract",
		Version:      "v1",
		MimeType:     "text/plain",
		Text:         "hello",
		Warning:      "quality warning",
		MetadataJSON: `{"truncated":false}`,
		ArtifactJSON: `{"kind":"text","text":"hello"}`,
		CreatedAt:    time.Now().UTC(),
	}
	if err := sqliteStore.PutFileUnderstandingArtifact(ctx, artifact); err != nil {
		t.Fatalf("PutFileUnderstandingArtifact() error = %v", err)
	}
	gotArtifact, ok, err := sqliteStore.GetFileUnderstandingArtifact(ctx, file.Sha256, "polaris_text_extract", "v1")
	if err != nil || !ok || gotArtifact.Text != "hello" || gotArtifact.Warning != "quality warning" || gotArtifact.ArtifactJSON == "" {
		t.Fatalf("GetFileUnderstandingArtifact() = %#v, %v, %v", gotArtifact, ok, err)
	}

	handle := store.FileProviderHandle{
		PolarisID:      file.PolarisID,
		Provider:       "openai",
		ProviderFileID: "file_provider",
		Purpose:        modality.FilePurposeUserData,
		SizeBytes:      5,
		MimeType:       "text/plain",
		CreatedAt:      time.Now().UTC(),
	}
	if err := sqliteStore.PutFileProviderHandle(ctx, handle); err != nil {
		t.Fatalf("PutFileProviderHandle() error = %v", err)
	}
	gotHandle, ok, err := sqliteStore.GetFileProviderHandle(ctx, file.PolarisID, "openai")
	if err != nil || !ok || gotHandle.ProviderFileID != "file_provider" {
		t.Fatalf("GetFileProviderHandle() = %#v, %v, %v", gotHandle, ok, err)
	}
	files, err := sqliteStore.ListFiles(ctx, store.FileFilter{ProjectID: "proj_a", Limit: 10})
	if err != nil || len(files) != 1 {
		t.Fatalf("ListFiles() len=%d err=%v", len(files), err)
	}
	if err := sqliteStore.DeleteFile(ctx, file.PolarisID); err != nil {
		t.Fatalf("DeleteFile() error = %v", err)
	}
	if _, ok, err := sqliteStore.GetFileProviderHandle(ctx, file.PolarisID, "openai"); err != nil || ok {
		t.Fatalf("expected provider handle cascade delete, ok=%v err=%v", ok, err)
	}
}
