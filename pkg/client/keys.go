package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (c *Client) CreateKey(ctx context.Context, req *CreateKeyRequest) (*APIKey, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	var response APIKey
	if err := c.doJSON(ctx, http.MethodPost, "/v1/keys", nil, req, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) ListKeys(ctx context.Context, params *ListKeysParams) (*APIKeyList, error) {
	query := url.Values{}
	if params != nil {
		if strings.TrimSpace(params.OwnerID) != "" {
			query.Set("owner_id", strings.TrimSpace(params.OwnerID))
		}
		if params.IncludeRevoked != nil {
			query.Set("include_revoked", strconv.FormatBool(*params.IncludeRevoked))
		}
	}

	var response APIKeyList
	if err := c.doJSON(ctx, http.MethodGet, "/v1/keys", query, nil, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) DeleteKey(ctx context.Context, id string) error {
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		return fmt.Errorf("id is required")
	}

	path := "/v1/keys/" + trimmedID
	resp, err := c.do(ctx, http.MethodDelete, path, nil, nil, "", "application/json")
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= http.StatusBadRequest {
		return decodeAPIErrorResponse(resp)
	}
	return nil
}
