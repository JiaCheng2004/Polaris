package bedrock

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"math"
	"net/http"
	"net/url"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

type EmbedAdapter struct {
	client *Client
	model  string
}

type embedRequest struct {
	InputText  string `json:"inputText"`
	Dimensions *int   `json:"dimensions,omitempty"`
	Normalize  bool   `json:"normalize"`
}

type embedResponse struct {
	Embedding           []float32 `json:"embedding"`
	InputTextTokenCount int       `json:"inputTextTokenCount"`
}

func NewEmbedAdapter(client *Client, model string) *EmbedAdapter {
	return &EmbedAdapter{
		client: client,
		model:  model,
	}
}

func (a *EmbedAdapter) Embed(ctx context.Context, req *modality.EmbedRequest) (*modality.EmbedResponse, error) {
	if req.EncodingFormat != "" && req.EncodingFormat != "float" && req.EncodingFormat != "base64" {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_encoding_format", "encoding_format", "Field 'encoding_format' must be 'float' or 'base64'.")
	}
	if req.Dimensions != nil && *req.Dimensions != 256 && *req.Dimensions != 512 && *req.Dimensions != 1024 {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_dimensions", "dimensions", "Amazon Titan Text Embeddings V2 only supports dimensions 256, 512, or 1024.")
	}

	values := req.Input.Values()
	data := make([]modality.Embedding, 0, len(values))
	usage := modality.EmbedUsage{Source: modality.TokenCountSourceProviderReported}
	providerModel := providerModelName(req.Model, a.model)

	for index, value := range values {
		payload := embedRequest{
			InputText:  value,
			Dimensions: req.Dimensions,
			Normalize:  true,
		}

		var response embedResponse
		if err := a.client.JSON(ctx, invokePath(providerModel), payload, &response); err != nil {
			return nil, err
		}
		if len(response.Embedding) == 0 {
			return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "Amazon Bedrock returned an embedding response without embedding data.")
		}

		usage.PromptTokens += response.InputTextTokenCount
		values := modality.EmbeddingValues{}
		if req.EncodingFormat == "base64" {
			values.Base64 = encodeEmbeddingBase64(response.Embedding)
		} else {
			values.Float32 = append([]float32(nil), response.Embedding...)
		}
		data = append(data, modality.Embedding{
			Object:    "embedding",
			Index:     index,
			Embedding: values,
		})
	}
	usage.TotalTokens = usage.PromptTokens

	return &modality.EmbedResponse{
		Object: "list",
		Data:   data,
		Model:  firstNonEmpty(req.Model, a.model),
		Usage:  usage,
	}, nil
}

func invokePath(model string) string {
	return "/model/" + url.PathEscape(model) + "/invoke"
}

func encodeEmbeddingBase64(values []float32) string {
	buf := make([]byte, len(values)*4)
	for i, value := range values {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(value))
	}
	return base64.StdEncoding.EncodeToString(buf)
}
