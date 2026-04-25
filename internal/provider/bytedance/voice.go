package bytedance

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

const (
	defaultTTSEndpoint      = "https://openspeech.bytedance.com/api/v3/tts/unidirectional/sse"
	defaultSTTEndpoint      = "https://openspeech.bytedance.com/api/v3/auc/bigmodel/recognize/flash"
	bytedanceTTSSuccessCode = 20000000
	bytedanceTTSResource2ID = "seed-tts-2.0"
	bytedanceTTSResource1ID = "seed-tts-1.0"
	bytedanceSTTResourceID  = "volc.bigasr.auc_turbo"
	bytedanceSTTSuccessCode = "20000000"
)

type VoiceAdapter struct {
	client   *Client
	model    string
	endpoint string
}

type ttsRequest struct {
	User      ttsUserConfig   `json:"user"`
	ReqParams ttsQueryRequest `json:"req_params"`
}

type ttsUserConfig struct {
	UID string `json:"uid"`
}

type ttsAudioConfig struct {
	Format     string `json:"format,omitempty"`
	SampleRate int    `json:"sample_rate,omitempty"`
	SpeechRate int    `json:"speech_rate,omitempty"`
}

type ttsQueryRequest struct {
	Text        string         `json:"text"`
	Speaker     string         `json:"speaker"`
	AudioParams ttsAudioConfig `json:"audio_params"`
}

type ttsResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data"`
}

type sttRequest struct {
	User    sttUserConfig   `json:"user"`
	Audio   sttAudioConfig  `json:"audio"`
	Request sttQueryRequest `json:"request"`
}

type sttUserConfig struct {
	UID string `json:"uid"`
}

type sttAudioConfig struct {
	Data string `json:"data,omitempty"`
}

type sttQueryRequest struct {
	ModelName string `json:"model_name"`
}

type sttResponse struct {
	AudioInfo sttAudioInfo `json:"audio_info"`
	Result    sttResult    `json:"result"`
}

type sttAudioInfo struct {
	Duration float64 `json:"duration"`
}

type sttResult struct {
	Text       string         `json:"text"`
	Utterances []sttUtterance `json:"utterances"`
}

type sttUtterance struct {
	StartTime int    `json:"start_time"`
	EndTime   int    `json:"end_time"`
	Text      string `json:"text"`
	Definite  bool   `json:"definite"`
}

func NewVoiceAdapter(client *Client, model string, endpoint string) *VoiceAdapter {
	return &VoiceAdapter{
		client:   client,
		model:    model,
		endpoint: endpoint,
	}
}

func (a *VoiceAdapter) TextToSpeech(ctx context.Context, req *modality.TTSRequest) (*modality.AudioResponse, error) {
	if strings.TrimSpace(a.client.speechAPIKey) == "" {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_misconfigured", "", "ByteDance TTS requires providers.bytedance.speech_api_key.")
	}

	encoding, contentType, err := bytedanceEncoding(req.ResponseFormat)
	if err != nil {
		return nil, err
	}

	audioParams := ttsAudioConfig{
		Format:     encoding,
		SampleRate: bytedanceSampleRate(encoding),
	}
	if req.Speed != nil {
		audioParams.SpeechRate = bytedanceSpeechRate(*req.Speed)
	}

	payload := ttsRequest{
		User: ttsUserConfig{
			UID: newRequestID(),
		},
		ReqParams: ttsQueryRequest{
			Text:        req.Input,
			Speaker:     req.Voice,
			AudioParams: audioParams,
		},
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal bytedance tts request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.ttsURL(), bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("build bytedance tts request: %w", err)
	}
	httpReq.Header.Set("X-Api-Key", a.client.speechAPIKey)
	httpReq.Header.Set("X-Api-Resource-Id", bytedanceTTSResourceID(req.Model, a.model))
	httpReq.Header.Set("X-Control-Require-Usage-Tokens-Return", "text_words")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := a.client.httpClient.Do(httpReq)
	if err != nil {
		return nil, translateTransportError(err, "ByteDance")
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance returned an invalid voice response.")
	}
	if resp.StatusCode >= http.StatusBadRequest {
		var parsed ttsResponse
		if err := json.Unmarshal(body, &parsed); err == nil {
			return nil, bytedanceVoiceError(resp.StatusCode, parsed.Code, parsed.Message)
		}
		return nil, bytedanceVoiceError(resp.StatusCode, 0, strings.TrimSpace(string(body)))
	}

	audioData, err := bytedanceDecodeTTSStream(body)
	if err != nil {
		return nil, err
	}
	if len(audioData) == 0 {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance returned an empty audio payload.")
	}

	return &modality.AudioResponse{
		Data:        audioData,
		ContentType: contentType,
	}, nil
}

func (a *VoiceAdapter) SpeechToText(ctx context.Context, req *modality.STTRequest) (*modality.TranscriptResponse, error) {
	if strings.TrimSpace(a.client.speechAPIKey) == "" {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_misconfigured", "", "ByteDance STT requires providers.bytedance.speech_api_key.")
	}

	requestID := newRequestID()
	payload := sttRequest{
		User: sttUserConfig{
			UID: requestID,
		},
		Audio: sttAudioConfig{
			Data: base64.StdEncoding.EncodeToString(req.File),
		},
		Request: sttQueryRequest{
			ModelName: bytedanceSTTModelName(req.Model, a.model),
		},
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal bytedance stt request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.sttURL(), bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("build bytedance stt request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("X-Api-Key", a.client.speechAPIKey)
	httpReq.Header.Set("X-Api-Resource-Id", bytedanceSTTResourceID)
	httpReq.Header.Set("X-Api-Request-Id", requestID)
	httpReq.Header.Set("X-Api-Sequence", "-1")

	resp, err := a.client.httpClient.Do(httpReq)
	if err != nil {
		return nil, translateTransportError(err, "ByteDance")
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance returned an invalid transcription response.")
	}

	apiStatus := strings.TrimSpace(resp.Header.Get("X-Api-Status-Code"))
	apiMessage := strings.TrimSpace(resp.Header.Get("X-Api-Message"))
	if resp.StatusCode >= http.StatusBadRequest || apiStatus != "" && apiStatus != bytedanceSTTSuccessCode {
		return nil, bytedanceSTTError(resp.StatusCode, apiStatus, apiMessage, body)
	}

	var parsed sttResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance returned an invalid STT JSON response.")
	}

	response := normalizeBytedanceTranscript(parsed, req)
	return response, nil
}

func (a *VoiceAdapter) ttsURL() string {
	if trimmed := strings.TrimSpace(a.endpoint); trimmed != "" {
		return trimmed
	}
	return defaultTTSEndpoint
}

func (a *VoiceAdapter) sttURL() string {
	if trimmed := strings.TrimSpace(a.endpoint); trimmed != "" {
		return trimmed
	}
	return defaultSTTEndpoint
}

func bytedanceEncoding(format string) (string, string, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "mp3":
		return "mp3", "audio/mpeg", nil
	case "opus":
		return "ogg_opus", "audio/ogg", nil
	case "pcm":
		return "pcm", "audio/pcm", nil
	default:
		return "", "", httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_response_format", "response_format", "Requested response format is not supported by ByteDance TTS 2.0.")
	}
}

func bytedanceVoiceError(status int, code int, message string) error {
	if strings.TrimSpace(message) == "" {
		message = "ByteDance returned an error."
	}

	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_auth_failed", "", message)
	case status == http.StatusTooManyRequests:
		return httputil.NewError(http.StatusTooManyRequests, "rate_limit_error", "provider_rate_limit", "", message)
	case status >= http.StatusInternalServerError:
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_server_error", "", message)
	case code >= 3001 && code < 4000:
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "provider_bad_request", "", message)
	default:
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_error", "", message)
	}
}

func bytedanceSTTError(status int, apiStatus string, message string, body []byte) error {
	if strings.TrimSpace(message) == "" {
		message = strings.TrimSpace(string(body))
	}
	if strings.TrimSpace(message) == "" {
		message = "ByteDance returned an STT error."
	}

	switch apiStatus {
	case "20000003":
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "silent_audio", "file", message)
	case "45000001":
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "provider_bad_request", "file", message)
	case "45000002":
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "empty_audio", "file", message)
	case "45000151":
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "unsupported_audio_format", "file", message)
	case "55000031":
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_server_busy", "", message)
	}

	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_auth_failed", "", message)
	}
	if status == http.StatusTooManyRequests {
		return httputil.NewError(http.StatusTooManyRequests, "rate_limit_error", "provider_rate_limit", "", message)
	}
	if strings.HasPrefix(apiStatus, "55") || status >= http.StatusInternalServerError {
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_server_error", "", message)
	}
	return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_error", "", message)
}

func normalizeBytedanceTranscript(parsed sttResponse, req *modality.STTRequest) *modality.TranscriptResponse {
	segments := make([]modality.TranscriptSegment, 0, len(parsed.Result.Utterances))
	for index, utterance := range parsed.Result.Utterances {
		segments = append(segments, modality.TranscriptSegment{
			ID:    index,
			Start: float64(utterance.StartTime) / 1000,
			End:   float64(utterance.EndTime) / 1000,
			Text:  utterance.Text,
		})
	}

	if len(segments) == 0 && strings.TrimSpace(parsed.Result.Text) != "" {
		segments = append(segments, modality.TranscriptSegment{
			ID:    0,
			Start: 0,
			End:   parsed.AudioInfo.Duration / 1000,
			Text:  parsed.Result.Text,
		})
	}

	response := &modality.TranscriptResponse{
		Text:     parsed.Result.Text,
		Language: strings.TrimSpace(req.Language),
		Duration: parsed.AudioInfo.Duration / 1000,
		Segments: segments,
		Format:   req.ResponseFormat,
	}

	switch strings.ToLower(strings.TrimSpace(req.ResponseFormat)) {
	case "text":
		response.Raw = []byte(parsed.Result.Text)
		response.ContentType = "text/plain; charset=utf-8"
	case "srt":
		response.Raw = []byte(bytedanceSRT(parsed.Result.Text, parsed.Result.Utterances, parsed.AudioInfo.Duration))
		response.ContentType = "application/x-subrip; charset=utf-8"
	case "vtt":
		response.Raw = []byte(bytedanceVTT(parsed.Result.Text, parsed.Result.Utterances, parsed.AudioInfo.Duration))
		response.ContentType = "text/vtt; charset=utf-8"
	default:
		response.ContentType = "application/json"
	}

	return response
}

func bytedanceSRT(fullText string, utterances []sttUtterance, durationMs float64) string {
	var builder strings.Builder
	items := subtitleUtterances(fullText, utterances, int(durationMs))
	for index, utterance := range items {
		builder.WriteString(strconv.Itoa(index + 1))
		builder.WriteString("\n")
		builder.WriteString(formatSubtitleTime(utterance.StartTime, false))
		builder.WriteString(" --> ")
		builder.WriteString(formatSubtitleTime(utterance.EndTime, false))
		builder.WriteString("\n")
		builder.WriteString(strings.TrimSpace(utterance.Text))
		builder.WriteString("\n\n")
	}
	return builder.String()
}

func bytedanceVTT(fullText string, utterances []sttUtterance, durationMs float64) string {
	var builder strings.Builder
	builder.WriteString("WEBVTT\n\n")
	items := subtitleUtterances(fullText, utterances, int(durationMs))
	for _, utterance := range items {
		builder.WriteString(formatSubtitleTime(utterance.StartTime, true))
		builder.WriteString(" --> ")
		builder.WriteString(formatSubtitleTime(utterance.EndTime, true))
		builder.WriteString("\n")
		builder.WriteString(strings.TrimSpace(utterance.Text))
		builder.WriteString("\n\n")
	}
	return builder.String()
}

func subtitleUtterances(fullText string, utterances []sttUtterance, durationMs int) []sttUtterance {
	if len(utterances) > 0 {
		return utterances
	}
	if strings.TrimSpace(fullText) == "" {
		return nil
	}
	return []sttUtterance{{
		StartTime: 0,
		EndTime:   durationMs,
		Text:      fullText,
	}}
}

func formatSubtitleTime(ms int, vtt bool) string {
	if ms < 0 {
		ms = 0
	}
	hours := ms / 3600000
	minutes := (ms % 3600000) / 60000
	seconds := (ms % 60000) / 1000
	millis := ms % 1000
	separator := ","
	if vtt {
		separator = "."
	}
	return fmt.Sprintf("%02d:%02d:%02d%s%03d", hours, minutes, seconds, separator, millis)
}

func bytedanceSTTModelName(requestModel string, fallbackModel string) string {
	switch providerModelName(requestModel, fallbackModel) {
	case "", "doubao-asr-flash", "doubao-stt-flash", "doubao-asr-2.0", "doubao-recording-asr-2.0":
		return "bigmodel"
	default:
		return providerModelName(requestModel, fallbackModel)
	}
}

func bytedanceDecodeTTSStream(body []byte) ([]byte, error) {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	audio := make([]byte, 0, len(body))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}

		var parsed ttsResponse
		if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
			return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance returned an invalid TTS stream event.")
		}
		if parsed.Code != 0 && parsed.Code != bytedanceTTSSuccessCode {
			return nil, bytedanceVoiceError(http.StatusOK, parsed.Code, parsed.Message)
		}
		if strings.TrimSpace(parsed.Data) == "" {
			continue
		}

		chunk, err := base64.StdEncoding.DecodeString(parsed.Data)
		if err != nil {
			return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance returned invalid base64 audio data.")
		}
		audio = append(audio, chunk...)
	}

	if err := scanner.Err(); err != nil {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance returned an unreadable TTS stream.")
	}
	return audio, nil
}

func bytedanceTTSResourceID(requestModel string, fallbackModel string) string {
	switch providerModelName(requestModel, fallbackModel) {
	case "", "doubao-tts", "doubao-tts-2.0", "seed-tts-2.0":
		return bytedanceTTSResource2ID
	case "doubao-tts-1.0", "seed-tts-1.0":
		return bytedanceTTSResource1ID
	default:
		return bytedanceTTSResource2ID
	}
}

func bytedanceSampleRate(format string) int {
	switch format {
	case "pcm":
		return 16000
	default:
		return 24000
	}
}

func bytedanceSpeechRate(speed float64) int {
	if speed <= 0 {
		return 0
	}
	value := int((speed - 1.0) * 100)
	if value < -50 {
		return -50
	}
	if value > 100 {
		return 100
	}
	return value
}

func newRequestID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "polaris-request"
	}
	return hex.EncodeToString(raw[:])
}
