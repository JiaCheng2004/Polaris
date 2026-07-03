package anthropic

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/apierror"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

type BatchAdapter struct {
	client   *Client
	provider string
}

func NewBatchAdapter(client *Client, provider string) *BatchAdapter {
	return &BatchAdapter{client: client, provider: provider}
}

func (a *BatchAdapter) Create(ctx context.Context, req *modality.BatchRequest) (*modality.BatchJob, error) {
	body := map[string]any{
		"input_file_id": strings.TrimSpace(req.InputFileID),
		"endpoint":      firstNonEmpty(req.Endpoint, "/v1/messages"),
		"metadata":      req.Metadata,
	}
	var response anthropicBatch
	if _, err := a.client.JSON(ctx, "/v1/messages/batches", body, &response); err != nil {
		return nil, err
	}
	return response.job(req.Model), nil
}

func (a *BatchAdapter) Get(ctx context.Context, providerJobID string) (*modality.BatchStatus, error) {
	// B1: batch retrieval is a GET; the shared compat JSON always POSTs, so issue
	// the request through the transport core with an explicit GET and no body.
	var response anthropicBatch
	if err := a.client.Core().JSON(ctx, http.MethodGet, "/v1/messages/batches/"+strings.TrimSpace(providerJobID), nil, &response); err != nil {
		return nil, err
	}
	status := modality.BatchStatus{BatchJob: *response.job("")}
	return &status, nil
}

func (a *BatchAdapter) Cancel(ctx context.Context, providerJobID string) error {
	var response anthropicBatch
	_, err := a.client.JSON(ctx, "/v1/messages/batches/"+strings.TrimSpace(providerJobID)+"/cancel", map[string]any{}, &response)
	return err
}

func (a *BatchAdapter) Output(ctx context.Context, providerJobID string) (io.ReadCloser, error) {
	return nil, apierror.NewError(http.StatusBadRequest, "capability_not_supported", "batch_output_proxy_unavailable", "id", "Anthropic batch output proxy is not implemented in this runtime build.")
}

type anthropicBatch struct {
	ID               string            `json:"id"`
	Type             string            `json:"type"`
	ProcessingStatus string            `json:"processing_status"`
	Status           string            `json:"status"`
	CreatedAt        string            `json:"created_at"`
	EndedAt          string            `json:"ended_at"`
	Metadata         map[string]string `json:"metadata"`
}

func (b anthropicBatch) job(model string) *modality.BatchJob {
	createdAt := time.Now().Unix()
	if parsed, err := time.Parse(time.RFC3339, b.CreatedAt); err == nil {
		createdAt = parsed.Unix()
	}
	completedAt := int64(0)
	if parsed, err := time.Parse(time.RFC3339, b.EndedAt); err == nil {
		completedAt = parsed.Unix()
	}
	return &modality.BatchJob{
		ID:            b.ID,
		Object:        firstNonEmpty(b.Type, "batch"),
		Model:         model,
		Status:        firstNonEmpty(b.ProcessingStatus, b.Status),
		CreatedAt:     createdAt,
		CompletedAt:   completedAt,
		ProviderJobID: b.ID,
		Metadata:      b.Metadata,
	}
}
