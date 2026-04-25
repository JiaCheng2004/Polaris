package google

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"math"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

type EmbedAdapter struct {
	client *Client
	model  string
}

func NewEmbedAdapter(client *Client, model string) *EmbedAdapter {
	return &EmbedAdapter{
		client: client,
		model:  model,
	}
}

type embedContentRequest struct {
	Model                string        `json:"model,omitempty"`
	Content              googleContent `json:"content"`
	OutputDimensionality *int          `json:"outputDimensionality,omitempty"`
}

type embedContentResponse struct {
	Embedding     *googleEmbedding        `json:"embedding,omitempty"`
	Embeddings    []googleEmbedding       `json:"embeddings,omitempty"`
	UsageMetadata *embedContentUsageStats `json:"usageMetadata,omitempty"`
}

type googleEmbedding struct {
	Values []float32 `json:"values"`
}

type embedContentUsageStats struct {
	PromptTokenCount int `json:"promptTokenCount,omitempty"`
	TotalTokenCount  int `json:"totalTokenCount,omitempty"`
}

type batchEmbedContentsRequest struct {
	Requests []embedContentRequest `json:"requests"`
}

type batchEmbedContentsResponse struct {
	Embeddings []googleEmbedding `json:"embeddings"`
}

func (a *EmbedAdapter) Embed(ctx context.Context, req *modality.EmbedRequest) (*modality.EmbedResponse, error) {
	values := req.Input.Values()
	if len(values) <= 1 {
		return a.embedSingle(ctx, req, firstEmbedValue(values))
	}
	return a.embedBatch(ctx, req, values)
}

func (a *EmbedAdapter) embedSingle(ctx context.Context, req *modality.EmbedRequest, value string) (*modality.EmbedResponse, error) {
	payload := embedContentRequest{
		Model: "models/" + providerEmbeddingModelName(req.Model, a.model),
		Content: googleContent{
			Parts: embedParts([]string{value}),
		},
		OutputDimensionality: req.Dimensions,
	}

	var response embedContentResponse
	if err := a.client.JSON(ctx, a.embedPath(req.Model), payload, &response); err != nil {
		return nil, err
	}
	embeddings := response.normalizedEmbeddings()
	if len(embeddings) == 0 {
		return nil, providerInvalidEmbeddingResponse()
	}
	return buildEmbedResponse(req, a.model, embeddings, response.usage()), nil
}

func (a *EmbedAdapter) embedBatch(ctx context.Context, req *modality.EmbedRequest, values []string) (*modality.EmbedResponse, error) {
	requests := make([]embedContentRequest, 0, len(values))
	for _, value := range values {
		requests = append(requests, embedContentRequest{
			Model: "models/" + providerEmbeddingModelName(req.Model, a.model),
			Content: googleContent{
				Parts: embedParts([]string{value}),
			},
			OutputDimensionality: req.Dimensions,
		})
	}

	var response batchEmbedContentsResponse
	if err := a.client.JSON(ctx, a.batchEmbedPath(req.Model), batchEmbedContentsRequest{Requests: requests}, &response); err != nil {
		return nil, err
	}
	if len(response.Embeddings) == 0 {
		return nil, providerInvalidEmbeddingResponse()
	}
	return buildEmbedResponse(req, a.model, response.Embeddings, modality.EmbedUsage{}), nil
}

func buildEmbedResponse(req *modality.EmbedRequest, fallbackModel string, embeddings []googleEmbedding, usage modality.EmbedUsage) *modality.EmbedResponse {
	data := make([]modality.Embedding, 0, len(embeddings))
	for index, embedding := range embeddings {
		values := modality.EmbeddingValues{}
		if req.EncodingFormat == "base64" {
			values.Base64 = encodeFloat32Base64(embedding.Values)
		} else {
			values.Float32 = append([]float32(nil), embedding.Values...)
		}
		data = append(data, modality.Embedding{
			Object:    "embedding",
			Index:     index,
			Embedding: values,
		})
	}

	return &modality.EmbedResponse{
		Object: "list",
		Data:   data,
		Model:  firstNonEmpty(req.Model, fallbackModel),
		Usage:  usage,
	}
}

func (a *EmbedAdapter) embedPath(requestModel string) string {
	return "/v1beta/models/" + providerEmbeddingModelName(requestModel, a.model) + ":embedContent"
}

func (a *EmbedAdapter) batchEmbedPath(requestModel string) string {
	return "/v1beta/models/" + providerEmbeddingModelName(requestModel, a.model) + ":batchEmbedContents"
}

func embedParts(values []string) []googlePart {
	parts := make([]googlePart, 0, len(values))
	for _, value := range values {
		parts = append(parts, googlePart{Text: value})
	}
	return parts
}

func firstEmbedValue(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (r embedContentResponse) normalizedEmbeddings() []googleEmbedding {
	if len(r.Embeddings) > 0 {
		return append([]googleEmbedding(nil), r.Embeddings...)
	}
	if r.Embedding == nil {
		return nil
	}
	return []googleEmbedding{*r.Embedding}
}

func (r embedContentResponse) usage() modality.EmbedUsage {
	if r.UsageMetadata == nil {
		return modality.EmbedUsage{}
	}
	return modality.EmbedUsage{
		PromptTokens: r.UsageMetadata.PromptTokenCount,
		TotalTokens:  r.UsageMetadata.TotalTokenCount,
		Source:       modality.TokenCountSourceProviderReported,
	}
}

func providerInvalidEmbeddingResponse() error {
	return httputil.NewError(502, "provider_error", "provider_invalid_response", "", "Google returned an embedding response without embedding data.")
}

func providerEmbeddingModelName(requestModel string, fallbackModel string) string {
	name := providerModelName(requestModel, fallbackModel)
	switch name {
	case "gemini-embedding":
		return "gemini-embedding-001"
	default:
		return name
	}
}

func encodeFloat32Base64(values []float32) string {
	buf := make([]byte, len(values)*4)
	for i, value := range values {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(value))
	}
	return base64.StdEncoding.EncodeToString(buf)
}
