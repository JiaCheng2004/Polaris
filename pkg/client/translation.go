package client

import (
	"context"
	"fmt"
	"net/http"
)

func (c *Client) CreateTranslation(ctx context.Context, req *TranslationRequest) (*TranslationResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	var response TranslationResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/translations", nil, req, &response); err != nil {
		return nil, err
	}
	return &response, nil
}
