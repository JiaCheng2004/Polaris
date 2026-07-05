package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/store"
)

func (s *Store) ensureIdempotencyUpgrade(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS idempotency_keys (
			key TEXT NOT NULL,
			project_id TEXT NOT NULL,
			endpoint TEXT NOT NULL,
			request_hash TEXT NOT NULL,
			response_status INTEGER NOT NULL,
			response_body BLOB,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at TIMESTAMP NOT NULL,
			PRIMARY KEY(key, project_id, endpoint)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_idempotency_expires_at ON idempotency_keys(expires_at);`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) CheckIdempotencyKey(ctx context.Context, key, projectID, endpoint string) (*store.IdempotencyKey, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT key, project_id, endpoint, request_hash, response_status, response_body, created_at, expires_at
		FROM idempotency_keys
		WHERE key = ? AND project_id = ? AND endpoint = ?
		LIMIT 1
	`, key, projectID, endpoint)
	var rec store.IdempotencyKey
	var body []byte
	if err := row.Scan(&rec.Key, &rec.ProjectID, &rec.Endpoint, &rec.RequestHash, &rec.ResponseStatus, &body, &rec.CreatedAt, &rec.ExpiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	rec.ResponseBody = body
	// An expired record is treated as absent.
	if !rec.ExpiresAt.IsZero() && time.Now().After(rec.ExpiresAt) {
		return nil, false, nil
	}
	return &rec, true, nil
}

func (s *Store) PutIdempotencyKey(ctx context.Context, rec store.IdempotencyKey) error {
	// DO NOTHING on conflict: the first writer of a key wins; a concurrent
	// duplicate on another replica simply keeps the existing record.
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO idempotency_keys (key, project_id, endpoint, request_hash, response_status, response_body, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(key, project_id, endpoint) DO NOTHING
	`, rec.Key, rec.ProjectID, rec.Endpoint, rec.RequestHash, rec.ResponseStatus, rec.ResponseBody, rec.CreatedAt, rec.ExpiresAt)
	return err
}

func (s *Store) PurgeExpiredIdempotencyKeys(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM idempotency_keys WHERE expires_at <= ?`, now)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
