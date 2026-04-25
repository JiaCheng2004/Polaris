package postgres

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
	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/001_init.up.sql
var migrationUp string

type Store struct {
	db *sql.DB
}

func New(cfg config.StoreConfig) (*Store, error) {
	db, err := sql.Open("pgx", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("open postgres store: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxConnections)
	db.SetMaxIdleConns(max(1, min(cfg.MaxConnections, 4)))
	db.SetConnMaxIdleTime(5 * time.Minute)
	db.SetConnMaxLifetime(time.Hour)

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
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`,
		key.ID,
		key.Name,
		key.KeyHash,
		key.KeyPrefix,
		nullableString(key.OwnerID),
		nullableString(key.RateLimit),
		string(allowedModels),
		key.IsAdmin,
		key.CreatedAt.UTC(),
		nullableTime(key.LastUsedAt),
		nullableTime(key.ExpiresAt),
		key.IsRevoked,
	)
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
		WHERE key_hash = $1
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
		args = append(args, ownerID)
		clauses = append(clauses, fmt.Sprintf("owner_id = $%d", len(args)))
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
	defer func() {
		_ = rows.Close()
	}()

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
	result, err := s.db.ExecContext(ctx, `UPDATE api_keys SET is_revoked = TRUE WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("revoke api key: %w", err)
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) UpdateAPIKeyLastUsed(ctx context.Context, id string, usedAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE api_keys SET last_used_at = $1 WHERE id = $2`, usedAt.UTC(), id)
	if err != nil {
		return fmt.Errorf("update api key last_used_at: %w", err)
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
			id, request_id, key_id, project_id, model, modality, interface_family, token_source,
			cache_status, fallback_model, trace_id, toolset, mcp_binding, provider_latency_ms, total_latency_ms,
			input_tokens, output_tokens, total_tokens, estimated_cost, status_code, error_type, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)
	`)
	if err != nil {
		return fmt.Errorf("prepare request log batch: %w", err)
	}
	defer func() {
		_ = stmt.Close()
	}()

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
			nullableString(entry.ProjectID),
			entry.Model,
			string(entry.Modality),
			nullableString(entry.InterfaceFamily),
			nullableString(entry.TokenSource),
			nullableString(entry.CacheStatus),
			nullableString(entry.FallbackModel),
			nullableString(entry.TraceID),
			nullableString(entry.Toolset),
			nullableString(entry.MCPBinding),
			entry.ProviderLatencyMs,
			entry.TotalLatencyMs,
			entry.InputTokens,
			entry.OutputTokens,
			entry.TotalTokens,
			entry.EstimatedCost,
			entry.StatusCode,
			nullableString(entry.ErrorType),
			entry.CreatedAt.UTC(),
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
		SELECT TO_CHAR(DATE(request_logs.created_at), 'YYYY-MM-DD') AS usage_date,
		       COUNT(*) AS requests,
		       COALESCE(SUM(total_tokens), 0) AS tokens,
		       COALESCE(SUM(estimated_cost), 0) AS cost
		FROM request_logs
		LEFT JOIN api_keys ON api_keys.id = request_logs.key_id
		LEFT JOIN virtual_keys ON virtual_keys.id = request_logs.key_id
	`
	if where != "" {
		query += " WHERE " + where
	}
	query += " GROUP BY DATE(request_logs.created_at) ORDER BY DATE(request_logs.created_at) ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return store.UsageReport{}, fmt.Errorf("query usage by day: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

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
		LEFT JOIN virtual_keys ON virtual_keys.id = request_logs.key_id
	`
	if where != "" {
		query += " WHERE " + where
	}
	query += " GROUP BY request_logs.model ORDER BY request_logs.model ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return store.UsageReport{}, fmt.Errorf("query usage by model: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

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
	result, err := s.db.ExecContext(ctx, `DELETE FROM request_logs WHERE created_at < $1`, olderThan.UTC())
	if err != nil {
		return 0, fmt.Errorf("purge request logs: %w", err)
	}
	return result.RowsAffected()
}

func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, migrationUp); err != nil {
		if upgradeErr := s.ensureControlPlaneUpgrade(ctx); upgradeErr != nil {
			return fmt.Errorf("apply postgres migrations: %w (upgrade fallback failed: %v)", err, upgradeErr)
		}
		return nil
	}
	return nil
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) ensureControlPlaneUpgrade(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS projects (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			created_at TIMESTAMPTZ NOT NULL,
			archived_at TIMESTAMPTZ
		);`,
		`ALTER TABLE request_logs ADD COLUMN IF NOT EXISTS project_id TEXT;`,
		`ALTER TABLE request_logs ADD COLUMN IF NOT EXISTS interface_family TEXT;`,
		`ALTER TABLE request_logs ADD COLUMN IF NOT EXISTS token_source TEXT;`,
		`ALTER TABLE request_logs ADD COLUMN IF NOT EXISTS cache_status TEXT;`,
		`ALTER TABLE request_logs ADD COLUMN IF NOT EXISTS fallback_model TEXT;`,
		`ALTER TABLE request_logs ADD COLUMN IF NOT EXISTS trace_id TEXT;`,
		`ALTER TABLE request_logs ADD COLUMN IF NOT EXISTS toolset TEXT;`,
		`ALTER TABLE request_logs ADD COLUMN IF NOT EXISTS mcp_binding TEXT;`,
		`CREATE TABLE IF NOT EXISTS virtual_keys (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL REFERENCES projects(id),
			name TEXT NOT NULL,
			key_hash TEXT NOT NULL UNIQUE,
			key_prefix TEXT NOT NULL,
			rate_limit TEXT,
			allowed_models JSONB NOT NULL,
			allowed_modalities JSONB NOT NULL DEFAULT '[]'::jsonb,
			allowed_toolsets JSONB NOT NULL DEFAULT '[]'::jsonb,
			allowed_mcp JSONB NOT NULL DEFAULT '[]'::jsonb,
			is_admin BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TIMESTAMPTZ NOT NULL,
			last_used_at TIMESTAMPTZ,
			expires_at TIMESTAMPTZ,
			is_revoked BOOLEAN NOT NULL DEFAULT FALSE
		);`,
		`CREATE TABLE IF NOT EXISTS policies (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL REFERENCES projects(id),
			name TEXT NOT NULL,
			description TEXT,
			allowed_models JSONB NOT NULL,
			allowed_modalities JSONB NOT NULL DEFAULT '[]'::jsonb,
			allowed_toolsets JSONB NOT NULL DEFAULT '[]'::jsonb,
			allowed_mcp JSONB NOT NULL DEFAULT '[]'::jsonb,
			created_at TIMESTAMPTZ NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS budgets (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL REFERENCES projects(id),
			name TEXT NOT NULL,
			mode TEXT NOT NULL,
			limit_usd DOUBLE PRECISION NOT NULL DEFAULT 0,
			limit_requests BIGINT NOT NULL DEFAULT 0,
			window TEXT NOT NULL DEFAULT 'monthly',
			created_at TIMESTAMPTZ NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS audit_events (
			id TEXT PRIMARY KEY,
			project_id TEXT,
			actor_key_id TEXT,
			kind TEXT NOT NULL,
			resource_type TEXT NOT NULL,
			resource_id TEXT NOT NULL,
			metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at TIMESTAMPTZ NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS tool_definitions (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			description TEXT,
			implementation TEXT NOT NULL,
			input_schema JSONB NOT NULL DEFAULT '{}'::jsonb,
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			created_at TIMESTAMPTZ NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS toolsets (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			description TEXT,
			tool_ids JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS mcp_bindings (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			kind TEXT NOT NULL,
			upstream_url TEXT,
			toolset_id TEXT REFERENCES toolsets(id),
			headers_json JSONB NOT NULL DEFAULT '{}'::jsonb,
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			created_at TIMESTAMPTZ NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS archived_voices (
			provider TEXT NOT NULL,
			model TEXT NOT NULL DEFAULT '',
			voice_id TEXT NOT NULL,
			archived_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (provider, model, voice_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_project_id ON request_logs(project_id);`,
		`CREATE INDEX IF NOT EXISTS idx_virtual_keys_key_hash ON virtual_keys(key_hash);`,
		`CREATE INDEX IF NOT EXISTS idx_virtual_keys_project_id ON virtual_keys(project_id);`,
		`CREATE INDEX IF NOT EXISTS idx_policies_project_id ON policies(project_id);`,
		`CREATE INDEX IF NOT EXISTS idx_budgets_project_id ON budgets(project_id);`,
		`CREATE INDEX IF NOT EXISTS idx_audit_events_project_id ON audit_events(project_id);`,
		`CREATE INDEX IF NOT EXISTS idx_archived_voices_provider_model ON archived_voices(provider, model);`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) CreateProject(ctx context.Context, project store.Project) error {
	if project.ID == "" {
		project.ID = newID()
	}
	if project.CreatedAt.IsZero() {
		project.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO projects (id, name, description, created_at, archived_at)
		VALUES ($1, $2, $3, $4, $5)
	`, project.ID, project.Name, nullableString(project.Description), project.CreatedAt.UTC(), nullableTime(project.ArchivedAt))
	if err != nil {
		return fmt.Errorf("insert project: %w", err)
	}
	return nil
}

func (s *Store) ListProjects(ctx context.Context, includeArchived bool) ([]store.Project, error) {
	query := `SELECT id, name, description, created_at, archived_at FROM projects`
	if !includeArchived {
		query += ` WHERE archived_at IS NULL`
	}
	query += ` ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var projects []store.Project
	for rows.Next() {
		project, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		projects = append(projects, *project)
	}
	return projects, rows.Err()
}

func (s *Store) GetProject(ctx context.Context, id string) (*store.Project, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, description, created_at, archived_at FROM projects WHERE id = $1 LIMIT 1`, id)
	project, err := scanProject(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	return project, nil
}

func (s *Store) CreateVirtualKey(ctx context.Context, key store.VirtualKey) error {
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
	allowedModalities, err := json.Marshal(key.AllowedModalities)
	if err != nil {
		return fmt.Errorf("encode allowed_modalities: %w", err)
	}
	allowedToolsets, err := json.Marshal(key.AllowedToolsets)
	if err != nil {
		return fmt.Errorf("encode allowed_toolsets: %w", err)
	}
	allowedMCP, err := json.Marshal(key.AllowedMCP)
	if err != nil {
		return fmt.Errorf("encode allowed_mcp: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO virtual_keys (
			id, project_id, name, key_hash, key_prefix, rate_limit, allowed_models,
			allowed_modalities, allowed_toolsets, allowed_mcp, is_admin, created_at, last_used_at, expires_at, is_revoked
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, key.ID, key.ProjectID, key.Name, key.KeyHash, key.KeyPrefix, nullableString(key.RateLimit), string(allowedModels), string(allowedModalities), string(allowedToolsets), string(allowedMCP), key.IsAdmin, key.CreatedAt.UTC(), nullableTime(key.LastUsedAt), nullableTime(key.ExpiresAt), key.IsRevoked)
	if err != nil {
		return fmt.Errorf("insert virtual key: %w", err)
	}
	return nil
}

func (s *Store) GetVirtualKeyByHash(ctx context.Context, keyHash string) (*store.VirtualKey, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, name, key_hash, key_prefix, rate_limit, allowed_models,
		       allowed_modalities, allowed_toolsets, allowed_mcp, is_admin, created_at, last_used_at, expires_at, is_revoked
		FROM virtual_keys
		WHERE key_hash = $1
		LIMIT 1
	`, keyHash)
	key, err := scanVirtualKey(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	return key, nil
}

func (s *Store) ListVirtualKeys(ctx context.Context, projectID string, includeRevoked bool) ([]store.VirtualKey, error) {
	query := `
		SELECT id, project_id, name, key_hash, key_prefix, rate_limit, allowed_models,
		       allowed_modalities, allowed_toolsets, allowed_mcp, is_admin, created_at, last_used_at, expires_at, is_revoked
		FROM virtual_keys
	`
	var clauses []string
	var args []any
	add := func(format string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(format, len(args)))
	}
	if projectID != "" {
		add("project_id = $%d", projectID)
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
		return nil, fmt.Errorf("list virtual keys: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var keys []store.VirtualKey
	for rows.Next() {
		key, err := scanVirtualKey(rows)
		if err != nil {
			return nil, err
		}
		keys = append(keys, *key)
	}
	return keys, rows.Err()
}

func (s *Store) DeleteVirtualKey(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE virtual_keys SET is_revoked = TRUE WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("revoke virtual key: %w", err)
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) UpdateVirtualKeyLastUsed(ctx context.Context, id string, usedAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE virtual_keys SET last_used_at = $1 WHERE id = $2`, usedAt.UTC(), id)
	if err != nil {
		return fmt.Errorf("update virtual key last_used_at: %w", err)
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) CreatePolicy(ctx context.Context, policy store.Policy) error {
	if policy.ID == "" {
		policy.ID = newID()
	}
	if policy.CreatedAt.IsZero() {
		policy.CreatedAt = time.Now().UTC()
	}
	modelsJSON, err := json.Marshal(policy.AllowedModels)
	if err != nil {
		return fmt.Errorf("encode policy allowed_models: %w", err)
	}
	modalitiesJSON, err := json.Marshal(policy.AllowedModalities)
	if err != nil {
		return fmt.Errorf("encode policy allowed_modalities: %w", err)
	}
	toolsetsJSON, err := json.Marshal(policy.AllowedToolsets)
	if err != nil {
		return fmt.Errorf("encode policy allowed_toolsets: %w", err)
	}
	mcpJSON, err := json.Marshal(policy.AllowedMCP)
	if err != nil {
		return fmt.Errorf("encode policy allowed_mcp: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO policies (id, project_id, name, description, allowed_models, allowed_modalities, allowed_toolsets, allowed_mcp, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, policy.ID, policy.ProjectID, policy.Name, nullableString(policy.Description), string(modelsJSON), string(modalitiesJSON), string(toolsetsJSON), string(mcpJSON), policy.CreatedAt.UTC())
	if err != nil {
		return fmt.Errorf("insert policy: %w", err)
	}
	return nil
}

func (s *Store) ListPolicies(ctx context.Context, projectID string) ([]store.Policy, error) {
	query := `SELECT id, project_id, name, description, allowed_models, allowed_modalities, allowed_toolsets, allowed_mcp, created_at FROM policies`
	var args []any
	if projectID != "" {
		args = append(args, projectID)
		query += fmt.Sprintf(" WHERE project_id = $%d", len(args))
	}
	query += ` ORDER BY created_at DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list policies: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var policies []store.Policy
	for rows.Next() {
		policy, err := scanPolicy(rows)
		if err != nil {
			return nil, err
		}
		policies = append(policies, *policy)
	}
	return policies, rows.Err()
}

func (s *Store) CreateBudget(ctx context.Context, budget store.Budget) error {
	if budget.ID == "" {
		budget.ID = newID()
	}
	if budget.CreatedAt.IsZero() {
		budget.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO budgets (id, project_id, name, mode, limit_usd, limit_requests, window, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, budget.ID, budget.ProjectID, budget.Name, string(budget.Mode), budget.LimitUSD, budget.LimitRequests, budget.Window, budget.CreatedAt.UTC())
	if err != nil {
		return fmt.Errorf("insert budget: %w", err)
	}
	return nil
}

func (s *Store) ListBudgets(ctx context.Context, projectID string) ([]store.Budget, error) {
	query := `SELECT id, project_id, name, mode, limit_usd, limit_requests, window, created_at FROM budgets`
	var args []any
	if projectID != "" {
		args = append(args, projectID)
		query += fmt.Sprintf(" WHERE project_id = $%d", len(args))
	}
	query += ` ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list budgets: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var budgets []store.Budget
	for rows.Next() {
		var budget store.Budget
		var mode string
		if err := rows.Scan(&budget.ID, &budget.ProjectID, &budget.Name, &mode, &budget.LimitUSD, &budget.LimitRequests, &budget.Window, &budget.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan budget: %w", err)
		}
		budget.Mode = store.BudgetMode(mode)
		budgets = append(budgets, budget)
	}
	return budgets, rows.Err()
}

func (s *Store) LogAuditEvent(ctx context.Context, event store.AuditEvent) error {
	return s.LogAuditEventBatch(ctx, []store.AuditEvent{event})
}

func (s *Store) LogAuditEventBatch(ctx context.Context, events []store.AuditEvent) error {
	if len(events) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin audit batch: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO audit_events (id, project_id, actor_key_id, kind, resource_type, resource_id, metadata_json, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`)
	if err != nil {
		return fmt.Errorf("prepare audit batch: %w", err)
	}
	defer func() {
		_ = stmt.Close()
	}()
	for _, event := range events {
		if event.ID == "" {
			event.ID = newID()
		}
		if event.CreatedAt.IsZero() {
			event.CreatedAt = time.Now().UTC()
		}
		if event.MetadataJSON == "" {
			event.MetadataJSON = "{}"
		}
		if _, err = stmt.ExecContext(ctx, event.ID, nullableString(event.ProjectID), nullableString(event.ActorKeyID), event.Kind, event.ResourceType, event.ResourceID, event.MetadataJSON, event.CreatedAt.UTC()); err != nil {
			return fmt.Errorf("insert audit event: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit audit batch: %w", err)
	}
	return nil
}

func (s *Store) CreateToolDefinition(ctx context.Context, tool store.ToolDefinition) error {
	if tool.ID == "" {
		tool.ID = newID()
	}
	if tool.CreatedAt.IsZero() {
		tool.CreatedAt = time.Now().UTC()
	}
	if tool.InputSchema == "" {
		tool.InputSchema = "{}"
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tool_definitions (id, name, description, implementation, input_schema, enabled, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, tool.ID, tool.Name, nullableString(tool.Description), tool.Implementation, tool.InputSchema, tool.Enabled, tool.CreatedAt.UTC())
	if err != nil {
		return fmt.Errorf("insert tool definition: %w", err)
	}
	return nil
}

func (s *Store) ListToolDefinitions(ctx context.Context) ([]store.ToolDefinition, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, description, implementation, input_schema, enabled, created_at FROM tool_definitions ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list tool definitions: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()
	var tools []store.ToolDefinition
	for rows.Next() {
		var tool store.ToolDefinition
		var description sql.NullString
		if err := rows.Scan(&tool.ID, &tool.Name, &description, &tool.Implementation, &tool.InputSchema, &tool.Enabled, &tool.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan tool definition: %w", err)
		}
		if description.Valid {
			tool.Description = description.String
		}
		tools = append(tools, tool)
	}
	return tools, rows.Err()
}

func (s *Store) GetToolDefinition(ctx context.Context, id string) (*store.ToolDefinition, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, description, implementation, input_schema, enabled, created_at FROM tool_definitions WHERE id = $1 LIMIT 1`, id)
	var tool store.ToolDefinition
	var description sql.NullString
	if err := row.Scan(&tool.ID, &tool.Name, &description, &tool.Implementation, &tool.InputSchema, &tool.Enabled, &tool.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, fmt.Errorf("scan tool definition: %w", err)
	}
	if description.Valid {
		tool.Description = description.String
	}
	return &tool, nil
}

func (s *Store) CreateToolset(ctx context.Context, toolset store.Toolset) error {
	if toolset.ID == "" {
		toolset.ID = newID()
	}
	if toolset.CreatedAt.IsZero() {
		toolset.CreatedAt = time.Now().UTC()
	}
	toolIDs, err := json.Marshal(toolset.ToolIDs)
	if err != nil {
		return fmt.Errorf("encode toolset tool_ids: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO toolsets (id, name, description, tool_ids, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, toolset.ID, toolset.Name, nullableString(toolset.Description), string(toolIDs), toolset.CreatedAt.UTC())
	if err != nil {
		return fmt.Errorf("insert toolset: %w", err)
	}
	return nil
}

func (s *Store) ListToolsets(ctx context.Context) ([]store.Toolset, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, description, tool_ids, created_at FROM toolsets ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list toolsets: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()
	var toolsets []store.Toolset
	for rows.Next() {
		toolset, err := scanToolset(rows)
		if err != nil {
			return nil, err
		}
		toolsets = append(toolsets, *toolset)
	}
	return toolsets, rows.Err()
}

func (s *Store) GetToolset(ctx context.Context, id string) (*store.Toolset, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, description, tool_ids, created_at FROM toolsets WHERE id = $1 LIMIT 1`, id)
	toolset, err := scanToolset(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	return toolset, nil
}

func (s *Store) CreateMCPBinding(ctx context.Context, binding store.MCPBinding) error {
	if binding.ID == "" {
		binding.ID = newID()
	}
	if binding.CreatedAt.IsZero() {
		binding.CreatedAt = time.Now().UTC()
	}
	if binding.HeadersJSON == "" {
		binding.HeadersJSON = "{}"
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO mcp_bindings (id, name, kind, upstream_url, toolset_id, headers_json, enabled, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, binding.ID, binding.Name, string(binding.Kind), nullableString(binding.UpstreamURL), nullableString(binding.ToolsetID), binding.HeadersJSON, binding.Enabled, binding.CreatedAt.UTC())
	if err != nil {
		return fmt.Errorf("insert mcp binding: %w", err)
	}
	return nil
}

func (s *Store) ListMCPBindings(ctx context.Context) ([]store.MCPBinding, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, kind, upstream_url, toolset_id, headers_json, enabled, created_at FROM mcp_bindings ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list mcp bindings: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()
	var bindings []store.MCPBinding
	for rows.Next() {
		binding, err := scanMCPBinding(rows)
		if err != nil {
			return nil, err
		}
		bindings = append(bindings, *binding)
	}
	return bindings, rows.Err()
}

func (s *Store) GetMCPBinding(ctx context.Context, id string) (*store.MCPBinding, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, kind, upstream_url, toolset_id, headers_json, enabled, created_at FROM mcp_bindings WHERE id = $1 LIMIT 1`, id)
	binding, err := scanMCPBinding(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	return binding, nil
}

func (s *Store) ArchiveVoice(ctx context.Context, voice store.ArchivedVoice) error {
	voice.Provider = strings.TrimSpace(voice.Provider)
	voice.Model = strings.TrimSpace(voice.Model)
	voice.VoiceID = strings.TrimSpace(voice.VoiceID)
	if voice.Provider == "" || voice.VoiceID == "" {
		return fmt.Errorf("archive voice: provider and voice_id are required")
	}
	if voice.ArchivedAt.IsZero() {
		voice.ArchivedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO archived_voices (provider, model, voice_id, archived_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (provider, model, voice_id) DO UPDATE SET archived_at = EXCLUDED.archived_at
	`, voice.Provider, voice.Model, voice.VoiceID, voice.ArchivedAt.UTC())
	if err != nil {
		return fmt.Errorf("archive voice: %w", err)
	}
	return nil
}

func (s *Store) UnarchiveVoice(ctx context.Context, provider string, model string, voiceID string) error {
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	voiceID = strings.TrimSpace(voiceID)
	result, err := s.db.ExecContext(ctx, `DELETE FROM archived_voices WHERE provider = $1 AND model = $2 AND voice_id = $3`, provider, model, voiceID)
	if err != nil {
		return fmt.Errorf("unarchive voice: %w", err)
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 && model != "" {
		result, err = s.db.ExecContext(ctx, `DELETE FROM archived_voices WHERE provider = $1 AND model = '' AND voice_id = $2`, provider, voiceID)
		if err != nil {
			return fmt.Errorf("unarchive voice: %w", err)
		}
		affected, err = result.RowsAffected()
	}
	if err == nil && affected == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) GetArchivedVoice(ctx context.Context, provider string, model string, voiceID string) (*store.ArchivedVoice, error) {
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	voiceID = strings.TrimSpace(voiceID)

	query := `SELECT provider, model, voice_id, archived_at FROM archived_voices WHERE provider = $1 AND voice_id = $2`
	args := []any{provider, voiceID}
	if model == "" {
		query += ` AND model = '' LIMIT 1`
	} else {
		args = append(args, model, model)
		query += ` AND (model = $3 OR model = '') ORDER BY CASE WHEN model = $4 THEN 0 ELSE 1 END LIMIT 1`
	}

	row := s.db.QueryRowContext(ctx, query, args...)
	voice, err := scanArchivedVoice(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	return voice, nil
}

func (s *Store) ListArchivedVoices(ctx context.Context, provider string, model string) ([]store.ArchivedVoice, error) {
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)

	query := `SELECT provider, model, voice_id, archived_at FROM archived_voices`
	args := make([]any, 0, 2)
	clauses := make([]string, 0, 2)
	if provider != "" {
		args = append(args, provider)
		clauses = append(clauses, fmt.Sprintf("provider = $%d", len(args)))
	}
	if model != "" && provider != "" {
		args = append(args, model)
		clauses = append(clauses, fmt.Sprintf("(model = $%d OR model = '')", len(args)))
	} else if model != "" {
		args = append(args, model)
		clauses = append(clauses, fmt.Sprintf("model = $%d", len(args)))
	}
	if len(clauses) > 0 {
		query += ` WHERE ` + strings.Join(clauses, " AND ")
	}
	query += ` ORDER BY archived_at DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list archived voices: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var voices []store.ArchivedVoice
	for rows.Next() {
		voice, err := scanArchivedVoice(rows)
		if err != nil {
			return nil, err
		}
		voices = append(voices, *voice)
	}
	return voices, rows.Err()
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

func scanProject(scanner interface{ Scan(dest ...any) error }) (*store.Project, error) {
	var project store.Project
	var description sql.NullString
	var archivedAt sql.NullTime
	if err := scanner.Scan(&project.ID, &project.Name, &description, &project.CreatedAt, &archivedAt); err != nil {
		return nil, err
	}
	if description.Valid {
		project.Description = description.String
	}
	if archivedAt.Valid {
		project.ArchivedAt = &archivedAt.Time
	}
	return &project, nil
}

func scanVirtualKey(scanner interface{ Scan(dest ...any) error }) (*store.VirtualKey, error) {
	var key store.VirtualKey
	var allowedModels, allowedModalities, allowedToolsets, allowedMCP string
	var rateLimit sql.NullString
	var lastUsedAt, expiresAt sql.NullTime
	if err := scanner.Scan(
		&key.ID,
		&key.ProjectID,
		&key.Name,
		&key.KeyHash,
		&key.KeyPrefix,
		&rateLimit,
		&allowedModels,
		&allowedModalities,
		&allowedToolsets,
		&allowedMCP,
		&key.IsAdmin,
		&key.CreatedAt,
		&lastUsedAt,
		&expiresAt,
		&key.IsRevoked,
	); err != nil {
		return nil, err
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
		return nil, fmt.Errorf("decode virtual key allowed_models: %w", err)
	}
	if err := json.Unmarshal([]byte(allowedModalities), &key.AllowedModalities); err != nil {
		return nil, fmt.Errorf("decode virtual key allowed_modalities: %w", err)
	}
	if err := json.Unmarshal([]byte(allowedToolsets), &key.AllowedToolsets); err != nil {
		return nil, fmt.Errorf("decode virtual key allowed_toolsets: %w", err)
	}
	if err := json.Unmarshal([]byte(allowedMCP), &key.AllowedMCP); err != nil {
		return nil, fmt.Errorf("decode virtual key allowed_mcp: %w", err)
	}
	return &key, nil
}

func scanPolicy(scanner interface{ Scan(dest ...any) error }) (*store.Policy, error) {
	var policy store.Policy
	var description sql.NullString
	var allowedModels, allowedModalities, allowedToolsets, allowedMCP string
	if err := scanner.Scan(&policy.ID, &policy.ProjectID, &policy.Name, &description, &allowedModels, &allowedModalities, &allowedToolsets, &allowedMCP, &policy.CreatedAt); err != nil {
		return nil, err
	}
	if description.Valid {
		policy.Description = description.String
	}
	if err := json.Unmarshal([]byte(allowedModels), &policy.AllowedModels); err != nil {
		return nil, fmt.Errorf("decode policy allowed_models: %w", err)
	}
	if err := json.Unmarshal([]byte(allowedModalities), &policy.AllowedModalities); err != nil {
		return nil, fmt.Errorf("decode policy allowed_modalities: %w", err)
	}
	if err := json.Unmarshal([]byte(allowedToolsets), &policy.AllowedToolsets); err != nil {
		return nil, fmt.Errorf("decode policy allowed_toolsets: %w", err)
	}
	if err := json.Unmarshal([]byte(allowedMCP), &policy.AllowedMCP); err != nil {
		return nil, fmt.Errorf("decode policy allowed_mcp: %w", err)
	}
	return &policy, nil
}

func scanToolset(scanner interface{ Scan(dest ...any) error }) (*store.Toolset, error) {
	var toolset store.Toolset
	var description sql.NullString
	var toolIDs string
	if err := scanner.Scan(&toolset.ID, &toolset.Name, &description, &toolIDs, &toolset.CreatedAt); err != nil {
		return nil, err
	}
	if description.Valid {
		toolset.Description = description.String
	}
	if err := json.Unmarshal([]byte(toolIDs), &toolset.ToolIDs); err != nil {
		return nil, fmt.Errorf("decode toolset tool_ids: %w", err)
	}
	return &toolset, nil
}

func scanMCPBinding(scanner interface{ Scan(dest ...any) error }) (*store.MCPBinding, error) {
	var binding store.MCPBinding
	var kind string
	var upstreamURL, toolsetID sql.NullString
	if err := scanner.Scan(&binding.ID, &binding.Name, &kind, &upstreamURL, &toolsetID, &binding.HeadersJSON, &binding.Enabled, &binding.CreatedAt); err != nil {
		return nil, err
	}
	binding.Kind = store.MCPBindingKind(kind)
	if upstreamURL.Valid {
		binding.UpstreamURL = upstreamURL.String
	}
	if toolsetID.Valid {
		binding.ToolsetID = toolsetID.String
	}
	return &binding, nil
}

func scanArchivedVoice(scanner interface{ Scan(dest ...any) error }) (*store.ArchivedVoice, error) {
	var voice store.ArchivedVoice
	if err := scanner.Scan(&voice.Provider, &voice.Model, &voice.VoiceID, &voice.ArchivedAt); err != nil {
		return nil, err
	}
	return &voice, nil
}

func usageWhereClause(filter store.UsageFilter) (string, []any) {
	var clauses []string
	var args []any

	addClause := func(template string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(template, len(args)))
	}

	if filter.KeyID != "" {
		addClause("request_logs.key_id = $%d", filter.KeyID)
	}
	if filter.OwnerID != "" {
		addClause("api_keys.owner_id = $%d", filter.OwnerID)
	}
	if filter.ProjectID != "" {
		addClause("request_logs.project_id = $%d", filter.ProjectID)
	}
	if filter.Model != "" {
		addClause("request_logs.model = $%d", filter.Model)
	}
	if filter.Modality != "" {
		addClause("request_logs.modality = $%d", string(filter.Modality))
	}
	if filter.From != nil {
		addClause("request_logs.created_at >= $%d", filter.From.UTC())
	}
	if filter.To != nil {
		addClause("request_logs.created_at < $%d", filter.To.UTC())
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
		LEFT JOIN virtual_keys ON virtual_keys.id = request_logs.key_id
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

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC()
}
