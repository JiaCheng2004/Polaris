package google

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
		"inputFile": strings.TrimSpace(req.InputFileID),
		"endpoint":  firstNonEmpty(req.Endpoint, "generateContent"),
		"metadata":  req.Metadata,
	}
	var response googleBatch
	if err := a.client.JSON(ctx, "/v1beta/batches", body, &response); err != nil {
		return nil, err
	}
	return response.job(req.Model), nil
}

func (a *BatchAdapter) Get(ctx context.Context, providerJobID string) (*modality.BatchStatus, error) {
	var response googleBatch
	if err := a.client.JSON(ctx, "/v1beta/batches/"+strings.TrimSpace(providerJobID), map[string]any{}, &response); err != nil {
		return nil, err
	}
	status := modality.BatchStatus{BatchJob: *response.job("")}
	return &status, nil
}

func (a *BatchAdapter) Cancel(ctx context.Context, providerJobID string) error {
	var response googleBatch
	return a.client.JSON(ctx, "/v1beta/batches/"+strings.TrimSpace(providerJobID)+":cancel", map[string]any{}, &response)
}

func (a *BatchAdapter) Output(ctx context.Context, providerJobID string) (io.ReadCloser, error) {
	return nil, apierror.NewError(http.StatusBadRequest, "capability_not_supported", "batch_output_proxy_unavailable", "id", "Google batch output proxy is not implemented in this runtime build.")
}

type googleBatch struct {
	Name        string            `json:"name"`
	DisplayName string            `json:"displayName"`
	State       string            `json:"state"`
	CreateTime  string            `json:"createTime"`
	EndTime     string            `json:"endTime"`
	Metadata    map[string]string `json:"metadata"`
}

func (b googleBatch) job(model string) *modality.BatchJob {
	createdAt := time.Now().Unix()
	if parsed, err := time.Parse(time.RFC3339, b.CreateTime); err == nil {
		createdAt = parsed.Unix()
	}
	completedAt := int64(0)
	if parsed, err := time.Parse(time.RFC3339, b.EndTime); err == nil {
		completedAt = parsed.Unix()
	}
	return &modality.BatchJob{
		ID:            b.Name,
		Object:        "batch",
		Model:         model,
		Status:        strings.ToLower(strings.TrimPrefix(b.State, "JOB_STATE_")),
		CreatedAt:     createdAt,
		CompletedAt:   completedAt,
		ProviderJobID: b.Name,
		Metadata:      b.Metadata,
	}
}
