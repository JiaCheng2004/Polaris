package store

import (
	"context"
	"time"
)

type Store interface {
	CreateAPIKey(ctx context.Context, key APIKey) error
	GetAPIKeyByHash(ctx context.Context, keyHash string) (*APIKey, error)
	ListAPIKeys(ctx context.Context, ownerID string, includeRevoked bool) ([]APIKey, error)
	DeleteAPIKey(ctx context.Context, id string) error

	LogRequest(ctx context.Context, log RequestLog) error
	LogRequestBatch(ctx context.Context, logs []RequestLog) error

	GetUsage(ctx context.Context, filter UsageFilter) (UsageReport, error)
	GetUsageByModel(ctx context.Context, filter UsageFilter) (UsageReport, error)

	PurgeOldLogs(ctx context.Context, olderThan time.Time) (int64, error)
	Migrate(ctx context.Context) error
	Ping(ctx context.Context) error
	Close() error
}
