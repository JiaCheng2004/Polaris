package client

import (
	"context"
	"fmt"
	"net/http"
)

func (c *Client) CreateVideoGeneration(ctx context.Context, req *VideoGenerationRequest) (*VideoJob, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	var response VideoJob
	if err := c.doJSON(ctx, http.MethodPost, "/v1/video/generations", nil, req, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) GetVideoGeneration(ctx context.Context, jobID string) (*VideoStatus, error) {
	if jobID == "" {
		return nil, fmt.Errorf("jobID is required")
	}

	var response VideoStatus
	if err := c.doJSON(ctx, http.MethodGet, "/v1/video/generations/"+jobID, nil, nil, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) CancelVideoGeneration(ctx context.Context, jobID string) error {
	if jobID == "" {
		return fmt.Errorf("jobID is required")
	}

	return c.doJSON(ctx, http.MethodDelete, "/v1/video/generations/"+jobID, nil, nil, nil)
}

func (c *Client) GetVideoGenerationContent(ctx context.Context, jobID string) (*VideoAsset, error) {
	if jobID == "" {
		return nil, fmt.Errorf("jobID is required")
	}

	data, contentType, err := c.doBinary(ctx, http.MethodGet, "/v1/video/generations/"+jobID+"/content", nil, nil)
	if err != nil {
		return nil, err
	}
	return &VideoAsset{
		Data:        data,
		ContentType: contentType,
	}, nil
}
