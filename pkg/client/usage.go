package client

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

func (c *Client) GetUsage(ctx context.Context, params *UsageParams) (*UsageReport, error) {
	query := url.Values{}
	if params != nil {
		if params.From != nil {
			query.Set("from", params.From.UTC().Format(time.RFC3339))
		}
		if params.To != nil {
			query.Set("to", params.To.UTC().Format(time.RFC3339))
		}
		if params.Model != "" {
			query.Set("model", params.Model)
		}
		if params.Modality != "" {
			query.Set("modality", params.Modality)
		}
		if params.GroupBy != "" {
			query.Set("group_by", params.GroupBy)
		}
	}

	var response UsageReport
	if err := c.doJSON(ctx, http.MethodGet, "/v1/usage", query, nil, &response); err != nil {
		return nil, err
	}
	return &response, nil
}
