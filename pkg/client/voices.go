package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (c *Client) ListVoices(ctx context.Context, req *VoiceListRequest) (*VoiceList, error) {
	query := url.Values{}
	if req != nil {
		if trimmed := strings.TrimSpace(req.Provider); trimmed != "" {
			query.Set("provider", trimmed)
		}
		if trimmed := strings.TrimSpace(req.Model); trimmed != "" {
			query.Set("model", trimmed)
		}
		if trimmed := strings.TrimSpace(req.Scope); trimmed != "" {
			query.Set("scope", trimmed)
		}
		if trimmed := strings.TrimSpace(req.Type); trimmed != "" {
			query.Set("type", trimmed)
		}
		if trimmed := strings.TrimSpace(req.State); trimmed != "" {
			query.Set("state", trimmed)
		}
		if req.Limit > 0 {
			query.Set("limit", strconv.Itoa(req.Limit))
		}
		if req.IncludeArchived {
			query.Set("include_archived", "true")
		}
	}

	var response VoiceList
	if err := c.doJSON(ctx, http.MethodGet, "/v1/voices", query, nil, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) GetVoice(ctx context.Context, id string, req *VoiceListRequest) (*VoiceItem, error) {
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("voice id is required")
	}
	query := url.Values{}
	if req != nil {
		if trimmed := strings.TrimSpace(req.Provider); trimmed != "" {
			query.Set("provider", trimmed)
		}
		if trimmed := strings.TrimSpace(req.Model); trimmed != "" {
			query.Set("model", trimmed)
		}
		if trimmed := strings.TrimSpace(req.Scope); trimmed != "" {
			query.Set("scope", trimmed)
		}
		if trimmed := strings.TrimSpace(req.Type); trimmed != "" {
			query.Set("type", trimmed)
		}
		if req.IncludeArchived {
			query.Set("include_archived", "true")
		}
	}
	var response VoiceItem
	if err := c.doJSON(ctx, http.MethodGet, "/v1/voices/"+url.PathEscape(id), query, nil, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) CreateVoiceClone(ctx context.Context, req *VoiceCloneRequest) (*VoiceItem, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	var response VoiceItem
	if err := c.doJSON(ctx, http.MethodPost, "/v1/voices/clones", nil, req, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) CreateVoiceDesign(ctx context.Context, req *VoiceDesignRequest) (*VoiceItem, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	var response VoiceItem
	if err := c.doJSON(ctx, http.MethodPost, "/v1/voices/designs", nil, req, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) RetrainVoice(ctx context.Context, id string, req *VoiceCloneRequest) (*VoiceItem, error) {
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("voice id is required")
	}
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	var response VoiceItem
	if err := c.doJSON(ctx, http.MethodPost, "/v1/voices/"+url.PathEscape(id)+"/retrain", nil, req, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) ActivateVoice(ctx context.Context, id string, req *VoiceActivationRequest) (*VoiceItem, error) {
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("voice id is required")
	}
	if req == nil {
		req = &VoiceActivationRequest{}
	}
	var response VoiceItem
	if err := c.doJSON(ctx, http.MethodPost, "/v1/voices/"+url.PathEscape(id)+"/activate", nil, req, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) DeleteVoice(ctx context.Context, id string, req *VoiceListRequest) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("voice id is required")
	}
	query := url.Values{}
	if req != nil {
		if trimmed := strings.TrimSpace(req.Provider); trimmed != "" {
			query.Set("provider", trimmed)
		}
		if trimmed := strings.TrimSpace(req.Model); trimmed != "" {
			query.Set("model", trimmed)
		}
	}
	return c.doJSON(ctx, http.MethodDelete, "/v1/voices/"+url.PathEscape(id), query, nil, nil)
}

func (c *Client) ArchiveVoice(ctx context.Context, id string, req *VoiceListRequest) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("voice id is required")
	}
	query := url.Values{}
	if req != nil {
		if trimmed := strings.TrimSpace(req.Provider); trimmed != "" {
			query.Set("provider", trimmed)
		}
		if trimmed := strings.TrimSpace(req.Model); trimmed != "" {
			query.Set("model", trimmed)
		}
	}
	return c.doJSON(ctx, http.MethodPost, "/v1/voices/"+url.PathEscape(id)+"/archive", query, nil, nil)
}

func (c *Client) UnarchiveVoice(ctx context.Context, id string, req *VoiceListRequest) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("voice id is required")
	}
	query := url.Values{}
	if req != nil {
		if trimmed := strings.TrimSpace(req.Provider); trimmed != "" {
			query.Set("provider", trimmed)
		}
		if trimmed := strings.TrimSpace(req.Model); trimmed != "" {
			query.Set("model", trimmed)
		}
	}
	return c.doJSON(ctx, http.MethodPost, "/v1/voices/"+url.PathEscape(id)+"/unarchive", query, nil, nil)
}
