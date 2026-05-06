package files

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/JiaCheng2004/Polaris/internal/store/blob"
)

type AdapterRegistry interface {
	GetFilesAdapter(providerName string) (modality.FilesAdapter, error)
}

type ResolvedKind string

const (
	ResolvedProviderRef ResolvedKind = "provider_ref"
	ResolvedInline      ResolvedKind = "inline"
	ResolvedURL         ResolvedKind = "url"
)

type ResolvedFile struct {
	Kind       ResolvedKind
	ProviderID string
	URL        string
	Bytes      []byte
	B64        string
	MimeType   string
	Filename   string
	Purpose    modality.FilePurpose
	PolarisID  string
	Citations  *bool
}

type Resolver struct {
	Store    store.Store
	Registry AdapterRegistry
	Config   config.FilesConfig
}

var materializeLocks sync.Map

func (r Resolver) Resolve(ctx context.Context, source modality.FileSource, projectID string, providerName string) (*ResolvedFile, error) {
	switch source.Kind {
	case modality.FileSourceInline:
		return r.resolveInlineSource(source), nil
	case modality.FileSourceURL:
		return &ResolvedFile{Kind: ResolvedURL, URL: strings.TrimSpace(source.URL), MimeType: source.MimeType, Filename: source.Filename}, nil
	case modality.FileSourceProviderRef:
		return &ResolvedFile{Kind: ResolvedProviderRef, ProviderID: strings.TrimSpace(source.ProviderID), MimeType: source.MimeType, Filename: source.Filename}, nil
	case modality.FileSourcePolarisRef:
		return r.resolvePolarisRef(ctx, source, projectID, providerName)
	default:
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_file_source", "messages.content.file", "Unsupported file source.")
	}
}

func (r Resolver) resolveInlineSource(source modality.FileSource) *ResolvedFile {
	data := append([]byte(nil), source.InlineBytes...)
	b64 := strings.TrimSpace(source.InlineB64)
	if len(data) == 0 && b64 != "" {
		if decoded, err := base64.StdEncoding.DecodeString(b64); err == nil {
			data = decoded
		}
	}
	if b64 == "" && len(data) > 0 {
		b64 = base64.StdEncoding.EncodeToString(data)
	}
	return &ResolvedFile{
		Kind:     ResolvedInline,
		Bytes:    data,
		B64:      b64,
		MimeType: source.MimeType,
		Filename: source.Filename,
	}
}

func (r Resolver) resolvePolarisRef(ctx context.Context, source modality.FileSource, projectID string, providerName string) (*ResolvedFile, error) {
	if r.Store == nil {
		return nil, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "store_unavailable", "file_id", "File store is unavailable.")
	}
	file, err := r.Store.GetFileForProject(ctx, source.PolarisID, projectID)
	if err != nil {
		return nil, fileStoreError(err)
	}
	if file.ExpiresAt != nil && time.Now().After(*file.ExpiresAt) {
		return nil, httputil.NewError(http.StatusGone, "invalid_request_error", "file_expired", "file_id", "File has expired.")
	}

	if preferVisionTransport(providerName, file.MimeType) {
		if file.OriginURL != "" && acceptsURL(file.MimeType) {
			return &ResolvedFile{
				Kind:      ResolvedURL,
				URL:       file.OriginURL,
				MimeType:  file.MimeType,
				Filename:  file.OriginalFilename,
				Purpose:   file.Purpose,
				PolarisID: file.PolarisID,
			}, nil
		}
		inlineMax := config.EffectiveInlineFileBytes(r.Config.Materialize.InlineFallbackMax)
		if inlineMax > 0 && file.Size <= inlineMax {
			data, err := r.ReadFileBytes(ctx, file)
			if err != nil {
				return nil, err
			}
			return &ResolvedFile{
				Kind:      ResolvedInline,
				Bytes:     data,
				B64:       base64.StdEncoding.EncodeToString(data),
				MimeType:  file.MimeType,
				Filename:  file.OriginalFilename,
				Purpose:   file.Purpose,
				PolarisID: file.PolarisID,
			}, nil
		}
		return nil, httputil.NewError(http.StatusBadRequest, "capability_not_supported", "provider_lacks_files_api", "file_id", "Selected provider requires URL or inline bytes for image file references.")
	}

	if handle, ok, err := r.Store.GetFileProviderHandle(ctx, file.PolarisID, providerName); err != nil {
		return nil, err
	} else if ok && !providerHandleExpired(handle) {
		return fileHandleResolution(file, handle), nil
	}

	if r.Registry != nil {
		if adapter, err := r.Registry.GetFilesAdapter(providerName); err == nil && adapter != nil {
			return r.materialize(ctx, adapter, providerName, file)
		}
	}

	if file.OriginURL != "" && acceptsURL(file.MimeType) {
		return &ResolvedFile{
			Kind:      ResolvedURL,
			URL:       file.OriginURL,
			MimeType:  file.MimeType,
			Filename:  file.OriginalFilename,
			Purpose:   file.Purpose,
			PolarisID: file.PolarisID,
		}, nil
	}

	inlineMax := config.EffectiveInlineFileBytes(r.Config.Materialize.InlineFallbackMax)
	if inlineMax > 0 && file.Size <= inlineMax {
		data, err := r.ReadFileBytes(ctx, file)
		if err != nil {
			return nil, err
		}
		return &ResolvedFile{
			Kind:      ResolvedInline,
			Bytes:     data,
			B64:       base64.StdEncoding.EncodeToString(data),
			MimeType:  file.MimeType,
			Filename:  file.OriginalFilename,
			Purpose:   file.Purpose,
			PolarisID: file.PolarisID,
		}, nil
	}

	return nil, httputil.NewError(http.StatusBadRequest, "capability_not_supported", "provider_lacks_files_api", "file_id", "Selected provider cannot accept this file reference.")
}

func (r Resolver) materialize(ctx context.Context, adapter modality.FilesAdapter, providerName string, file *store.File) (*ResolvedFile, error) {
	lockKey := file.PolarisID + "\x00" + providerName
	rawLock, _ := materializeLocks.LoadOrStore(lockKey, &sync.Mutex{})
	lock := rawLock.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	if handle, ok, err := r.Store.GetFileProviderHandle(ctx, file.PolarisID, providerName); err != nil {
		return nil, err
	} else if ok && !providerHandleExpired(handle) {
		return fileHandleResolution(file, handle), nil
	}

	data, err := r.ReadFileBytes(ctx, file)
	if err != nil {
		return nil, err
	}
	handle, err := adapter.Materialize(ctx, &modality.FileMaterializeRequest{
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
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "materialization_failed", "file_id", "Provider file materialization failed.")
	}
	record := store.FileProviderHandle{
		PolarisID:      file.PolarisID,
		Provider:       providerName,
		ProviderFileID: handle.ProviderFileID,
		Purpose:        firstPurpose(handle.Purpose, file.Purpose),
		SizeBytes:      firstPositive(handle.SizeBytes, file.Size),
		MimeType:       firstNonEmpty(handle.MimeType, file.MimeType),
		ExpiresAt:      handle.ExpiresAt,
		CreatedAt:      handle.CreatedAt,
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	if err := r.Store.PutFileProviderHandle(ctx, record); err != nil {
		return nil, err
	}
	return fileHandleResolution(file, &record), nil
}

func (r Resolver) ReadFileBytes(ctx context.Context, file *store.File) ([]byte, error) {
	if len(file.InlineBytes) > 0 {
		return append([]byte(nil), file.InlineBytes...), nil
	}
	if file.BlobKey != "" {
		blobStore, err := BlobStoreFromConfig(r.Config)
		if err != nil {
			return nil, err
		}
		if blobStore == nil {
			return nil, httputil.NewError(http.StatusNotFound, "invalid_request_error", "file_not_found", "file_id", "File blob backing is not configured.")
		}
		defer func() {
			_ = blobStore.Close()
		}()
		body, _, err := blobStore.Get(ctx, file.BlobKey)
		if err != nil {
			return nil, httputil.NewError(http.StatusNotFound, "invalid_request_error", "file_not_found", "file_id", "File content was not found.")
		}
		defer func() {
			_ = body.Close()
		}()
		return io.ReadAll(io.LimitReader(body, config.EffectiveMaxFileUploadBytes(r.Config.Ingestion.MaxUploadBytes)+1))
	}
	return nil, httputil.NewError(http.StatusNotFound, "invalid_request_error", "file_not_found", "file_id", "File content was not found.")
}

func BlobStoreFromConfig(cfg config.FilesConfig) (blob.BlobStore, error) {
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
		return nil, fmt.Errorf("invalid file blob store %q", cfg.Storage.BlobStore)
	}
}

func SourceFromFilePart(part *modality.FilePart) modality.FileSource {
	if part == nil {
		return modality.FileSource{}
	}
	switch {
	case strings.HasPrefix(strings.TrimSpace(part.FileID), "pl_file_"):
		return modality.FileSource{
			Kind:      modality.FileSourcePolarisRef,
			PolarisID: strings.TrimSpace(part.FileID),
			MimeType:  part.MimeType,
			Filename:  part.Filename,
		}
	case strings.TrimSpace(part.FileID) != "":
		return modality.FileSource{
			Kind:       modality.FileSourceProviderRef,
			ProviderID: strings.TrimSpace(part.FileID),
			MimeType:   part.MimeType,
			Filename:   part.Filename,
		}
	case strings.TrimSpace(part.URL) != "":
		return modality.FileSource{
			Kind:     modality.FileSourceURL,
			URL:      strings.TrimSpace(part.URL),
			MimeType: part.MimeType,
			Filename: part.Filename,
		}
	case strings.TrimSpace(part.Data) != "":
		return modality.FileSource{
			Kind:      modality.FileSourceInline,
			InlineB64: strings.TrimSpace(part.Data),
			MimeType:  part.MimeType,
			Filename:  part.Filename,
		}
	default:
		return modality.FileSource{}
	}
}

func DataURI(mimeType string, b64 string) string {
	if strings.TrimSpace(mimeType) == "" {
		mimeType = "application/octet-stream"
	}
	return "data:" + mimeType + ";base64," + b64
}

func fileHandleResolution(file *store.File, handle *store.FileProviderHandle) *ResolvedFile {
	return &ResolvedFile{
		Kind:       ResolvedProviderRef,
		ProviderID: handle.ProviderFileID,
		MimeType:   firstNonEmpty(handle.MimeType, file.MimeType),
		Filename:   file.OriginalFilename,
		Purpose:    firstPurpose(handle.Purpose, file.Purpose),
		PolarisID:  file.PolarisID,
	}
}

func fileStoreError(err error) error {
	if err == nil {
		return nil
	}
	if err == store.ErrNotFound {
		return httputil.NewError(http.StatusNotFound, "invalid_request_error", "file_not_found", "file_id", "File was not found.")
	}
	return err
}

func providerHandleExpired(handle *store.FileProviderHandle) bool {
	return handle != nil && handle.ExpiresAt != nil && time.Now().After(*handle.ExpiresAt)
}

func acceptsURL(mimeType string) bool {
	return strings.HasPrefix(mimeType, "image/") || strings.HasPrefix(mimeType, "audio/") || mimeType == "application/pdf"
}

func preferVisionTransport(providerName string, mimeType string) bool {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(mimeType)), "image/") {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(providerName)) {
	case "openai", "qwen":
		return true
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstPurpose(values ...modality.FilePurpose) modality.FilePurpose {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return modality.FilePurposeUserData
}

func firstPositive(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func BytesSource(data []byte, mimeType string, filename string) modality.FileSource {
	return modality.FileSource{
		Kind:        modality.FileSourceInline,
		InlineBytes: append([]byte(nil), data...),
		InlineB64:   base64.StdEncoding.EncodeToString(data),
		MimeType:    mimeType,
		Filename:    filename,
	}
}

func ReaderUpload(data []byte) io.Reader {
	return bytes.NewReader(data)
}
