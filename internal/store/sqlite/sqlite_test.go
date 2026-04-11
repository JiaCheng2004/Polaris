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
	defer sqliteStore.Close()

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
	}
	if err := sqliteStore.LogRequestBatch(ctx, logs); err != nil {
		t.Fatalf("LogRequestBatch() error = %v", err)
	}

	report, err := sqliteStore.GetUsage(ctx, store.UsageFilter{KeyID: gotKey.ID})
	if err != nil {
		t.Fatalf("GetUsage() error = %v", err)
	}
	if report.TotalRequests != 2 {
		t.Fatalf("expected 2 requests, got %d", report.TotalRequests)
	}
	if report.TotalTokens != 52 {
		t.Fatalf("expected 52 tokens, got %d", report.TotalTokens)
	}

	modelReport, err := sqliteStore.GetUsageByModel(ctx, store.UsageFilter{KeyID: gotKey.ID})
	if err != nil {
		t.Fatalf("GetUsageByModel() error = %v", err)
	}
	if len(modelReport.ByModel) != 2 {
		t.Fatalf("expected 2 model groups, got %d", len(modelReport.ByModel))
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
