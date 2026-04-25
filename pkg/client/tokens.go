package client

import (
	"context"
	"fmt"
	"net/http"
)

func (c *Client) CountTokens(ctx context.Context, req *TokenCountRequest) (*TokenCountResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	var response TokenCountResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/tokens/count", nil, req, &response); err != nil {
		return nil, err
	}
	return &response, nil
}
