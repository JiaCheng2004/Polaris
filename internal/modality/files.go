package modality

import (
	"context"
	"io"
	"time"
)

type FilePurpose string

const (
	FilePurposeUserData    FilePurpose = "user_data"
	FilePurposeVision      FilePurpose = "vision"
	FilePurposeAssistants  FilePurpose = "assistants"
	FilePurposeBatch       FilePurpose = "batch"
	FilePurposeFileExtract FilePurpose = "file_extract"
)

func (p FilePurpose) Valid() bool {
	switch p {
	case FilePurposeUserData, FilePurposeVision, FilePurposeAssistants, FilePurposeBatch, FilePurposeFileExtract:
		return true
	default:
		return false
	}
}

type FileSourceKind string

const (
	FileSourceInline      FileSourceKind = "inline"
	FileSourceURL         FileSourceKind = "url"
	FileSourcePolarisRef  FileSourceKind = "polaris_ref"
	FileSourceProviderRef FileSourceKind = "provider_ref"
)

type FileSource struct {
	Kind        FileSourceKind
	PolarisID   string
	ProviderID  string
	URL         string
	InlineBytes []byte
	InlineB64   string
	MimeType    string
	Filename    string
}

type FileMetadata struct {
	PolarisID        string
	ProjectID        string
	KeyID            string
	Sha256           string
	MimeType         string
	Size             int64
	OriginalFilename string
	Purpose          FilePurpose
	OriginURL        string
	HasInlineBytes   bool
	HasBlobBacking   bool
	Metadata         map[string]string
	CreatedAt        time.Time
	ExpiresAt        *time.Time
}

type ProviderFileHandle struct {
	Provider       string
	ProviderFileID string
	Purpose        FilePurpose
	SizeBytes      int64
	MimeType       string
	ExpiresAt      *time.Time
	CreatedAt      time.Time
}

type FileUploadRequest struct {
	Body     io.Reader
	Size     int64
	Filename string
	MimeType string
	Purpose  FilePurpose
	Metadata map[string]string
}

type FileMaterializeRequest struct {
	PolarisID string
	MimeType  string
	Filename  string
	Purpose   FilePurpose
	SourceURL string
	InlineSrc []byte
	Sha256    string
	Size      int64
}

type FilesAdapter interface {
	Upload(ctx context.Context, req *FileUploadRequest) (*ProviderFileHandle, error)
	Get(ctx context.Context, providerFileID string) (*ProviderFileHandle, error)
	Delete(ctx context.Context, providerFileID string) error
	Materialize(ctx context.Context, req *FileMaterializeRequest) (*ProviderFileHandle, error)
}
