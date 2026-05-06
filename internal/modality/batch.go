package modality

import (
	"context"
	"io"
	"time"
)

type BatchRequest struct {
	Model            string            `json:"model"`
	Routing          *RoutingOptions   `json:"routing,omitempty"`
	InputFileID      string            `json:"input_file_id"`
	Endpoint         string            `json:"endpoint"`
	CompletionWindow string            `json:"completion_window,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}

type BatchJob struct {
	ID            string            `json:"id"`
	Object        string            `json:"object,omitempty"`
	Model         string            `json:"model,omitempty"`
	Endpoint      string            `json:"endpoint,omitempty"`
	Status        string            `json:"status"`
	InputFileID   string            `json:"input_file_id,omitempty"`
	OutputFileID  string            `json:"output_file_id,omitempty"`
	ErrorFileID   string            `json:"error_file_id,omitempty"`
	CreatedAt     int64             `json:"created_at,omitempty"`
	ExpiresAt     int64             `json:"expires_at,omitempty"`
	CompletedAt   int64             `json:"completed_at,omitempty"`
	FailedAt      int64             `json:"failed_at,omitempty"`
	ProviderJobID string            `json:"provider_job_id,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

type BatchStatus struct {
	BatchJob
	RequestCounts *BatchRequestCounts `json:"request_counts,omitempty"`
	Error         *BatchError         `json:"error,omitempty"`
}

type BatchRequestCounts struct {
	Total     int `json:"total"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
}

type BatchError struct {
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

type BatchAdapter interface {
	Create(ctx context.Context, req *BatchRequest) (*BatchJob, error)
	Get(ctx context.Context, providerJobID string) (*BatchStatus, error)
	Cancel(ctx context.Context, providerJobID string) error
	Output(ctx context.Context, providerJobID string) (io.ReadCloser, error)
}

func (j BatchJob) Expired(now time.Time) bool {
	return j.ExpiresAt > 0 && now.Unix() >= j.ExpiresAt
}
