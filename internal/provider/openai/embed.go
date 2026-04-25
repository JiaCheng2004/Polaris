package openai

import (
	"context"
	"strings"

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

func (a *EmbedAdapter) Embed(ctx context.Context, req *modality.EmbedRequest) (*modality.EmbedResponse, error) {
	payload := *req
	payload.Model = providerModelName(payload.Model, a.model)

	var response modality.EmbedResponse
	if err := a.client.JSON(ctx, "/embeddings", payload, &response); err != nil {
		return nil, err
	}

	if response.Object == "" {
		response.Object = "list"
	}
	if response.Model == "" || !strings.Contains(response.Model, "/") {
		response.Model = firstNonEmpty(req.Model, a.model)
	}
	for index := range response.Data {
		if response.Data[index].Object == "" {
			response.Data[index].Object = "embedding"
		}
		if response.Data[index].Index == 0 && index != 0 {
			response.Data[index].Index = index
		}
	}
	if response.Usage.TotalTokens == 0 {
		response.Usage.TotalTokens = response.Usage.PromptTokens
	}
	if len(response.Data) == 0 {
		response.Data = []modality.Embedding{}
	}
	return &response, nil
}
