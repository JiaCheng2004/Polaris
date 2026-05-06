package handler

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	providerpkg "github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/JiaCheng2004/Polaris/internal/store/blob"
	"github.com/gin-gonic/gin"
	"github.com/oklog/ulid/v2"
)

type FilesHandler struct {
	runtime     *gwruntime.Holder
	store       store.Store
	auditLogger *store.AsyncAuditLogger
	logger      *slog.Logger
}

type fileURLUploadRequest struct {
	URL       string            `json:"url"`
	Filename  string            `json:"filename,omitempty"`
	MimeType  string            `json:"mime_type,omitempty"`
	Purpose   string            `json:"purpose,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	ExpiresAt string            `json:"expires_at,omitempty"`
}

type fileObjectResponse struct {
	ID        string               `json:"id"`
	Object    string               `json:"object"`
	Bytes     int64                `json:"bytes"`
	CreatedAt int64                `json:"created_at"`
	Filename  string               `json:"filename"`
	Purpose   modality.FilePurpose `json:"purpose"`
	Polaris   filePolarisMetadata  `json:"polaris"`
}

type filePolarisMetadata struct {
	Sha256                string            `json:"sha256"`
	MimeType              string            `json:"mime_type"`
	ProjectID             string            `json:"project_id"`
	ExpiresAt             *int64            `json:"expires_at"`
	MaterializedProviders []string          `json:"materialized_providers"`
	HasInlineBytes        bool              `json:"has_inline_bytes"`
	HasBlobBacking        bool              `json:"has_blob_backing"`
	OriginURL             string            `json:"origin_url,omitempty"`
	Metadata              map[string]string `json:"metadata,omitempty"`
	ContentURL            string            `json:"content_url,omitempty"`
}

type fileListResponse struct {
	Object  string               `json:"object"`
	Data    []fileObjectResponse `json:"data"`
	HasMore bool                 `json:"has_more"`
}

type fileMaterializationResponse struct {
	Object         string               `json:"object"`
	FileID         string               `json:"file_id"`
	Provider       string               `json:"provider"`
	ProviderFileID string               `json:"provider_file_id"`
	Purpose        modality.FilePurpose `json:"purpose"`
	Bytes          int64                `json:"bytes"`
	MimeType       string               `json:"mime_type"`
	CreatedAt      int64                `json:"created_at"`
	ExpiresAt      *int64               `json:"expires_at,omitempty"`
	Cached         bool                 `json:"cached"`
}

func NewFilesHandler(runtime *gwruntime.Holder, appStore store.Store, auditLogger *store.AsyncAuditLogger, logger *slog.Logger) *FilesHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &FilesHandler{runtime: runtime, store: appStore, auditLogger: auditLogger, logger: logger}
}

func (h *FilesHandler) Upload(c *gin.Context) {
	cfg, ok := h.requireEnabled(c)
	if !ok {
		return
	}
	auth := middleware.GetAuthContext(c)
	if !middleware.ModalityScopeAllowed(auth.AllowedModalities, auth.PolicyModalities, modality.ModalityFiles) {
		httputil.WriteError(c, httputil.NewError(http.StatusForbidden, "permission_error", "modality_not_allowed", "files", "API key is not permitted to use the files modality."))
		return
	}
	projectID, err := h.projectID(c, auth)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}

	input, err := h.readUploadInput(c, cfg)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	defer input.close()

	purpose, err := normalizeFilePurpose(input.purpose)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	mimeType, err := validateUploadMIME(input.mimeType, input.filename, input.data, cfg.Ingestion.AllowedMime)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	filename := strings.TrimSpace(input.filename)
	if filename == "" {
		filename = "file"
	}
	filename = filepath.Base(filename)

	polarisID, err := newPolarisFileID()
	if err != nil {
		httputil.WriteError(c, httputil.NewError(http.StatusInternalServerError, "internal_error", "file_id_generation_failed", "", "Unable to generate file id."))
		return
	}
	sha := sha256.Sum256(input.data)
	file := store.File{
		PolarisID:        polarisID,
		ProjectID:        projectID,
		KeyID:            auth.KeyID,
		Sha256:           hex.EncodeToString(sha[:]),
		Size:             int64(len(input.data)),
		MimeType:         mimeType,
		OriginalFilename: filename,
		Purpose:          purpose,
		OriginURL:        input.originURL,
		Metadata:         input.metadata,
		CreatedAt:        time.Now().UTC(),
		ExpiresAt:        input.expiresAt,
	}
	if err := h.enforceFileBudgets(c, file); err != nil {
		httputil.WriteError(c, err)
		return
	}
	if err := h.persistBytes(c.Request.Context(), cfg, &file, input.data); err != nil {
		httputil.WriteError(c, err)
		return
	}
	if err := h.store.CreateFile(c.Request.Context(), file); err != nil {
		httputil.WriteError(c, err)
		return
	}

	h.logAudit(c, "file.upload", file, map[string]any{"size": file.Size, "mime_type": file.MimeType})
	middleware.SetRequestOutcome(c, middleware.RequestOutcome{
		Modality:   modality.ModalityFiles,
		StatusCode: http.StatusOK,
	})
	c.JSON(http.StatusOK, h.fileResponse(c, cfg, file))
}

func (h *FilesHandler) List(c *gin.Context) {
	cfg, ok := h.requireEnabled(c)
	if !ok {
		return
	}
	auth := middleware.GetAuthContext(c)
	projectID, err := h.projectID(c, auth)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	limit := 100
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_limit", "limit", "Field 'limit' must be a positive integer."))
			return
		}
		limit = parsed
	}
	filter := store.FileFilter{
		ProjectID: projectID,
		Limit:     limit + 1,
		AfterID:   strings.TrimSpace(c.Query("after")),
	}
	if purpose := strings.TrimSpace(c.Query("purpose")); purpose != "" {
		parsed, err := normalizeFilePurpose(purpose)
		if err != nil {
			httputil.WriteError(c, err)
			return
		}
		filter.Purpose = parsed
	}
	files, err := h.store.ListFiles(c.Request.Context(), filter)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	hasMore := len(files) > limit
	if hasMore {
		files = files[:limit]
	}
	response := fileListResponse{Object: "list", HasMore: hasMore}
	for _, file := range files {
		response.Data = append(response.Data, h.fileResponse(c, cfg, file))
	}
	c.JSON(http.StatusOK, response)
}

func (h *FilesHandler) Get(c *gin.Context) {
	cfg, ok := h.requireEnabled(c)
	if !ok {
		return
	}
	file, err := h.getAuthorizedFile(c)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, h.fileResponse(c, cfg, *file))
}

func (h *FilesHandler) Delete(c *gin.Context) {
	cfg, ok := h.requireEnabled(c)
	if !ok {
		return
	}
	file, err := h.getAuthorizedFile(c)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	if err := h.store.DeleteFileProviderHandlesByPolarisID(c.Request.Context(), file.PolarisID); err != nil {
		httputil.WriteError(c, err)
		return
	}
	if err := h.store.DeleteFile(c.Request.Context(), file.PolarisID); err != nil {
		httputil.WriteError(c, err)
		return
	}
	if file.BlobKey != "" {
		remaining, err := h.store.CountFilesByBlobKey(c.Request.Context(), file.BlobKey)
		if err != nil {
			httputil.WriteError(c, err)
			return
		}
		if remaining == 0 {
			if blobStore, err := blobStoreFromConfig(cfg); err == nil && blobStore != nil {
				_ = blobStore.Delete(c.Request.Context(), file.BlobKey)
				_ = blobStore.Close()
			}
		}
	}
	h.logAudit(c, "file.delete", *file, nil)
	c.Status(http.StatusNoContent)
}

func (h *FilesHandler) Content(c *gin.Context) {
	cfg, ok := h.requireEnabled(c)
	if !ok {
		return
	}
	token, err := parseFileDownloadToken(strings.TrimSpace(c.Query("token")))
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	if token.PolarisID != c.Param("id") {
		httputil.WriteError(c, invalidFileDownloadTokenError())
		return
	}
	auth := middleware.GetAuthContext(c)
	if token.KeyID != auth.KeyID {
		httputil.WriteError(c, invalidFileDownloadTokenError())
		return
	}
	projectID, err := h.projectID(c, auth)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	if token.ProjectID != projectID {
		httputil.WriteError(c, invalidFileDownloadTokenError())
		return
	}
	file, err := h.store.GetFileForProject(c.Request.Context(), token.PolarisID, token.ProjectID)
	if err != nil {
		writeFileStoreError(c, err)
		return
	}
	if expired(file) {
		httputil.WriteError(c, httputil.NewError(http.StatusGone, "invalid_request_error", "file_expired", "id", "File has expired."))
		return
	}
	if len(file.InlineBytes) > 0 {
		h.logAudit(c, "file.content_download", *file, nil)
		c.Data(http.StatusOK, file.MimeType, file.InlineBytes)
		return
	}
	if file.BlobKey != "" {
		blobStore, err := h.blobStore(c)
		if err != nil {
			httputil.WriteError(c, err)
			return
		}
		if blobStore != nil {
			defer func() {
				_ = blobStore.Close()
			}()
			body, size, err := blobStore.Get(c.Request.Context(), file.BlobKey)
			if err != nil {
				httputil.WriteError(c, httputil.NewError(http.StatusNotFound, "invalid_request_error", "file_not_found", "id", "File content was not found."))
				return
			}
			defer func() {
				_ = body.Close()
			}()
			h.logAudit(c, "file.content_download", *file, nil)
			c.DataFromReader(http.StatusOK, size, file.MimeType, body, nil)
			return
		}
	}
	if file.OriginURL != "" {
		resp, err := middleware.NewSSRFClient(cfg.SSRF, cfgTimeout(c)).Get(c.Request.Context(), file.OriginURL)
		if err != nil {
			httputil.WriteError(c, ssrfError(err))
			return
		}
		defer func() {
			_ = resp.Body.Close()
		}()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			httputil.WriteError(c, httputil.NewError(http.StatusBadGateway, "provider_error", "origin_fetch_failed", "url", "Origin URL did not return a successful response."))
			return
		}
		h.logAudit(c, "file.content_download", *file, nil)
		c.DataFromReader(http.StatusOK, resp.ContentLength, file.MimeType, resp.Body, nil)
		return
	}
	httputil.WriteError(c, httputil.NewError(http.StatusNotFound, "invalid_request_error", "file_not_found", "id", "File content was not found."))
}

func (h *FilesHandler) Materialize(c *gin.Context) {
	cfg, ok := h.requireEnabled(c)
	if !ok {
		return
	}
	file, err := h.getAuthorizedFile(c)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	providerName := strings.TrimSpace(c.Query("provider"))
	if providerName == "" {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_provider", "provider", "Field 'provider' is required."))
		return
	}
	if cached, ok, err := h.store.GetFileProviderHandle(c.Request.Context(), file.PolarisID, providerName); err != nil {
		httputil.WriteError(c, err)
		return
	} else if ok && !providerHandleExpired(cached) {
		c.JSON(http.StatusOK, fileHandleResponse(cached, true))
		return
	}

	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	if snapshot == nil || snapshot.Registry == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "provider", "Provider registry is unavailable."))
		return
	}
	adapter, err := snapshot.Registry.GetFilesAdapter(providerName)
	if err != nil {
		if errors.Is(err, providerpkg.ErrAdapterMissing) {
			h.logAudit(c, "file.materialize", *file, map[string]any{"provider": providerName, "supported": false})
			httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "capability_not_supported", "provider_lacks_files_api", "provider", "Provider does not expose a files API."))
			return
		}
		httputil.WriteError(c, err)
		return
	}
	data, err := h.readFileBytes(c, cfg, file)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	handle, err := adapter.Materialize(c.Request.Context(), &modality.FileMaterializeRequest{
		PolarisID: file.PolarisID,
		MimeType:  file.MimeType,
		Filename:  file.OriginalFilename,
		Purpose:   file.Purpose,
		SourceURL: file.OriginURL,
		InlineSrc: data,
		Sha256:    file.Sha256,
		Size:      file.Size,
	})
	if err != nil {
		h.logAudit(c, "file.materialize", *file, map[string]any{"provider": providerName, "supported": true, "failed": true})
		httputil.WriteError(c, httputil.NewError(http.StatusBadGateway, "provider_error", "materialization_failed", "provider", "Provider file materialization failed."))
		return
	}
	if handle.Provider == "" {
		handle.Provider = providerName
	}
	record := store.FileProviderHandle{
		PolarisID:      file.PolarisID,
		Provider:       providerName,
		ProviderFileID: handle.ProviderFileID,
		Purpose:        handle.Purpose,
		SizeBytes:      handle.SizeBytes,
		MimeType:       firstNonEmpty(handle.MimeType, file.MimeType),
		ExpiresAt:      handle.ExpiresAt,
		CreatedAt:      handle.CreatedAt,
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	if record.Purpose == "" {
		record.Purpose = file.Purpose
	}
	if record.SizeBytes == 0 {
		record.SizeBytes = file.Size
	}
	if err := h.store.PutFileProviderHandle(c.Request.Context(), record); err != nil {
		httputil.WriteError(c, err)
		return
	}
	h.logAudit(c, "file.materialize", *file, map[string]any{"provider": providerName, "provider_file_id": record.ProviderFileID, "supported": true})
	c.JSON(http.StatusOK, fileHandleResponse(&record, false))
}

type uploadInput struct {
	data      []byte
	filename  string
	mimeType  string
	purpose   string
	metadata  map[string]string
	originURL string
	expiresAt *time.Time
	closers   []io.Closer
}

func (i *uploadInput) close() {
	for _, closer := range i.closers {
		_ = closer.Close()
	}
}

func (h *FilesHandler) readUploadInput(c *gin.Context, cfg config.FilesConfig) (*uploadInput, error) {
	contentType := c.GetHeader("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if strings.HasPrefix(mediaType, "multipart/") {
		header, err := c.FormFile("file")
		if err != nil {
			return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_file", "file", "Multipart upload must include a file field named 'file'.")
		}
		data, mimeType, err := readMultipartFile(header)
		if err != nil {
			if httputil.IsRequestBodyTooLarge(err) {
				return nil, httputil.RequestBodyTooLargeError(config.EffectiveMaxFileUploadBytes(cfg.Ingestion.MaxUploadBytes))
			}
			return nil, err
		}
		metadata, err := parseMetadata(c.PostForm("metadata"))
		if err != nil {
			return nil, err
		}
		expiresAt, err := parseOptionalFileExpiry(c.PostForm("expires_at"))
		if err != nil {
			return nil, err
		}
		return &uploadInput{
			data:      data,
			filename:  firstNonEmpty(c.PostForm("filename"), header.Filename),
			mimeType:  firstNonEmpty(c.PostForm("mime_type"), mimeType),
			purpose:   c.PostForm("purpose"),
			metadata:  metadata,
			expiresAt: expiresAt,
		}, nil
	}

	var req fileURLUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON.")
	}
	req.URL = strings.TrimSpace(req.URL)
	if req.URL == "" {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_url", "url", "Field 'url' is required.")
	}
	client := middleware.NewSSRFClient(cfg.SSRF, cfgTimeout(c))
	resp, err := client.Get(c.Request.Context(), req.URL)
	if err != nil {
		return nil, ssrfError(err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_url", "url", "URL did not return a successful response.")
	}
	data, err := readBounded(resp.Body, config.EffectiveMaxFileUploadBytes(cfg.Ingestion.MaxUploadBytes))
	if err != nil {
		return nil, err
	}
	expiresAt, err := parseOptionalFileExpiry(req.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return &uploadInput{
		data:      data,
		filename:  req.Filename,
		mimeType:  firstNonEmpty(req.MimeType, resp.Header.Get("Content-Type")),
		purpose:   req.Purpose,
		metadata:  req.Metadata,
		originURL: req.URL,
		expiresAt: expiresAt,
	}, nil
}

func (h *FilesHandler) persistBytes(ctx context.Context, cfg config.FilesConfig, file *store.File, data []byte) error {
	inlineMax := config.EffectiveInlineFileBytes(cfg.Storage.InlineMaxBytes)
	if int64(len(data)) <= inlineMax {
		file.InlineBytes = append([]byte(nil), data...)
		return nil
	}
	blobStore, err := blobStoreFromConfig(cfg)
	if err != nil {
		return err
	}
	if blobStore == nil {
		if file.OriginURL != "" {
			return nil
		}
		return httputil.NewError(http.StatusRequestEntityTooLarge, "invalid_request_error", "file_too_large", "file", "File exceeds inline storage size and no blob store is configured.")
	}
	defer func() {
		_ = blobStore.Close()
	}()
	blobKey := blobKeyForFile(file.Sha256)
	exists, err := blobStore.Exists(ctx, blobKey)
	if err != nil {
		return err
	}
	if !exists {
		if err := blobStore.Put(ctx, blobKey, bytes.NewReader(data), int64(len(data)), file.MimeType); err != nil {
			return err
		}
	}
	file.BlobKey = blobKey
	return nil
}

func (h *FilesHandler) requireEnabled(c *gin.Context) (config.FilesConfig, bool) {
	if h.store == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "store_unavailable", "", "File store is unavailable."))
		return config.FilesConfig{}, false
	}
	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	if snapshot == nil || snapshot.Config == nil {
		httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "runtime_unavailable", "", "Runtime configuration is unavailable."))
		return config.FilesConfig{}, false
	}
	if !snapshot.Config.Files.Enabled {
		httputil.WriteError(c, httputil.NewError(http.StatusNotFound, "invalid_request_error", "files_disabled", "", "Files API is disabled."))
		return config.FilesConfig{}, false
	}
	return snapshot.Config.Files, true
}

func (h *FilesHandler) getAuthorizedFile(c *gin.Context) (*store.File, error) {
	auth := middleware.GetAuthContext(c)
	projectID, err := h.projectID(c, auth)
	if err != nil {
		return nil, err
	}
	file, err := h.store.GetFileForProject(c.Request.Context(), c.Param("id"), projectID)
	if err != nil {
		return nil, fileStoreError(err)
	}
	if expired(file) {
		return nil, httputil.NewError(http.StatusGone, "invalid_request_error", "file_expired", "id", "File has expired.")
	}
	return file, nil
}

func (h *FilesHandler) projectID(c *gin.Context, auth middleware.AuthContext) (string, error) {
	projectID := strings.TrimSpace(auth.ProjectID)
	if projectID == "" {
		projectID = strings.TrimSpace(auth.OwnerID)
	}
	if projectID == "" {
		projectID = "legacy-default"
	}
	if _, err := h.store.GetProject(c.Request.Context(), projectID); err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return "", err
		}
		project := store.Project{
			ID:          projectID,
			Name:        projectID,
			Description: "Compatibility project created for file storage.",
			CreatedAt:   time.Now().UTC(),
		}
		if createErr := h.store.CreateProject(c.Request.Context(), project); createErr != nil {
			if _, retryErr := h.store.GetProject(c.Request.Context(), projectID); retryErr != nil {
				return "", createErr
			}
		}
	}
	return projectID, nil
}

func (h *FilesHandler) enforceFileBudgets(c *gin.Context, file store.File) error {
	budgets, err := h.store.ListBudgets(c.Request.Context(), file.ProjectID)
	if err != nil {
		return err
	}
	if len(budgets) == 0 {
		return nil
	}

	var (
		currentBytes int64
		bytesLoaded  bool
		currentCount int64
		countLoaded  bool
	)
	for _, budget := range budgets {
		if budget.LimitFileBytes <= 0 && budget.LimitFileCount <= 0 {
			continue
		}
		exceeded := false
		metadata := map[string]any{
			"budget_id":        budget.ID,
			"budget_name":      budget.Name,
			"mode":             budget.Mode,
			"limit_file_bytes": budget.LimitFileBytes,
			"limit_file_count": budget.LimitFileCount,
		}
		if budget.LimitFileBytes > 0 {
			if !bytesLoaded {
				currentBytes, err = h.store.SumProjectFileBytes(c.Request.Context(), file.ProjectID)
				if err != nil {
					return err
				}
				bytesLoaded = true
			}
			nextBytes := currentBytes + file.Size
			metadata["current_file_bytes"] = currentBytes
			metadata["next_file_bytes"] = nextBytes
			if nextBytes > budget.LimitFileBytes {
				exceeded = true
			}
		}
		if budget.LimitFileCount > 0 {
			if !countLoaded {
				currentCount, err = h.store.CountProjectFiles(c.Request.Context(), file.ProjectID)
				if err != nil {
					return err
				}
				countLoaded = true
			}
			nextCount := currentCount + 1
			metadata["current_file_count"] = currentCount
			metadata["next_file_count"] = nextCount
			if nextCount > budget.LimitFileCount {
				exceeded = true
			}
		}
		if !exceeded {
			continue
		}
		if budget.Mode == store.BudgetModeHard {
			h.logAudit(c, "file.quota_denied", file, metadata)
			return httputil.NewError(http.StatusTooManyRequests, "budget_exceeded", "budget_exceeded", "", "Project file storage budget has been exceeded.")
		}
		h.logAudit(c, "file.quota_warning", file, metadata)
	}
	return nil
}

func (h *FilesHandler) fileResponse(c *gin.Context, cfg config.FilesConfig, file store.File) fileObjectResponse {
	var expiresAt *int64
	if file.ExpiresAt != nil {
		value := file.ExpiresAt.Unix()
		expiresAt = &value
	}
	response := fileObjectResponse{
		ID:        file.PolarisID,
		Object:    "file",
		Bytes:     file.Size,
		CreatedAt: file.CreatedAt.Unix(),
		Filename:  file.OriginalFilename,
		Purpose:   file.Purpose,
		Polaris: filePolarisMetadata{
			Sha256:                file.Sha256,
			MimeType:              file.MimeType,
			ProjectID:             file.ProjectID,
			ExpiresAt:             expiresAt,
			MaterializedProviders: []string{},
			HasInlineBytes:        len(file.InlineBytes) > 0,
			HasBlobBacking:        file.BlobKey != "",
			OriginURL:             file.OriginURL,
			Metadata:              file.Metadata,
		},
	}
	if token, err := signFileDownloadToken(file.PolarisID, file.ProjectID, file.KeyID, time.Now().Add(config.EffectiveFileDownloadTTL(cfg.Downloads.TokenTTL)).Unix()); err == nil {
		response.Polaris.ContentURL = fileContentURL(c, file.PolarisID, token)
	}
	return response
}

func (h *FilesHandler) readFileBytes(c *gin.Context, cfg config.FilesConfig, file *store.File) ([]byte, error) {
	if len(file.InlineBytes) > 0 {
		return append([]byte(nil), file.InlineBytes...), nil
	}
	if file.BlobKey != "" {
		blobStore, err := blobStoreFromConfig(cfg)
		if err != nil {
			return nil, err
		}
		if blobStore == nil {
			return nil, httputil.NewError(http.StatusNotFound, "invalid_request_error", "file_not_found", "id", "File blob backing is not configured.")
		}
		defer func() {
			_ = blobStore.Close()
		}()
		body, _, err := blobStore.Get(c.Request.Context(), file.BlobKey)
		if err != nil {
			return nil, httputil.NewError(http.StatusNotFound, "invalid_request_error", "file_not_found", "id", "File content was not found.")
		}
		defer func() {
			_ = body.Close()
		}()
		return readBounded(body, config.EffectiveMaxFileUploadBytes(cfg.Ingestion.MaxUploadBytes))
	}
	if file.OriginURL != "" {
		resp, err := middleware.NewSSRFClient(cfg.SSRF, cfgTimeout(c)).Get(c.Request.Context(), file.OriginURL)
		if err != nil {
			return nil, ssrfError(err)
		}
		defer func() {
			_ = resp.Body.Close()
		}()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "origin_fetch_failed", "url", "Origin URL did not return a successful response.")
		}
		return readBounded(resp.Body, config.EffectiveMaxFileUploadBytes(cfg.Ingestion.MaxUploadBytes))
	}
	return nil, httputil.NewError(http.StatusNotFound, "invalid_request_error", "file_not_found", "id", "File content was not found.")
}

func (h *FilesHandler) blobStore(c *gin.Context) (blob.BlobStore, error) {
	cfg, ok := h.requireEnabled(c)
	if !ok {
		return nil, httputil.NewError(http.StatusNotFound, "invalid_request_error", "files_disabled", "", "Files API is disabled.")
	}
	return blobStoreFromConfig(cfg)
}

func fileHandleResponse(handle *store.FileProviderHandle, cached bool) fileMaterializationResponse {
	var expiresAt *int64
	if handle.ExpiresAt != nil {
		value := handle.ExpiresAt.Unix()
		expiresAt = &value
	}
	return fileMaterializationResponse{
		Object:         "file.materialization",
		FileID:         handle.PolarisID,
		Provider:       handle.Provider,
		ProviderFileID: handle.ProviderFileID,
		Purpose:        handle.Purpose,
		Bytes:          handle.SizeBytes,
		MimeType:       handle.MimeType,
		CreatedAt:      handle.CreatedAt.Unix(),
		ExpiresAt:      expiresAt,
		Cached:         cached,
	}
}

func providerHandleExpired(handle *store.FileProviderHandle) bool {
	return handle != nil && handle.ExpiresAt != nil && time.Now().After(*handle.ExpiresAt)
}

func (h *FilesHandler) logAudit(c *gin.Context, kind string, file store.File, extra map[string]any) {
	metadata := map[string]any{
		"polaris_id": file.PolarisID,
		"size":       file.Size,
		"mime_type":  file.MimeType,
	}
	for key, value := range extra {
		metadata[key] = value
	}
	raw, _ := json.Marshal(metadata)
	auth := middleware.GetAuthContext(c)
	event := store.AuditEvent{
		ProjectID:    file.ProjectID,
		ActorKeyID:   auth.KeyID,
		Kind:         kind,
		ResourceType: "file",
		ResourceID:   file.PolarisID,
		MetadataJSON: string(raw),
		CreatedAt:    time.Now().UTC(),
	}
	if h.auditLogger != nil {
		if h.auditLogger.Log(event) {
			return
		}
	}
	if err := h.store.LogAuditEvent(c.Request.Context(), event); err != nil && h.logger != nil {
		h.logger.Warn("file audit event failed", "kind", kind, "file_id", file.PolarisID, "error", err)
	}
}

func blobStoreFromConfig(cfg config.FilesConfig) (blob.BlobStore, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Storage.BlobStore)) {
	case "", "none":
		return nil, nil
	case "disk":
		return blob.NewDiskStore(cfg.Storage.DiskPath)
	case "s3":
		return blob.NewS3Store(blob.S3Options{
			Endpoint:        cfg.Storage.S3.Endpoint,
			Bucket:          cfg.Storage.S3.Bucket,
			Region:          cfg.Storage.S3.Region,
			AccessKeyID:     cfg.Storage.S3.AccessKeyID,
			SecretAccessKey: cfg.Storage.S3.SecretAccessKey,
			SessionToken:    cfg.Storage.S3.SessionToken,
			UseSSL:          cfg.Storage.S3.UseSSL,
		})
	default:
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_blob_store", "files.storage.blob_store", "Configured file blob store is invalid.")
	}
}

func validateUploadMIME(declared string, filename string, data []byte, allowed []string) (string, error) {
	detected := http.DetectContentType(data)
	declared = canonicalMIME(declared)
	detected = canonicalMIME(detected)
	mimeType := detected
	if declared != "" && declared != "application/octet-stream" {
		mimeType = declared
	}
	if declared != "" && detected != "" &&
		declared != "application/octet-stream" &&
		detected != "application/octet-stream" &&
		declared != detected &&
		!compatibleDetectedMIME(declared, detected, filename, data) {
		return "", httputil.NewError(http.StatusUnsupportedMediaType, "invalid_request_error", "unsupported_mime", "mime_type", "Declared content type does not match detected file content.")
	}
	if !mimeAllowed(mimeType, allowed) {
		return "", httputil.NewError(http.StatusUnsupportedMediaType, "invalid_request_error", "unsupported_mime", "mime_type", "File MIME type is not allowed.")
	}
	return mimeType, nil
}

func compatibleDetectedMIME(declared, detected string, filename string, data []byte) bool {
	if detected == "text/plain" {
		switch declared {
		case "application/json", "application/xml", "application/x-yaml", "application/yaml", "text/markdown", "text/csv":
			return true
		}
	}
	if declared == "image/svg+xml" && (detected == "text/xml" || detected == "text/plain") {
		return looksLikeSVG(data)
	}
	if (detected == "application/zip" || detected == "application/x-zip-compressed") && ooxmlMIMEMatchesExtension(declared, filename) {
		return looksLikeOOXML(data, declared)
	}
	return false
}

func canonicalMIME(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	mediaType, _, err := mime.ParseMediaType(value)
	if err == nil {
		value = mediaType
	}
	return strings.ToLower(strings.TrimSpace(value))
}

func mimeAllowed(mimeType string, allowed []string) bool {
	if len(allowed) == 0 {
		allowed = []string{
			"application/pdf",
			"application/json",
			"application/xml",
			"application/x-yaml",
			"application/yaml",
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			"application/vnd.openxmlformats-officedocument.presentationml.presentation",
			"text/*",
			"image/*",
			"audio/*",
		}
	}
	for _, candidate := range allowed {
		candidate = strings.ToLower(strings.TrimSpace(candidate))
		switch {
		case candidate == "*/*" || candidate == mimeType:
			return true
		case strings.HasSuffix(candidate, "/*"):
			prefix := strings.TrimSuffix(candidate, "*")
			if strings.HasPrefix(mimeType, prefix) {
				return true
			}
		}
	}
	return false
}

func ooxmlMIMEMatchesExtension(mimeType string, filename string) bool {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(filename)))
	switch mimeType {
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return ext == ".docx"
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return ext == ".xlsx"
	case "application/vnd.openxmlformats-officedocument.presentationml.presentation":
		return ext == ".pptx"
	default:
		return false
	}
}

func looksLikeOOXML(data []byte, mimeType string) bool {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false
	}
	required := ""
	switch mimeType {
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		required = "word/document.xml"
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		required = "xl/workbook.xml"
	case "application/vnd.openxmlformats-officedocument.presentationml.presentation":
		required = "ppt/presentation.xml"
	default:
		return false
	}
	hasContentTypes := false
	hasRequired := false
	for _, file := range reader.File {
		name := strings.TrimPrefix(file.Name, "/")
		if name == "[Content_Types].xml" {
			hasContentTypes = true
		}
		if name == required {
			hasRequired = true
		}
		if hasContentTypes && hasRequired {
			return true
		}
	}
	return false
}

func looksLikeSVG(data []byte) bool {
	prefix := strings.ToLower(string(bytes.TrimSpace(data)))
	if strings.HasPrefix(prefix, "<?xml") {
		if idx := strings.Index(prefix, ">"); idx >= 0 {
			prefix = strings.TrimSpace(prefix[idx+1:])
		}
	}
	return strings.HasPrefix(prefix, "<svg") || strings.Contains(prefix[:min(len(prefix), 512)], "<svg")
}

func normalizeFilePurpose(raw string) (modality.FilePurpose, error) {
	if strings.TrimSpace(raw) == "" {
		return modality.FilePurposeUserData, nil
	}
	purpose := modality.FilePurpose(strings.TrimSpace(raw))
	if !purpose.Valid() {
		return "", httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_purpose", "purpose", "File purpose is invalid.")
	}
	return purpose, nil
}

func parseMetadata(raw string) (map[string]string, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]string{}, nil
	}
	metadata := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_metadata", "metadata", "Metadata must be a JSON object with string values.")
	}
	return metadata, nil
}

func parseOptionalFileExpiry(raw string) (*time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(raw))
	if err != nil {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_expires_at", "expires_at", "Field 'expires_at' must be RFC3339.")
	}
	return &parsed, nil
}

func readBounded(r io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = config.DefaultMaxFileUploadBytes
	}
	var buf bytes.Buffer
	n, err := io.Copy(&buf, io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if n > maxBytes {
		return nil, httputil.NewError(http.StatusRequestEntityTooLarge, "invalid_request_error", "file_too_large", "file", "File exceeds the configured maximum upload size.")
	}
	return buf.Bytes(), nil
}

func newPolarisFileID() (string, error) {
	id, err := ulid.New(ulid.Timestamp(time.Now().UTC()), rand.Reader)
	if err != nil {
		return "", err
	}
	return "pl_file_" + id.String(), nil
}

func blobKeyForFile(sha256Hex string) string {
	sha256Hex = strings.ToLower(strings.TrimSpace(sha256Hex))
	if len(sha256Hex) >= 2 {
		return "sha256/" + sha256Hex[:2] + "/" + sha256Hex
	}
	return "sha256/unknown/" + strings.Trim(sha256Hex, "/")
}

func expired(file *store.File) bool {
	return file != nil && file.ExpiresAt != nil && time.Now().UTC().After(file.ExpiresAt.UTC())
}

func cfgTimeout(c *gin.Context) time.Duration {
	snapshot, ok := middleware.GetRuntimeSnapshot(c)
	if !ok || snapshot == nil || snapshot.Config == nil || snapshot.Config.Server.ReadTimeout <= 0 {
		return 30 * time.Second
	}
	return snapshot.Config.Server.ReadTimeout
}

func ssrfError(err error) error {
	if err == nil {
		return nil
	}
	return httputil.NewError(http.StatusUnprocessableEntity, "invalid_request_error", "ssrf_blocked", "url", "URL failed SSRF validation.")
}

func fileStoreError(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return httputil.NewError(http.StatusNotFound, "invalid_request_error", "file_not_found", "id", "File was not found.")
	}
	return err
}

func writeFileStoreError(c *gin.Context, err error) {
	httputil.WriteError(c, fileStoreError(err))
}

func fileContentURL(c *gin.Context, polarisID string, token string) string {
	scheme := "http"
	if c.Request != nil && c.Request.TLS != nil {
		scheme = "https"
	}
	if forwarded := c.GetHeader("X-Forwarded-Proto"); forwarded != "" {
		scheme = strings.Split(forwarded, ",")[0]
	}
	host := ""
	if c.Request != nil {
		host = c.Request.Host
	}
	return fmt.Sprintf("%s://%s/v1/files/%s/content?token=%s", strings.TrimSpace(scheme), host, polarisID, token)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
