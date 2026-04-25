package client

import (
	"context"
	"fmt"
	"net/http"
)

func (c *Client) CreateEmbedding(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	var response EmbeddingResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/embeddings", nil, req, &response); err != nil {
		return nil, err
	}
	return &response, nil
}
