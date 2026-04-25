package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

func (c *Client) CreatePodcast(ctx context.Context, req *PodcastRequest) (*PodcastJob, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	var response PodcastJob
	if err := c.doJSON(ctx, http.MethodPost, "/v1/audio/podcasts", nil, req, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) GetPodcast(ctx context.Context, id string) (*PodcastStatus, error) {
	if id == "" {
		return nil, fmt.Errorf("podcast id is required")
	}
	var response PodcastStatus
	if err := c.doJSON(ctx, http.MethodGet, "/v1/audio/podcasts/"+url.PathEscape(id), nil, nil, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) GetPodcastContent(ctx context.Context, id string) (*PodcastAsset, error) {
	if id == "" {
		return nil, fmt.Errorf("podcast id is required")
	}
	data, contentType, err := c.doBinary(ctx, http.MethodGet, "/v1/audio/podcasts/"+url.PathEscape(id)+"/content", nil, nil)
	if err != nil {
		return nil, err
	}
	return &PodcastAsset{Data: data, ContentType: contentType}, nil
}

func (c *Client) CancelPodcast(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("podcast id is required")
	}
	return c.doJSON(ctx, http.MethodDelete, "/v1/audio/podcasts/"+url.PathEscape(id), nil, nil, nil)
}
