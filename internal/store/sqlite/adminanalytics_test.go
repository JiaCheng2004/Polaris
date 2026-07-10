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

func newAdminTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(config.StoreConfig{
		Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "polaris.db"),
		MaxConnections: 1, LogRetentionDays: 30, LogBufferSize: 10, LogFlushInterval: time.Second,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	return s
}

func TestSummarizeUsageBreakdownsAndCacheTokens(t *testing.T) {
	ctx := context.Background()
	s := newAdminTestStore(t)
	now := time.Now().UTC()

	logs := []store.RequestLog{
		{RequestID: "a", KeyID: "k1", Model: "openai/gpt-4o", Modality: modality.ModalityChat, InputTokens: 100, OutputTokens: 50, CachedInputTokens: 20, CacheWrite5mTokens: 10, TotalTokens: 150, EstimatedCost: 0.10, StatusCode: 200, ProviderLatencyMs: 200, TotalLatencyMs: 250, CreatedAt: now},
		{RequestID: "b", KeyID: "k1", Model: "openai/gpt-4o", Modality: modality.ModalityChat, InputTokens: 10, TotalTokens: 10, EstimatedCost: 0.01, StatusCode: 500, ProviderLatencyMs: 100, TotalLatencyMs: 120, CreatedAt: now},
		{RequestID: "c", KeyID: "k1", Model: "anthropic/claude-opus-4-8", Modality: modality.ModalityChat, InputTokens: 200, OutputTokens: 100, CachedInputTokens: 50, CacheWrite1hTokens: 5, TotalTokens: 300, EstimatedCost: 0.30, StatusCode: 200, ProviderLatencyMs: 400, TotalLatencyMs: 450, CreatedAt: now},
	}
	if err := s.LogRequestBatch(ctx, logs); err != nil {
		t.Fatalf("LogRequestBatch: %v", err)
	}

	totals, err := s.SummarizeUsage(ctx, store.UsageFilter{}, "")
	if err != nil {
		t.Fatalf("SummarizeUsage totals: %v", err)
	}
	if len(totals) != 1 {
		t.Fatalf("expected 1 totals row, got %d", len(totals))
	}
	tot := totals[0]
	if tot.Requests != 3 || tot.InputTokens != 310 || tot.OutputTokens != 150 {
		t.Fatalf("totals tokens wrong: %#v", tot)
	}
	if tot.CachedInputTokens != 70 || tot.CacheWriteTokens != 15 {
		t.Fatalf("cache token economics wrong: cached=%d write=%d", tot.CachedInputTokens, tot.CacheWriteTokens)
	}
	if tot.Errors != 1 {
		t.Fatalf("expected 1 error (status 500), got %d", tot.Errors)
	}
	if tot.TotalTokens != 460 {
		t.Fatalf("expected 460 total tokens, got %d", tot.TotalTokens)
	}

	byModel, err := s.SummarizeUsage(ctx, store.UsageFilter{}, "model")
	if err != nil {
		t.Fatalf("SummarizeUsage model: %v", err)
	}
	if len(byModel) != 2 {
		t.Fatalf("expected 2 model rows, got %d", len(byModel))
	}
	// Ordered by requests DESC: openai/gpt-4o (2) first.
	if byModel[0].Key != "openai/gpt-4o" || byModel[0].Requests != 2 || byModel[0].Errors != 1 {
		t.Fatalf("top model row wrong: %#v", byModel[0])
	}

	// Provider filter narrows to a single provider's models.
	anth, err := s.SummarizeUsage(ctx, store.UsageFilter{Provider: "anthropic"}, "")
	if err != nil {
		t.Fatalf("SummarizeUsage provider filter: %v", err)
	}
	if anth[0].Requests != 1 || anth[0].InputTokens != 200 {
		t.Fatalf("provider filter wrong: %#v", anth[0])
	}

	// Unknown dimension is rejected (allow-list guard against injection).
	if _, err := s.SummarizeUsage(ctx, store.UsageFilter{}, "model; DROP TABLE request_logs"); err == nil {
		t.Fatal("expected error for unsupported dimension")
	}
}

func TestListAuditEventsOrderingFilterPagination(t *testing.T) {
	ctx := context.Background()
	s := newAdminTestStore(t)
	base := time.Now().UTC().Add(-time.Hour)

	events := []store.AuditEvent{
		{ID: "e1", ProjectID: "p1", ActorKeyID: "k1", Kind: "project.created", ResourceType: "project", ResourceID: "p1", CreatedAt: base},
		{ID: "e2", ProjectID: "p1", ActorKeyID: "k1", Kind: "virtual_key.created", ResourceType: "virtual_key", ResourceID: "vk1", CreatedAt: base.Add(time.Minute)},
		{ID: "e3", ProjectID: "p2", ActorKeyID: "k2", Kind: "budget.created", ResourceType: "budget", ResourceID: "b1", CreatedAt: base.Add(2 * time.Minute)},
		// project_id + actor_key_id are stored NULL when empty (nullableString);
		// ListAuditEvents must read them back without a NULL-scan failure.
		{ID: "e4", Kind: "tool.created", ResourceType: "tool", ResourceID: "t1", CreatedAt: base.Add(3 * time.Minute)},
	}
	if err := s.LogAuditEventBatch(ctx, events); err != nil {
		t.Fatalf("LogAuditEventBatch: %v", err)
	}

	all, err := s.ListAuditEvents(ctx, store.AuditFilter{})
	if err != nil {
		t.Fatalf("ListAuditEvents: %v", err)
	}
	if len(all) != 4 || all[0].ID != "e4" {
		t.Fatalf("expected newest-first (e4 first), got %d rows head=%q", len(all), all[0].ID)
	}
	if all[0].ProjectID != "" || all[0].ActorKeyID != "" {
		t.Fatalf("expected empty (NULL-sourced) project/actor, got %q/%q", all[0].ProjectID, all[0].ActorKeyID)
	}

	scoped, err := s.ListAuditEvents(ctx, store.AuditFilter{ProjectID: "p1"})
	if err != nil {
		t.Fatalf("ListAuditEvents filter: %v", err)
	}
	if len(scoped) != 2 {
		t.Fatalf("expected 2 events for p1, got %d", len(scoped))
	}

	page, err := s.ListAuditEvents(ctx, store.AuditFilter{Limit: 1})
	if err != nil {
		t.Fatalf("ListAuditEvents limit: %v", err)
	}
	if len(page) != 1 || page[0].ID != "e4" {
		t.Fatalf("expected 1 event (e4), got %d head=%q", len(page), page[0].ID)
	}
	next, err := s.ListAuditEvents(ctx, store.AuditFilter{Limit: 1, Before: &page[0].CreatedAt})
	if err != nil {
		t.Fatalf("ListAuditEvents cursor: %v", err)
	}
	if len(next) != 1 || next[0].ID != "e3" {
		t.Fatalf("expected e3 on next page, got %#v", next)
	}
}
