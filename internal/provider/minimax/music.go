package minimax

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"net/http"
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

type musicGenerateRequest struct {
	Model           string        `json:"model"`
	Prompt          string        `json:"prompt,omitempty"`
	Lyrics          string        `json:"lyrics,omitempty"`
	Stream          bool          `json:"stream,omitempty"`
	OutputFormat    string        `json:"output_format,omitempty"`
	AudioSetting    *audioSetting `json:"audio_setting,omitempty"`
	LyricsOptimizer bool          `json:"lyrics_optimizer,omitempty"`
	IsInstrumental  bool          `json:"is_instrumental,omitempty"`
	AudioURL        string        `json:"audio_url,omitempty"`
	AudioBase64     string        `json:"audio_base64,omitempty"`
}

type audioSetting struct {
	SampleRate int    `json:"sample_rate,omitempty"`
	Bitrate    int    `json:"bitrate,omitempty"`
	Format     string `json:"format,omitempty"`
}

type musicGenerateResponse struct {
	Data struct {
		Audio  string `json:"audio"`
		Status int    `json:"status"`
	} `json:"data"`
	ExtraInfo struct {
		MusicDuration   int `json:"music_duration"`
		MusicSampleRate int `json:"music_sample_rate"`
		Bitrate         int `json:"bitrate"`
		MusicSize       int `json:"music_size"`
	} `json:"extra_info"`
	BaseResp struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
}

type lyricsRequest struct {
	Mode   string `json:"mode"`
	Prompt string `json:"prompt,omitempty"`
	Lyrics string `json:"lyrics,omitempty"`
	Title  string `json:"title,omitempty"`
}

type lyricsResponse struct {
	SongTitle string `json:"song_title"`
	StyleTags string `json:"style_tags"`
	Lyrics    string `json:"lyrics"`
	BaseResp  struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
}

func (a *MusicAdapter) Generate(ctx context.Context, req *modality.MusicGenerationRequest) (*modality.MusicOperationResult, error) {
	payload := musicGenerateRequest{
		Model:           providerModelName(req.Model, a.model),
		Prompt:          req.Prompt,
		Lyrics:          req.Lyrics,
		OutputFormat:    "hex",
		LyricsOptimizer: strings.TrimSpace(req.Lyrics) == "" && strings.TrimSpace(req.Prompt) != "" && !req.Instrumental,
		IsInstrumental:  req.Instrumental,
	}
	if setting := minimaxAudioSetting(req.OutputFormat, req.SampleRateHz, req.Bitrate); setting != nil {
		payload.AudioSetting = setting
	}
	var response musicGenerateResponse
	if _, err := a.client.JSON(ctx, http.MethodPost, "/v1/music_generation", payload, &response); err != nil {
		return nil, err
	}
	if err := minimaxBaseRespError(response.BaseResp.StatusCode, response.BaseResp.StatusMsg); err != nil {
		return nil, err
	}
	return decodeMiniMaxMusicResponse(&response, req.OutputFormat)
}

func (a *MusicAdapter) StreamGenerate(ctx context.Context, req *modality.MusicGenerationRequest) (*modality.MusicStream, error) {
	return nil, httputil.NewError(http.StatusBadRequest, "capability_not_supported", "music_streaming_not_supported", "model", "MiniMax music streaming is not enabled in this Polaris build.")
}

func (a *MusicAdapter) Edit(ctx context.Context, req *modality.MusicEditRequest) (*modality.MusicOperationResult, error) {
	if strings.ToLower(strings.TrimSpace(req.Operation)) != "cover" {
		return nil, httputil.NewError(http.StatusBadRequest, "capability_not_supported", "unsupported_music_operation", "operation", "MiniMax currently supports only the cover edit operation.")
	}

	audioURL, audioBase64, err := resolveMiniMaxReferenceAudio(req)
	if err != nil {
		return nil, err
	}
	payload := musicGenerateRequest{
		Model:           providerModelName(req.Model, a.model),
		Prompt:          req.Prompt,
		Lyrics:          req.Lyrics,
		OutputFormat:    "hex",
		AudioURL:        audioURL,
		AudioBase64:     audioBase64,
		LyricsOptimizer: false,
	}
	if setting := minimaxAudioSetting(req.OutputFormat, req.SampleRateHz, req.Bitrate); setting != nil {
		payload.AudioSetting = setting
	}
	var response musicGenerateResponse
	if _, err := a.client.JSON(ctx, http.MethodPost, "/v1/music_generation", payload, &response); err != nil {
		return nil, err
	}
	if err := minimaxBaseRespError(response.BaseResp.StatusCode, response.BaseResp.StatusMsg); err != nil {
		return nil, err
	}
	return decodeMiniMaxMusicResponse(&response, req.OutputFormat)
}

func (a *MusicAdapter) StreamEdit(ctx context.Context, req *modality.MusicEditRequest) (*modality.MusicStream, error) {
	return nil, httputil.NewError(http.StatusBadRequest, "capability_not_supported", "music_streaming_not_supported", "model", "MiniMax music streaming is not enabled in this Polaris build.")
}

func (a *MusicAdapter) SeparateStems(ctx context.Context, req *modality.MusicStemRequest) (*modality.MusicOperationResult, error) {
	return nil, httputil.NewError(http.StatusBadRequest, "capability_not_supported", "music_stems_not_supported", "model", "MiniMax music stems separation is not supported.")
}

func (a *MusicAdapter) GenerateLyrics(ctx context.Context, req *modality.MusicLyricsRequest) (*modality.MusicLyricsResponse, error) {
	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = "write_full_song"
	}
	payload := lyricsRequest{
		Mode:   mode,
		Prompt: req.Prompt,
		Lyrics: req.Lyrics,
		Title:  req.Title,
	}
	var response lyricsResponse
	if _, err := a.client.JSON(ctx, http.MethodPost, "/v1/lyrics_generation", payload, &response); err != nil {
		return nil, err
	}
	if err := minimaxBaseRespError(response.BaseResp.StatusCode, response.BaseResp.StatusMsg); err != nil {
		return nil, err
	}
	return &modality.MusicLyricsResponse{
		Title:     response.SongTitle,
		StyleTags: response.StyleTags,
		Lyrics:    response.Lyrics,
	}, nil
}

func (a *MusicAdapter) CreatePlan(ctx context.Context, req *modality.MusicPlanRequest) (*modality.MusicPlanResponse, error) {
	return nil, httputil.NewError(http.StatusBadRequest, "capability_not_supported", "composition_plans_not_supported", "model", "MiniMax music composition plans are not supported.")
}

func decodeMiniMaxMusicResponse(response *musicGenerateResponse, requestedFormat string) (*modality.MusicOperationResult, error) {
	if response == nil || strings.TrimSpace(response.Data.Audio) == "" {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "MiniMax returned an empty music response.")
	}
	raw, err := hex.DecodeString(strings.TrimSpace(response.Data.Audio))
	if err != nil {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "MiniMax returned invalid hex audio data.")
	}
	format := strings.ToLower(strings.TrimSpace(requestedFormat))
	if format == "" {
		format = "mp3"
	}
	return &modality.MusicOperationResult{
		Asset: &modality.MusicAsset{
			Data:        raw,
			ContentType: musicContentType(format),
			Filename:    "music." + musicFileExtension(format),
		},
		DurationMS:   response.ExtraInfo.MusicDuration,
		SampleRateHz: response.ExtraInfo.MusicSampleRate,
		Bitrate:      response.ExtraInfo.Bitrate,
		SizeBytes:    response.ExtraInfo.MusicSize,
	}, nil
}

func resolveMiniMaxReferenceAudio(req *modality.MusicEditRequest) (string, string, error) {
	switch {
	case strings.TrimSpace(req.SourceAudio) != "":
		value := strings.TrimSpace(req.SourceAudio)
		if strings.HasPrefix(strings.ToLower(value), "http://") || strings.HasPrefix(strings.ToLower(value), "https://") {
			return value, "", nil
		}
		if strings.HasPrefix(strings.ToLower(value), "data:") {
			comma := strings.Index(value, ",")
			if comma < 0 || comma == len(value)-1 {
				return "", "", httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_source_audio", "source_audio", "Field 'source_audio' must be a valid data URL.")
			}
			return "", value[comma+1:], nil
		}
		return "", value, nil
	case len(req.File) > 0:
		return "", base64.StdEncoding.EncodeToString(req.File), nil
	default:
		return "", "", httputil.NewError(http.StatusBadRequest, "invalid_request_error", "missing_source_audio", "source_audio", "MiniMax cover requires source audio from 'source_audio', an uploaded file, or a previous music job.")
	}
}

func minimaxAudioSetting(format string, sampleRateHz int, bitrate int) *audioSetting {
	setting := &audioSetting{}
	if sampleRateHz > 0 {
		setting.SampleRate = sampleRateHz
	}
	if bitrate > 0 {
		setting.Bitrate = bitrate
	}
	if trimmed := strings.ToLower(strings.TrimSpace(format)); trimmed != "" {
		setting.Format = trimmed
	}
	if setting.SampleRate == 0 && setting.Bitrate == 0 && setting.Format == "" {
		return nil
	}
	return setting
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

func musicContentType(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "wav":
		return "audio/wav"
	case "flac":
		return "audio/flac"
	default:
		return "audio/mpeg"
	}
}

func musicFileExtension(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "wav":
		return "wav"
	case "flac":
		return "flac"
	default:
		return "mp3"
	}
}

func minimaxBaseRespError(statusCode int, statusMsg string) error {
	if statusCode == 0 {
		return nil
	}
	if strings.Contains(strings.ToLower(strings.TrimSpace(statusMsg)), "invalid api key") {
		return httputil.ProviderAuthError("MiniMax", statusMsg)
	}
	return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_error", "", firstNonEmptyMiniMax(statusMsg, "MiniMax returned an error."))
}

func firstNonEmptyMiniMax(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
