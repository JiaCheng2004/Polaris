package elevenlabs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

type MusicAdapter struct {
	client *Client
	model  string
}

func NewMusicAdapter(client *Client, model string) *MusicAdapter {
	return &MusicAdapter{client: client, model: model}
}

type composeRequest struct {
	Prompt                  string          `json:"prompt,omitempty"`
	CompositionPlan         json.RawMessage `json:"composition_plan,omitempty"`
	MusicLengthMS           int             `json:"music_length_ms,omitempty"`
	ModelID                 string          `json:"model_id,omitempty"`
	Seed                    *int            `json:"seed,omitempty"`
	ForceInstrumental       bool            `json:"force_instrumental,omitempty"`
	RespectSectionsDuration *bool           `json:"respect_sections_durations,omitempty"`
	StoreForInpainting      bool            `json:"store_for_inpainting,omitempty"`
	SignWithC2PA            bool            `json:"sign_with_c2pa,omitempty"`
}

type planRequest struct {
	Prompt                string          `json:"prompt"`
	MusicLengthMS         int             `json:"music_length_ms,omitempty"`
	SourceCompositionPlan json.RawMessage `json:"source_composition_plan,omitempty"`
	ModelID               string          `json:"model_id,omitempty"`
}

func (a *MusicAdapter) Generate(ctx context.Context, req *modality.MusicGenerationRequest) (*modality.MusicOperationResult, error) {
	payload, query, err := a.composePayload(req)
	if err != nil {
		return nil, err
	}
	resp, err := a.client.Raw(ctx, http.MethodPost, "/v1/music", query, payload, "*/*")
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read elevenlabs music response: %w", err)
	}
	contentType := firstNonEmpty(resp.Header.Get("Content-Type"), musicContentType(req.OutputFormat))
	return &modality.MusicOperationResult{
		Asset: &modality.MusicAsset{
			Data:        data,
			ContentType: contentType,
			Filename:    "music." + musicFileExtension(req.OutputFormat),
		},
		SongID:    strings.TrimSpace(resp.Header.Get("song-id")),
		SizeBytes: len(data),
	}, nil
}

func (a *MusicAdapter) StreamGenerate(ctx context.Context, req *modality.MusicGenerationRequest) (*modality.MusicStream, error) {
	payload, query, err := a.composePayload(req)
	if err != nil {
		return nil, err
	}
	resp, err := a.client.Raw(ctx, http.MethodPost, "/v1/music/stream", query, payload, "*/*")
	if err != nil {
		return nil, err
	}
	return &modality.MusicStream{
		Body:        resp.Body,
		ContentType: firstNonEmpty(resp.Header.Get("Content-Type"), musicContentType(req.OutputFormat)),
		Filename:    "music." + musicFileExtension(req.OutputFormat),
	}, nil
}

func (a *MusicAdapter) Edit(ctx context.Context, req *modality.MusicEditRequest) (*modality.MusicOperationResult, error) {
	return nil, httputil.NewError(http.StatusBadRequest, "capability_not_supported", "unsupported_music_operation", "operation", "ElevenLabs editing workflows are not enabled in this Polaris build.")
}

func (a *MusicAdapter) StreamEdit(ctx context.Context, req *modality.MusicEditRequest) (*modality.MusicStream, error) {
	return nil, httputil.NewError(http.StatusBadRequest, "capability_not_supported", "unsupported_music_operation", "operation", "ElevenLabs editing workflows are not enabled in this Polaris build.")
}

func (a *MusicAdapter) SeparateStems(ctx context.Context, req *modality.MusicStemRequest) (*modality.MusicOperationResult, error) {
	file, filename, contentType, err := resolveSourceFile(req)
	if err != nil {
		return nil, err
	}
	query := url.Values{}
	if value := elevenOutputFormat(req.OutputFormat, 0, 0); value != "" {
		query.Set("output_format", value)
	}
	fields := map[string]string{}
	if strings.TrimSpace(req.StemVariant) != "" {
		fields["stem_variation_id"] = strings.TrimSpace(req.StemVariant)
	}
	if req.SignWithC2PA {
		fields["sign_with_c2pa"] = "true"
	}
	resp, err := a.client.UploadFile(ctx, "/v1/music/stem-separation", "file", filename, contentType, file, fields, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read elevenlabs stems response: %w", err)
	}
	return &modality.MusicOperationResult{
		Asset: &modality.MusicAsset{
			Data:        payload,
			ContentType: firstNonEmpty(resp.Header.Get("Content-Type"), "application/zip"),
			Filename:    "stems.zip",
		},
		SizeBytes: len(payload),
	}, nil
}

func (a *MusicAdapter) GenerateLyrics(ctx context.Context, req *modality.MusicLyricsRequest) (*modality.MusicLyricsResponse, error) {
	return nil, httputil.NewError(http.StatusBadRequest, "capability_not_supported", "lyrics_generation_not_supported", "model", "ElevenLabs does not expose a standalone lyrics generation endpoint through Polaris.")
}

func (a *MusicAdapter) CreatePlan(ctx context.Context, req *modality.MusicPlanRequest) (*modality.MusicPlanResponse, error) {
	payload := planRequest{
		Prompt:        req.Prompt,
		MusicLengthMS: req.DurationMS,
		ModelID:       providerModelName(req.Model, a.model),
	}
	if len(req.SourcePlan) > 0 {
		payload.SourceCompositionPlan = append(json.RawMessage(nil), req.SourcePlan...)
	}
	var response map[string]any
	if _, err := a.client.JSON(ctx, http.MethodPost, "/v1/music/plan", nil, payload, &response); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("marshal elevenlabs plan response: %w", err)
	}
	return &modality.MusicPlanResponse{Plan: raw}, nil
}

func (a *MusicAdapter) composePayload(req *modality.MusicGenerationRequest) (composeRequest, url.Values, error) {
	payload := composeRequest{
		Prompt:             req.Prompt,
		MusicLengthMS:      req.DurationMS,
		ModelID:            providerModelName(req.Model, a.model),
		Seed:               req.Seed,
		ForceInstrumental:  req.Instrumental,
		StoreForInpainting: req.StoreForEditing,
		SignWithC2PA:       req.SignWithC2PA,
	}
	if len(req.Plan) > 0 {
		payload.CompositionPlan = append(json.RawMessage(nil), req.Plan...)
	}
	if strings.TrimSpace(req.Prompt) == "" && len(req.Plan) == 0 {
		return composeRequest{}, nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_prompt", "prompt", "Field 'prompt' or 'plan' is required.")
	}
	query := url.Values{}
	if value := elevenOutputFormat(req.OutputFormat, req.SampleRateHz, req.Bitrate); value != "" {
		query.Set("output_format", value)
	}
	return payload, query, nil
}

func resolveSourceFile(req *modality.MusicStemRequest) ([]byte, string, string, error) {
	switch {
	case len(req.File) > 0:
		filename := strings.TrimSpace(req.Filename)
		if filename == "" {
			filename = "source_audio"
		}
		contentType := strings.TrimSpace(req.ContentType)
		if contentType == "" {
			contentType = "audio/mpeg"
		}
		return req.File, filename, contentType, nil
	case strings.TrimSpace(req.SourceAudio) != "":
		return nil, "", "", httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_source_audio", "source_audio", "ElevenLabs stems currently require an uploaded file or a previous music job.")
	default:
		return nil, "", "", httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_source_audio", "source_audio", "Field 'source_audio', an uploaded file, or 'source_job_id' is required.")
	}
}

func elevenOutputFormat(format string, sampleRateHz int, bitrate int) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "mp3":
		if sampleRateHz <= 0 {
			sampleRateHz = 44100
		}
		if bitrate <= 0 {
			bitrate = 128
		}
		return fmt.Sprintf("mp3_%d_%d", sampleRateHz, bitrate)
	case "pcm", "wav":
		if sampleRateHz <= 0 {
			sampleRateHz = 44100
		}
		return fmt.Sprintf("pcm_%d", sampleRateHz)
	case "opus":
		if sampleRateHz <= 0 {
			sampleRateHz = 48000
		}
		if bitrate <= 0 {
			bitrate = 64
		}
		return fmt.Sprintf("opus_%d_%d", sampleRateHz, bitrate)
	case "ulaw":
		return "ulaw_8000"
	case "alaw":
		return "alaw_8000"
	default:
		return strings.TrimSpace(format)
	}
}

func musicContentType(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "pcm", "wav":
		return "audio/wav"
	case "opus":
		return "audio/opus"
	case "ulaw":
		return "audio/basic"
	case "alaw":
		return "audio/basic"
	default:
		return "audio/mpeg"
	}
}

func musicFileExtension(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "pcm":
		return "wav"
	case "wav":
		return "wav"
	case "opus":
		return "opus"
	case "ulaw":
		return "ulaw"
	case "alaw":
		return "alaw"
	default:
		return "mp3"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func providerModelName(requestModel string, fallback string) string {
	if idx := strings.Index(strings.TrimSpace(requestModel), "/"); idx >= 0 {
		return requestModel[idx+1:]
	}
	if idx := strings.Index(strings.TrimSpace(fallback), "/"); idx >= 0 {
		return fallback[idx+1:]
	}
	return strings.TrimSpace(requestModel)
}
