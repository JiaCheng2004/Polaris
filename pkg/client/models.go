package client

import (
	"context"
	"net/http"
	"net/url"
)

func (c *Client) ListModels(ctx context.Context, includeAliases bool) (*ModelList, error) {
	query := url.Values{}
	if includeAliases {
		query.Set("include_aliases", "true")
	}

	var response ModelList
	if err := c.doJSON(ctx, http.MethodGet, "/v1/models", query, nil, &response); err != nil {
		return nil, err
	}
	return &response, nil
}
