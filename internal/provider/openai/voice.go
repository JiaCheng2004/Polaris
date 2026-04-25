package openai

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

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

type VoiceAdapter struct {
	client *Client
	model  string
}

type ttsRequest struct {
	Model          string   `json:"model"`
	Input          string   `json:"input"`
	Voice          string   `json:"voice"`
	ResponseFormat string   `json:"response_format,omitempty"`
	Speed          *float64 `json:"speed,omitempty"`
}

type verboseTranscriptionResponse struct {
	Text     string                      `json:"text"`
	Language string                      `json:"language"`
	Duration float64                     `json:"duration"`
	Segments []verboseTranscriptionSlice `json:"segments"`
}

type verboseTranscriptionSlice struct {
	ID    int     `json:"id"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

func NewVoiceAdapter(client *Client, model string) *VoiceAdapter {
	return &VoiceAdapter{
		client: client,
		model:  model,
	}
}

func (a *VoiceAdapter) TextToSpeech(ctx context.Context, req *modality.TTSRequest) (*modality.AudioResponse, error) {
	payload := ttsRequest{
		Model:          providerModelName(req.Model, a.model),
		Input:          req.Input,
		Voice:          req.Voice,
		ResponseFormat: req.ResponseFormat,
		Speed:          req.Speed,
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal openai tts request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.client.baseURL+"/audio/speech", bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("build openai tts request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+a.client.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "*/*")

	resp, err := a.client.httpClient.Do(httpReq)
	if err != nil {
		return nil, translateTransportError(err, "OpenAI")
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, a.client.apiError(resp)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "OpenAI returned an invalid audio response.")
	}

	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = openAIAudioContentType(req.ResponseFormat)
	}

	return &modality.AudioResponse{
		Data:        data,
		ContentType: contentType,
	}, nil
}

func (a *VoiceAdapter) SpeechToText(ctx context.Context, req *modality.STTRequest) (*modality.TranscriptResponse, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	writeField := func(name, value string) error {
		if strings.TrimSpace(value) == "" {
			return nil
		}
		return writer.WriteField(name, value)
	}

	providerModel := providerModelName(req.Model, a.model)
	providerFormat := openAITranscriptionFormat(providerModel, req.ResponseFormat)

	if err := writeField("model", providerModel); err != nil {
		return nil, fmt.Errorf("write transcription model: %w", err)
	}
	if err := writeField("language", req.Language); err != nil {
		return nil, fmt.Errorf("write transcription language: %w", err)
	}
	if err := writeField("response_format", providerFormat); err != nil {
		return nil, fmt.Errorf("write transcription response_format: %w", err)
	}
	if providerFormat == "verbose_json" {
		if err := writeField("timestamp_granularities[]", "segment"); err != nil {
			return nil, fmt.Errorf("write transcription timestamp_granularities: %w", err)
		}
	}
	if req.Temperature != nil {
		if err := writeField("temperature", strconv.FormatFloat(*req.Temperature, 'f', -1, 64)); err != nil {
			return nil, fmt.Errorf("write transcription temperature: %w", err)
		}
	}
	if err := writeFile(writer, "file", req.Filename, req.ContentType, req.File); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close transcription multipart writer: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.client.baseURL+"/audio/transcriptions", bytes.NewReader(body.Bytes()))
	if err != nil {
		return nil, fmt.Errorf("build openai transcription request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+a.client.apiKey)
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	httpReq.Header.Set("Accept", "*/*")

	resp, err := a.client.httpClient.Do(httpReq)
	if err != nil {
		return nil, translateTransportError(err, "OpenAI")
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, a.client.apiError(resp)
	}

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "OpenAI returned an invalid transcription response.")
	}

	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = openAITranscriptContentType(req.ResponseFormat)
	}

	if req.ResponseFormat != "json" {
		return &modality.TranscriptResponse{
			Text:        string(payload),
			Raw:         payload,
			ContentType: contentType,
			Format:      req.ResponseFormat,
		}, nil
	}

	var verbose verboseTranscriptionResponse
	if err := json.Unmarshal(payload, &verbose); err != nil {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "OpenAI returned an invalid transcription JSON response.")
	}

	segments := make([]modality.TranscriptSegment, 0, len(verbose.Segments))
	for _, segment := range verbose.Segments {
		segments = append(segments, modality.TranscriptSegment{
			ID:    segment.ID,
			Start: segment.Start,
			End:   segment.End,
			Text:  segment.Text,
		})
	}

	return &modality.TranscriptResponse{
		Text:        verbose.Text,
		Language:    verbose.Language,
		Duration:    verbose.Duration,
		Segments:    segments,
		Raw:         payload,
		ContentType: contentType,
		Format:      req.ResponseFormat,
	}, nil
}

func openAITranscriptionFormat(model string, requested string) string {
	if strings.EqualFold(strings.TrimSpace(requested), "json") && strings.EqualFold(strings.TrimSpace(model), "whisper-1") {
		return "verbose_json"
	}
	return requested
}

func openAIAudioContentType(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "mp3":
		return "audio/mpeg"
	case "wav":
		return "audio/wav"
	case "opus":
		return "audio/ogg"
	case "aac":
		return "audio/aac"
	case "flac":
		return "audio/flac"
	case "pcm":
		return "audio/pcm"
	default:
		return "application/octet-stream"
	}
}

func openAITranscriptContentType(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "json":
		return "application/json"
	case "text":
		return "text/plain; charset=utf-8"
	case "srt":
		return "application/x-subrip; charset=utf-8"
	case "vtt":
		return "text/vtt; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}
