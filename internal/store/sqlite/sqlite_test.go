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
