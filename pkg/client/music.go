package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
)

func (c *Client) CreateMusicGeneration(ctx context.Context, req *MusicGenerationRequest) (*MusicOperationResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	switch normalizeMusicMode(req.Mode) {
	case "async":
		var response MusicJob
		if err := c.doJSON(ctx, http.MethodPost, "/v1/music/generations", nil, req, &response); err != nil {
			return nil, err
		}
		return &MusicOperationResponse{Job: &response}, nil
	case "stream":
		return nil, fmt.Errorf("stream mode requires StreamMusicGeneration")
	default:
		data, contentType, err := c.doBinary(ctx, http.MethodPost, "/v1/music/generations", nil, req)
		if err != nil {
			return nil, err
		}
		return &MusicOperationResponse{Asset: &MusicAsset{Data: data, ContentType: contentType}}, nil
	}
}

func (c *Client) StreamMusicGeneration(ctx context.Context, req *MusicGenerationRequest) (*MusicStream, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	payload := *req
	payload.Mode = "stream"
	resp, err := c.do(ctx, http.MethodPost, "/v1/music/generations", nil, payload, "application/json", "*/*")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		defer func() {
			_ = resp.Body.Close()
		}()
		return nil, decodeAPIErrorResponse(resp)
	}
	return &MusicStream{Body: resp.Body, ContentType: resp.Header.Get("Content-Type")}, nil
}

func (c *Client) EditMusic(ctx context.Context, req *MusicEditRequest) (*MusicOperationResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	mode := normalizeMusicMode(req.Mode)
	if mode == "stream" {
		return nil, fmt.Errorf("stream mode requires StreamMusicEdit")
	}
	if len(req.File) > 0 {
		if mode == "async" {
			var response MusicJob
			if err := c.doMusicMultipartJSON(ctx, "/v1/music/edits", req, &response); err != nil {
				return nil, err
			}
			return &MusicOperationResponse{Job: &response}, nil
		}
		data, contentType, err := c.doMusicMultipartBinary(ctx, "/v1/music/edits", req)
		if err != nil {
			return nil, err
		}
		return &MusicOperationResponse{Asset: &MusicAsset{Data: data, ContentType: contentType}}, nil
	}
	if mode == "async" {
		var response MusicJob
		if err := c.doJSON(ctx, http.MethodPost, "/v1/music/edits", nil, req, &response); err != nil {
			return nil, err
		}
		return &MusicOperationResponse{Job: &response}, nil
	}
	data, contentType, err := c.doBinary(ctx, http.MethodPost, "/v1/music/edits", nil, req)
	if err != nil {
		return nil, err
	}
	return &MusicOperationResponse{Asset: &MusicAsset{Data: data, ContentType: contentType}}, nil
}

func (c *Client) StreamMusicEdit(ctx context.Context, req *MusicEditRequest) (*MusicStream, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	payload := *req
	payload.Mode = "stream"
	if len(payload.File) > 0 {
		resp, err := c.doMusicMultipart(ctx, http.MethodPost, "/v1/music/edits", &payload, "*/*")
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= http.StatusBadRequest {
			defer func() {
				_ = resp.Body.Close()
			}()
			return nil, decodeAPIErrorResponse(resp)
		}
		return &MusicStream{Body: resp.Body, ContentType: resp.Header.Get("Content-Type")}, nil
	}
	resp, err := c.do(ctx, http.MethodPost, "/v1/music/edits", nil, payload, "application/json", "*/*")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		defer func() {
			_ = resp.Body.Close()
		}()
		return nil, decodeAPIErrorResponse(resp)
	}
	return &MusicStream{Body: resp.Body, ContentType: resp.Header.Get("Content-Type")}, nil
}

func (c *Client) SeparateMusicStems(ctx context.Context, req *MusicStemRequest) (*MusicOperationResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	mode := normalizeMusicMode(req.Mode)
	if len(req.File) > 0 {
		if mode == "async" {
			var response MusicJob
			if err := c.doMusicMultipartJSON(ctx, "/v1/music/stems", req, &response); err != nil {
				return nil, err
			}
			return &MusicOperationResponse{Job: &response}, nil
		}
		data, contentType, err := c.doMusicMultipartBinary(ctx, "/v1/music/stems", req)
		if err != nil {
			return nil, err
		}
		return &MusicOperationResponse{Asset: &MusicAsset{Data: data, ContentType: contentType}}, nil
	}
	if mode == "async" {
		var response MusicJob
		if err := c.doJSON(ctx, http.MethodPost, "/v1/music/stems", nil, req, &response); err != nil {
			return nil, err
		}
		return &MusicOperationResponse{Job: &response}, nil
	}
	data, contentType, err := c.doBinary(ctx, http.MethodPost, "/v1/music/stems", nil, req)
	if err != nil {
		return nil, err
	}
	return &MusicOperationResponse{Asset: &MusicAsset{Data: data, ContentType: contentType}}, nil
}

func (c *Client) CreateMusicLyrics(ctx context.Context, req *MusicLyricsRequest) (*MusicLyricsResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	var response MusicLyricsResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/music/lyrics", nil, req, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) CreateMusicPlan(ctx context.Context, req *MusicPlanRequest) (*MusicPlanResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	var response MusicPlanResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/music/plans", nil, req, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) GetMusicJob(ctx context.Context, jobID string) (*MusicStatus, error) {
	if jobID == "" {
		return nil, fmt.Errorf("jobID is required")
	}
	var response MusicStatus
	if err := c.doJSON(ctx, http.MethodGet, "/v1/music/jobs/"+jobID, nil, nil, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) CancelMusicJob(ctx context.Context, jobID string) error {
	if jobID == "" {
		return fmt.Errorf("jobID is required")
	}
	return c.doJSON(ctx, http.MethodDelete, "/v1/music/jobs/"+jobID, nil, nil, nil)
}

func (c *Client) GetMusicJobContent(ctx context.Context, jobID string) (*MusicAsset, error) {
	if jobID == "" {
		return nil, fmt.Errorf("jobID is required")
	}
	data, contentType, err := c.doBinary(ctx, http.MethodGet, "/v1/music/jobs/"+jobID+"/content", nil, nil)
	if err != nil {
		return nil, err
	}
	return &MusicAsset{Data: data, ContentType: contentType}, nil
}

func normalizeMusicMode(mode string) string {
	trimmed := strings.ToLower(strings.TrimSpace(mode))
	if trimmed == "" {
		return "sync"
	}
	return trimmed
}

func (c *Client) doMusicMultipartJSON(ctx context.Context, path string, req any, out any) error {
	resp, err := c.doMusicMultipart(ctx, http.MethodPost, path, req, "application/json")
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= http.StatusBadRequest {
		return decodeAPIErrorResponse(resp)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) doMusicMultipartBinary(ctx context.Context, path string, req any) ([]byte, string, error) {
	resp, err := c.doMusicMultipart(ctx, http.MethodPost, path, req, "*/*")
	if err != nil {
		return nil, "", err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, "", decodeAPIErrorResponse(resp)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read binary response: %w", err)
	}
	return data, resp.Header.Get("Content-Type"), nil
}

func (c *Client) doMusicMultipart(ctx context.Context, method string, path string, req any, accept string) (*http.Response, error) {
	body, contentType, err := buildMusicMultipartBody(req)
	if err != nil {
		return nil, err
	}
	return c.do(ctx, method, path, nil, bytes.NewReader(body), contentType, accept)
}

func buildMusicMultipartBody(req any) ([]byte, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	writeField := func(name string, value string) error {
		if strings.TrimSpace(value) == "" {
			return nil
		}
		return writer.WriteField(name, value)
	}

	switch value := req.(type) {
	case *MusicEditRequest:
		if err := writeField("mode", normalizeMusicMode(value.Mode)); err != nil {
			return nil, "", err
		}
		if err := writeField("model", value.Model); err != nil {
			return nil, "", err
		}
		if err := writeRoutingField(writer, value.Routing); err != nil {
			return nil, "", err
		}
		if err := writeField("operation", value.Operation); err != nil {
			return nil, "", err
		}
		if err := writeField("prompt", value.Prompt); err != nil {
			return nil, "", err
		}
		if err := writeField("lyrics", value.Lyrics); err != nil {
			return nil, "", err
		}
		if len(value.Plan) > 0 {
			if err := writeField("plan", string(value.Plan)); err != nil {
				return nil, "", err
			}
		}
		if err := writeField("source_job_id", value.SourceJobID); err != nil {
			return nil, "", err
		}
		if err := writeField("source_audio", value.SourceAudio); err != nil {
			return nil, "", err
		}
		if value.DurationMS > 0 {
			if err := writeField("duration_ms", strconv.Itoa(value.DurationMS)); err != nil {
				return nil, "", err
			}
		}
		if value.SampleRateHz > 0 {
			if err := writeField("sample_rate_hz", strconv.Itoa(value.SampleRateHz)); err != nil {
				return nil, "", err
			}
		}
		if value.Bitrate > 0 {
			if err := writeField("bitrate", strconv.Itoa(value.Bitrate)); err != nil {
				return nil, "", err
			}
		}
		if value.Seed != nil {
			if err := writeField("seed", strconv.Itoa(*value.Seed)); err != nil {
				return nil, "", err
			}
		}
		if err := writeField("output_format", value.OutputFormat); err != nil {
			return nil, "", err
		}
		if value.StoreForEditing {
			if err := writeField("store_for_editing", "true"); err != nil {
				return nil, "", err
			}
		}
		if value.SignWithC2PA {
			if err := writeField("sign_with_c2pa", "true"); err != nil {
				return nil, "", err
			}
		}
		if value.Instrumental {
			if err := writeField("instrumental", "true"); err != nil {
				return nil, "", err
			}
		}
		if err := writeMultipartFile(writer, "file", defaultFilename(value.Filename, "source_audio"), defaultContentType(value.ContentType, value.File), value.File); err != nil {
			return nil, "", err
		}
	case *MusicStemRequest:
		if err := writeField("mode", normalizeMusicMode(value.Mode)); err != nil {
			return nil, "", err
		}
		if err := writeField("model", value.Model); err != nil {
			return nil, "", err
		}
		if err := writeRoutingField(writer, value.Routing); err != nil {
			return nil, "", err
		}
		if err := writeField("source_job_id", value.SourceJobID); err != nil {
			return nil, "", err
		}
		if err := writeField("source_audio", value.SourceAudio); err != nil {
			return nil, "", err
		}
		if err := writeField("stem_variant", value.StemVariant); err != nil {
			return nil, "", err
		}
		if err := writeField("output_format", value.OutputFormat); err != nil {
			return nil, "", err
		}
		if value.SignWithC2PA {
			if err := writeField("sign_with_c2pa", "true"); err != nil {
				return nil, "", err
			}
		}
		if err := writeMultipartFile(writer, "file", defaultFilename(value.Filename, "source_audio"), defaultContentType(value.ContentType, value.File), value.File); err != nil {
			return nil, "", err
		}
	default:
		return nil, "", fmt.Errorf("unsupported multipart music request type")
	}

	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("close multipart body: %w", err)
	}
	return body.Bytes(), writer.FormDataContentType(), nil
}
