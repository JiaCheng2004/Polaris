package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

func (c *Client) CreateAudioNote(ctx context.Context, req *AudioNoteRequest) (*AudioNoteJob, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	var response AudioNoteJob
	if err := c.doJSON(ctx, http.MethodPost, "/v1/audio/notes", nil, req, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) GetAudioNote(ctx context.Context, id string) (*AudioNoteJob, error) {
	if id == "" {
		return nil, fmt.Errorf("note id is required")
	}
	var response AudioNoteJob
	if err := c.doJSON(ctx, http.MethodGet, "/v1/audio/notes/"+url.PathEscape(id), nil, nil, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) DeleteAudioNote(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("note id is required")
	}
	return c.doJSON(ctx, http.MethodDelete, "/v1/audio/notes/"+url.PathEscape(id), nil, nil, nil)
}
