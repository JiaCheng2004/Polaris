package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
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
		"input_file_id":     req.InputFileID,
		"endpoint":          firstNonEmpty(req.Endpoint, "/v1/chat/completions"),
		"completion_window": firstNonEmpty(req.CompletionWindow, "24h"),
	}
	if len(req.Metadata) > 0 {
		body["metadata"] = req.Metadata
	}
	var response openAIBatch
	if err := a.jsonRequest(ctx, http.MethodPost, "/batches", body, &response); err != nil {
		return nil, err
	}
	return response.job(req.Model), nil
}

func (a *BatchAdapter) Get(ctx context.Context, providerJobID string) (*modality.BatchStatus, error) {
	var response openAIBatch
	if err := a.jsonRequest(ctx, http.MethodGet, "/batches/"+strings.TrimSpace(providerJobID), nil, &response); err != nil {
		return nil, err
	}
	status := modality.BatchStatus{BatchJob: *response.job("")}
	status.RequestCounts = response.RequestCounts
	if response.Errors != nil {
		status.Error = response.Errors
	}
	return &status, nil
}

func (a *BatchAdapter) Cancel(ctx context.Context, providerJobID string) error {
	var response openAIBatch
	return a.jsonRequest(ctx, http.MethodPost, "/batches/"+strings.TrimSpace(providerJobID)+"/cancel", nil, &response)
}

func (a *BatchAdapter) Output(ctx context.Context, providerJobID string) (io.ReadCloser, error) {
	status, err := a.Get(ctx, providerJobID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(status.OutputFileID) == "" {
		return nil, httputil.NewError(http.StatusNotFound, "invalid_request_error", "batch_output_not_ready", "id", "Batch output is not ready.")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.client.baseURL+"/files/"+status.OutputFileID+"/content", nil)
	if err != nil {
		return nil, fmt.Errorf("build openai batch output request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+a.client.apiKey)
	resp, err := a.client.httpClient.Do(req)
	if err != nil {
		return nil, translateTransportError(err, "OpenAI")
	}
	if resp.StatusCode >= http.StatusBadRequest {
		defer func() {
			_ = resp.Body.Close()
		}()
		return nil, a.client.apiError(resp)
	}
	return resp.Body, nil
}

type openAIBatch struct {
	ID               string                       `json:"id"`
	Object           string                       `json:"object"`
	Endpoint         string                       `json:"endpoint"`
	Status           string                       `json:"status"`
	InputFileID      string                       `json:"input_file_id"`
	OutputFileID     string                       `json:"output_file_id"`
	ErrorFileID      string                       `json:"error_file_id"`
	CreatedAt        int64                        `json:"created_at"`
	ExpiresAt        int64                        `json:"expires_at"`
	CompletedAt      int64                        `json:"completed_at"`
	FailedAt         int64                        `json:"failed_at"`
	RequestCounts    *modality.BatchRequestCounts `json:"request_counts"`
	Errors           *modality.BatchError         `json:"error"`
	Metadata         map[string]string            `json:"metadata"`
	CompletionWindow string                       `json:"completion_window"`
}

func (b openAIBatch) job(model string) *modality.BatchJob {
	return &modality.BatchJob{
		ID:            b.ID,
		Object:        firstNonEmpty(b.Object, "batch"),
		Model:         model,
		Endpoint:      b.Endpoint,
		Status:        b.Status,
		InputFileID:   b.InputFileID,
		OutputFileID:  b.OutputFileID,
		ErrorFileID:   b.ErrorFileID,
		CreatedAt:     firstPositiveInt64(b.CreatedAt, time.Now().Unix()),
		ExpiresAt:     b.ExpiresAt,
		CompletedAt:   b.CompletedAt,
		FailedAt:      b.FailedAt,
		ProviderJobID: b.ID,
		Metadata:      b.Metadata,
	}
}

func (a *BatchAdapter) jsonRequest(ctx context.Context, method string, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal openai batch request: %w", err)
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.client.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build openai batch request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+a.client.apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.client.httpClient.Do(req)
	if err != nil {
		return translateTransportError(err, "OpenAI")
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= http.StatusBadRequest {
		return a.client.apiError(resp)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "OpenAI returned an invalid batch response.")
		}
	}
	return nil
}

func firstPositiveInt64(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
