package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/store"
	_ "modernc.org/sqlite"
)

//go:embed migrations/001_init.up.sql
var migrationUp string

type Store struct {
	db *sql.DB
}

func New(cfg config.StoreConfig) (*Store, error) {
	db, err := sql.Open("sqlite", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("open sqlite store: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxConnections)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	if _, err := db.Exec(`PRAGMA busy_timeout = 5000; PRAGMA foreign_keys = ON; PRAGMA journal_mode = WAL;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("configure sqlite pragmas: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) CreateAPIKey(ctx context.Context, key store.APIKey) error {
	if key.ID == "" {
		key.ID = newID()
	}
	if key.CreatedAt.IsZero() {
		key.CreatedAt = time.Now().UTC()
	}
	if len(key.AllowedModels) == 0 {
		key.AllowedModels = []string{"*"}
	}
	allowedModels, err := json.Marshal(key.AllowedModels)
	if err != nil {
		return fmt.Errorf("encode allowed_models: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO api_keys (
			id, name, key_hash, key_prefix, owner_id, rate_limit, allowed_models,
			is_admin, created_at, last_used_at, expires_at, is_revoked
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, key.ID, key.Name, key.KeyHash, key.KeyPrefix, nullableString(key.OwnerID), nullableString(key.RateLimit), string(allowedModels), key.IsAdmin, key.CreatedAt, key.LastUsedAt, key.ExpiresAt, key.IsRevoked)
	if err != nil {
		return fmt.Errorf("insert api key: %w", err)
	}
	return nil
}

func (s *Store) GetAPIKeyByHash(ctx context.Context, keyHash string) (*store.APIKey, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, key_hash, key_prefix, owner_id, rate_limit, allowed_models,
		       is_admin, created_at, last_used_at, expires_at, is_revoked
		FROM api_keys
		WHERE key_hash = ?
		LIMIT 1
	`, keyHash)

	key, err := scanAPIKey(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	return key, nil
}

func (s *Store) ListAPIKeys(ctx context.Context, ownerID string, includeRevoked bool) ([]store.APIKey, error) {
	query := `
		SELECT id, name, key_hash, key_prefix, owner_id, rate_limit, allowed_models,
		       is_admin, created_at, last_used_at, expires_at, is_revoked
		FROM api_keys
	`
	var clauses []string
	var args []any
	if ownerID != "" {
		clauses = append(clauses, "owner_id = ?")
		args = append(args, ownerID)
	}
	if !includeRevoked {
		clauses = append(clauses, "is_revoked = FALSE")
	}
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	var keys []store.APIKey
	for rows.Next() {
		key, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		keys = append(keys, *key)
	}
	return keys, rows.Err()
}

func (s *Store) DeleteAPIKey(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE api_keys SET is_revoked = TRUE WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("revoke api key: %w", err)
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) LogRequest(ctx context.Context, entry store.RequestLog) error {
	return s.LogRequestBatch(ctx, []store.RequestLog{entry})
}

func (s *Store) LogRequestBatch(ctx context.Context, logs []store.RequestLog) error {
	if len(logs) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin request log batch: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO request_logs (
			id, request_id, key_id, model, modality, provider_latency_ms, total_latency_ms,
			input_tokens, output_tokens, total_tokens, estimated_cost, status_code, error_type, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare request log batch: %w", err)
	}
	defer stmt.Close()

	for _, entry := range logs {
		if entry.ID == "" {
			entry.ID = newID()
		}
		if entry.CreatedAt.IsZero() {
			entry.CreatedAt = time.Now().UTC()
		}
		if _, err = stmt.ExecContext(ctx,
			entry.ID,
			entry.RequestID,
			entry.KeyID,
			entry.Model,
			string(entry.Modality),
			entry.ProviderLatencyMs,
			entry.TotalLatencyMs,
			entry.InputTokens,
			entry.OutputTokens,
			entry.TotalTokens,
			entry.EstimatedCost,
			entry.StatusCode,
			nullableString(entry.ErrorType),
			entry.CreatedAt,
		); err != nil {
			return fmt.Errorf("insert request log: %w", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit request log batch: %w", err)
	}
	return nil
}

func (s *Store) GetUsage(ctx context.Context, filter store.UsageFilter) (store.UsageReport, error) {
	where, args := usageWhereClause(filter)
	report, err := usageTotals(ctx, s.db, where, args)
	if err != nil {
		return store.UsageReport{}, err
	}

	query := `
		SELECT substr(CAST(request_logs.created_at AS TEXT), 1, 10) AS usage_date,
		       COUNT(*) AS requests,
		       COALESCE(SUM(total_tokens), 0) AS tokens,
		       COALESCE(SUM(estimated_cost), 0) AS cost
		FROM request_logs
		LEFT JOIN api_keys ON api_keys.id = request_logs.key_id
	`
	if where != "" {
		query += " WHERE " + where
	}
	query += " GROUP BY usage_date ORDER BY usage_date ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return store.UsageReport{}, fmt.Errorf("query usage by day: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var day store.DailyUsage
		if err := rows.Scan(&day.Date, &day.Requests, &day.Tokens, &day.CostUSD); err != nil {
			return store.UsageReport{}, fmt.Errorf("scan usage by day: %w", err)
		}
		report.ByDay = append(report.ByDay, day)
	}
	return report, rows.Err()
}

func (s *Store) GetUsageByModel(ctx context.Context, filter store.UsageFilter) (store.UsageReport, error) {
	where, args := usageWhereClause(filter)
	report, err := usageTotals(ctx, s.db, where, args)
	if err != nil {
		return store.UsageReport{}, err
	}

	query := `
		SELECT request_logs.model,
		       COUNT(*) AS requests,
		       COALESCE(SUM(total_tokens), 0) AS tokens,
		       COALESCE(SUM(estimated_cost), 0) AS cost
		FROM request_logs
		LEFT JOIN api_keys ON api_keys.id = request_logs.key_id
	`
	if where != "" {
		query += " WHERE " + where
	}
	query += " GROUP BY request_logs.model ORDER BY request_logs.model ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return store.UsageReport{}, fmt.Errorf("query usage by model: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var modelUsage store.ModelUsage
		if err := rows.Scan(&modelUsage.Model, &modelUsage.Requests, &modelUsage.Tokens, &modelUsage.CostUSD); err != nil {
			return store.UsageReport{}, fmt.Errorf("scan usage by model: %w", err)
		}
		report.ByModel = append(report.ByModel, modelUsage)
	}
	return report, rows.Err()
}

func (s *Store) PurgeOldLogs(ctx context.Context, olderThan time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM request_logs WHERE created_at < ?`, olderThan.UTC())
	if err != nil {
		return 0, fmt.Errorf("purge request logs: %w", err)
	}
	return result.RowsAffected()
}

func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, migrationUp); err != nil {
		return fmt.Errorf("apply sqlite migrations: %w", err)
	}
	return nil
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *Store) Close() error {
	return s.db.Close()
}

func scanAPIKey(scanner interface{ Scan(dest ...any) error }) (*store.APIKey, error) {
	var key store.APIKey
	var allowedModels string
	var ownerID, rateLimit sql.NullString
	var lastUsedAt, expiresAt sql.NullTime

	if err := scanner.Scan(
		&key.ID,
		&key.Name,
		&key.KeyHash,
		&key.KeyPrefix,
		&ownerID,
		&rateLimit,
		&allowedModels,
		&key.IsAdmin,
		&key.CreatedAt,
		&lastUsedAt,
		&expiresAt,
		&key.IsRevoked,
	); err != nil {
		return nil, err
	}

	if ownerID.Valid {
		key.OwnerID = ownerID.String
	}
	if rateLimit.Valid {
		key.RateLimit = rateLimit.String
	}
	if lastUsedAt.Valid {
		key.LastUsedAt = &lastUsedAt.Time
	}
	if expiresAt.Valid {
		key.ExpiresAt = &expiresAt.Time
	}
	if err := json.Unmarshal([]byte(allowedModels), &key.AllowedModels); err != nil {
		return nil, fmt.Errorf("decode allowed_models: %w", err)
	}
	return &key, nil
}

func usageWhereClause(filter store.UsageFilter) (string, []any) {
	var clauses []string
	var args []any

	if filter.KeyID != "" {
		clauses = append(clauses, "request_logs.key_id = ?")
		args = append(args, filter.KeyID)
	}
	if filter.OwnerID != "" {
		clauses = append(clauses, "api_keys.owner_id = ?")
		args = append(args, filter.OwnerID)
	}
	if filter.Model != "" {
		clauses = append(clauses, "request_logs.model = ?")
		args = append(args, filter.Model)
	}
	if filter.Modality != "" {
		clauses = append(clauses, "request_logs.modality = ?")
		args = append(args, string(filter.Modality))
	}
	if filter.From != nil {
		clauses = append(clauses, "request_logs.created_at >= ?")
		args = append(args, filter.From.UTC())
	}
	if filter.To != nil {
		clauses = append(clauses, "request_logs.created_at < ?")
		args = append(args, filter.To.UTC())
	}

	return strings.Join(clauses, " AND "), args
}

func usageTotals(ctx context.Context, db *sql.DB, where string, args []any) (store.UsageReport, error) {
	query := `
		SELECT COUNT(*),
		       COALESCE(SUM(total_tokens), 0),
		       COALESCE(SUM(estimated_cost), 0)
		FROM request_logs
		LEFT JOIN api_keys ON api_keys.id = request_logs.key_id
	`
	if where != "" {
		query += " WHERE " + where
	}

	var report store.UsageReport
	if err := db.QueryRowContext(ctx, query, args...).Scan(&report.TotalRequests, &report.TotalTokens, &report.TotalCost); err != nil {
		return store.UsageReport{}, fmt.Errorf("query usage totals: %w", err)
	}
	return report, nil
}

func newID() string {
	var data [16]byte
	_, _ = rand.Read(data[:])
	return hex.EncodeToString(data[:])
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
