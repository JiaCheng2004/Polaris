package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/store"
)

func (s *Store) ensureFilesUpgrade(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS files (
			polaris_id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL REFERENCES projects(id),
			key_id TEXT NOT NULL,
			sha256 TEXT NOT NULL,
			size BIGINT NOT NULL,
			mime_type TEXT NOT NULL,
			original_filename TEXT NOT NULL,
			purpose TEXT NOT NULL,
			origin_url TEXT,
			inline_bytes BYTEA,
			blob_key TEXT,
			metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at TIMESTAMPTZ,
			deleted_at TIMESTAMPTZ
		);`,
		`CREATE INDEX IF NOT EXISTS idx_files_project_id ON files(project_id);`,
		`CREATE INDEX IF NOT EXISTS idx_files_sha256 ON files(sha256);`,
		`CREATE INDEX IF NOT EXISTS idx_files_expires_at ON files(expires_at);`,
		`CREATE TABLE IF NOT EXISTS file_provider_handles (
			polaris_id TEXT NOT NULL REFERENCES files(polaris_id) ON DELETE CASCADE,
			provider TEXT NOT NULL,
			provider_file_id TEXT NOT NULL,
			purpose TEXT NOT NULL,
			size_bytes BIGINT NOT NULL,
			mime_type TEXT NOT NULL,
			expires_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY(polaris_id, provider)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_fph_provider ON file_provider_handles(provider, expires_at);`,
		`CREATE TABLE IF NOT EXISTS file_understanding_artifacts (
			sha256 TEXT NOT NULL,
			processor TEXT NOT NULL,
			version TEXT NOT NULL,
			mime_type TEXT NOT NULL,
			text TEXT NOT NULL,
			warning TEXT,
			metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
			artifact_json JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY(sha256, processor, version)
		);`,
		`ALTER TABLE file_understanding_artifacts ADD COLUMN IF NOT EXISTS artifact_json JSONB NOT NULL DEFAULT '{}'::jsonb;`,
		`CREATE INDEX IF NOT EXISTS idx_fua_sha256 ON file_understanding_artifacts(sha256);`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) CreateFile(ctx context.Context, file store.File) error {
	if file.CreatedAt.IsZero() {
		file.CreatedAt = time.Now().UTC()
	}
	metadataJSON, err := json.Marshal(file.Metadata)
	if err != nil {
		return fmt.Errorf("encode file metadata: %w", err)
	}
	if len(metadataJSON) == 0 || string(metadataJSON) == "null" {
		metadataJSON = []byte("{}")
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO files (
			polaris_id, project_id, key_id, sha256, size, mime_type, original_filename,
			purpose, origin_url, inline_bytes, blob_key, metadata_json, created_at, expires_at, deleted_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, file.PolarisID, file.ProjectID, file.KeyID, file.Sha256, file.Size, file.MimeType, file.OriginalFilename,
		string(file.Purpose), nullableString(file.OriginURL), nullableBytes(file.InlineBytes), nullableString(file.BlobKey),
		string(metadataJSON), file.CreatedAt.UTC(), nullableTime(file.ExpiresAt), nullableTime(file.DeletedAt))
	if err != nil {
		return fmt.Errorf("insert file: %w", err)
	}
	return nil
}

func (s *Store) GetFile(ctx context.Context, polarisID string) (*store.File, error) {
	row := s.db.QueryRowContext(ctx, fileSelectSQL()+` WHERE polaris_id = $1 AND deleted_at IS NULL LIMIT 1`, polarisID)
	file, err := scanFile(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	return file, nil
}

func (s *Store) GetFileForProject(ctx context.Context, polarisID, projectID string) (*store.File, error) {
	row := s.db.QueryRowContext(ctx, fileSelectSQL()+` WHERE polaris_id = $1 AND project_id = $2 AND deleted_at IS NULL LIMIT 1`, polarisID, projectID)
	file, err := scanFile(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	return file, nil
}

func (s *Store) ListFiles(ctx context.Context, filter store.FileFilter) ([]store.File, error) {
	query := fileSelectSQL()
	clauses := []string{"deleted_at IS NULL"}
	args := []any{}
	add := func(format string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(format, len(args)))
	}
	if filter.ProjectID != "" {
		add("project_id = $%d", filter.ProjectID)
	}
	if filter.Purpose != "" {
		add("purpose = $%d", string(filter.Purpose))
	}
	if filter.AfterID != "" {
		add("polaris_id < $%d", filter.AfterID)
	}
	query = appendWhereClauses(query, clauses)
	limit := filter.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	args = append(args, limit)
	query += fmt.Sprintf(" ORDER BY polaris_id DESC LIMIT $%d", len(args))

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var files []store.File
	for rows.Next() {
		file, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, *file)
	}
	return files, rows.Err()
}

func (s *Store) DeleteFile(ctx context.Context, polarisID string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM files WHERE polaris_id = $1`, polarisID)
	if err != nil {
		return fmt.Errorf("delete file: %w", err)
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) GetFileInline(ctx context.Context, polarisID string) ([]byte, error) {
	var data []byte
	err := s.db.QueryRowContext(ctx, `SELECT inline_bytes FROM files WHERE polaris_id = $1 AND deleted_at IS NULL LIMIT 1`, polarisID).Scan(&data)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, fmt.Errorf("get file inline bytes: %w", err)
	}
	if len(data) == 0 {
		return nil, store.ErrNotFound
	}
	return data, nil
}

func (s *Store) PutFileProviderHandle(ctx context.Context, handle store.FileProviderHandle) error {
	if handle.CreatedAt.IsZero() {
		handle.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO file_provider_handles (
			polaris_id, provider, provider_file_id, purpose, size_bytes, mime_type, expires_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT(polaris_id, provider) DO UPDATE SET
			provider_file_id = excluded.provider_file_id,
			purpose = excluded.purpose,
			size_bytes = excluded.size_bytes,
			mime_type = excluded.mime_type,
			expires_at = excluded.expires_at,
			created_at = excluded.created_at
	`, handle.PolarisID, handle.Provider, handle.ProviderFileID, string(handle.Purpose), handle.SizeBytes, handle.MimeType, nullableTime(handle.ExpiresAt), handle.CreatedAt.UTC())
	if err != nil {
		return fmt.Errorf("put file provider handle: %w", err)
	}
	return nil
}

func (s *Store) GetFileProviderHandle(ctx context.Context, polarisID, provider string) (*store.FileProviderHandle, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT polaris_id, provider, provider_file_id, purpose, size_bytes, mime_type, expires_at, created_at
		FROM file_provider_handles
		WHERE polaris_id = $1 AND provider = $2
		LIMIT 1
	`, polarisID, provider)
	handle, err := scanFileProviderHandle(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return handle, true, nil
}

func (s *Store) DeleteFileProviderHandlesByPolarisID(ctx context.Context, polarisID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM file_provider_handles WHERE polaris_id = $1`, polarisID)
	if err != nil {
		return fmt.Errorf("delete file provider handles: %w", err)
	}
	return nil
}

func (s *Store) PurgeExpiredFiles(ctx context.Context, now time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM files WHERE expires_at IS NOT NULL AND expires_at < $1`, now.UTC())
	if err != nil {
		return 0, fmt.Errorf("purge expired files: %w", err)
	}
	return result.RowsAffected()
}

func (s *Store) SumProjectFileBytes(ctx context.Context, projectID string) (int64, error) {
	var total sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(size), 0) FROM files WHERE project_id = $1 AND deleted_at IS NULL`, projectID).Scan(&total); err != nil {
		return 0, fmt.Errorf("sum project file bytes: %w", err)
	}
	if !total.Valid {
		return 0, nil
	}
	return total.Int64, nil
}

func (s *Store) CountProjectFiles(ctx context.Context, projectID string) (int64, error) {
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM files WHERE project_id = $1 AND deleted_at IS NULL`, projectID).Scan(&total); err != nil {
		return 0, fmt.Errorf("count project files: %w", err)
	}
	return total, nil
}

func (s *Store) CountFilesByBlobKey(ctx context.Context, blobKey string) (int64, error) {
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM files WHERE blob_key = $1 AND deleted_at IS NULL`, blobKey).Scan(&total); err != nil {
		return 0, fmt.Errorf("count files by blob key: %w", err)
	}
	return total, nil
}

func (s *Store) PutFileUnderstandingArtifact(ctx context.Context, artifact store.FileUnderstandingArtifact) error {
	if artifact.CreatedAt.IsZero() {
		artifact.CreatedAt = time.Now().UTC()
	}
	if artifact.MetadataJSON == "" {
		artifact.MetadataJSON = "{}"
	}
	if artifact.ArtifactJSON == "" {
		artifact.ArtifactJSON = "{}"
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO file_understanding_artifacts (
			sha256, processor, version, mime_type, text, warning, metadata_json, artifact_json, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT(sha256, processor, version) DO UPDATE SET
			mime_type = excluded.mime_type,
			text = excluded.text,
			warning = excluded.warning,
			metadata_json = excluded.metadata_json,
			artifact_json = excluded.artifact_json,
			created_at = excluded.created_at
	`, artifact.Sha256, artifact.Processor, artifact.Version, artifact.MimeType, artifact.Text, nullableString(artifact.Warning), artifact.MetadataJSON, artifact.ArtifactJSON, artifact.CreatedAt.UTC())
	if err != nil {
		return fmt.Errorf("put file understanding artifact: %w", err)
	}
	return nil
}

func (s *Store) GetFileUnderstandingArtifact(ctx context.Context, sha256, processor, version string) (*store.FileUnderstandingArtifact, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT sha256, processor, version, mime_type, text, warning, metadata_json::text, artifact_json::text, created_at
		FROM file_understanding_artifacts
		WHERE sha256 = $1 AND processor = $2 AND version = $3
		LIMIT 1
	`, sha256, processor, version)
	artifact, err := scanFileUnderstandingArtifact(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return artifact, true, nil
}

func fileSelectSQL() string {
	return `SELECT polaris_id, project_id, key_id, sha256, size, mime_type, original_filename,
		purpose, origin_url, inline_bytes, blob_key, metadata_json, created_at, expires_at, deleted_at
		FROM files`
}

func scanFileUnderstandingArtifact(scanner interface {
	Scan(dest ...any) error
}) (*store.FileUnderstandingArtifact, error) {
	var artifact store.FileUnderstandingArtifact
	var warning sql.NullString
	if err := scanner.Scan(
		&artifact.Sha256,
		&artifact.Processor,
		&artifact.Version,
		&artifact.MimeType,
		&artifact.Text,
		&warning,
		&artifact.MetadataJSON,
		&artifact.ArtifactJSON,
		&artifact.CreatedAt,
	); err != nil {
		return nil, err
	}
	if warning.Valid {
		artifact.Warning = warning.String
	}
	return &artifact, nil
}

func scanFile(scanner interface {
	Scan(dest ...any) error
}) (*store.File, error) {
	var file store.File
	var purpose string
	var originURL sql.NullString
	var inlineBytes []byte
	var blobKey sql.NullString
	var metadataJSON string
	var expiresAt sql.NullTime
	var deletedAt sql.NullTime
	if err := scanner.Scan(
		&file.PolarisID,
		&file.ProjectID,
		&file.KeyID,
		&file.Sha256,
		&file.Size,
		&file.MimeType,
		&file.OriginalFilename,
		&purpose,
		&originURL,
		&inlineBytes,
		&blobKey,
		&metadataJSON,
		&file.CreatedAt,
		&expiresAt,
		&deletedAt,
	); err != nil {
		return nil, err
	}
	file.Purpose = modality.FilePurpose(purpose)
	if originURL.Valid {
		file.OriginURL = originURL.String
	}
	if len(inlineBytes) > 0 {
		file.InlineBytes = append([]byte(nil), inlineBytes...)
	}
	if blobKey.Valid {
		file.BlobKey = blobKey.String
	}
	if metadataJSON == "" {
		metadataJSON = "{}"
	}
	if err := json.Unmarshal([]byte(metadataJSON), &file.Metadata); err != nil {
		return nil, fmt.Errorf("decode file metadata: %w", err)
	}
	if file.Metadata == nil {
		file.Metadata = map[string]string{}
	}
	if expiresAt.Valid {
		value := expiresAt.Time
		file.ExpiresAt = &value
	}
	if deletedAt.Valid {
		value := deletedAt.Time
		file.DeletedAt = &value
	}
	return &file, nil
}

func scanFileProviderHandle(scanner interface {
	Scan(dest ...any) error
}) (*store.FileProviderHandle, error) {
	var handle store.FileProviderHandle
	var purpose string
	var expiresAt sql.NullTime
	if err := scanner.Scan(
		&handle.PolarisID,
		&handle.Provider,
		&handle.ProviderFileID,
		&purpose,
		&handle.SizeBytes,
		&handle.MimeType,
		&expiresAt,
		&handle.CreatedAt,
	); err != nil {
		return nil, err
	}
	handle.Purpose = modality.FilePurpose(purpose)
	if expiresAt.Valid {
		value := expiresAt.Time
		handle.ExpiresAt = &value
	}
	return &handle, nil
}

func nullableBytes(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return value
}
