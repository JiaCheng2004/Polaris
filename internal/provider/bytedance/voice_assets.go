package bytedance

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

const (
	bytedanceVoiceAssetListAction  = "BatchListMegaTTSTrainStatus"
	bytedanceVoiceAssetListVersion = "2025-05-21"
	defaultVoiceCloneEndpoint      = "https://openspeech.bytedance.com/api/v3/tts/voice_clone"
	defaultVoiceDesignEndpoint     = "https://openspeech.bytedance.com/api/v3/tts/voice_design"
	defaultVoiceUpgradeEndpoint    = "https://openspeech.bytedance.com/api/v3/tts/upgrade_voice"
	bytedanceSpeechSuccessCode     = 20000000
	bytedanceVoiceCloneReadyState  = "ready"
	bytedanceVoiceCloneActiveState = "active"
	bytedanceVoiceCloneFailedState = "failed"
	bytedanceVoiceCloneDeleteState = "deleted"
	bytedanceVoiceCloneTrainState  = "training"
	bytedanceVoiceCloneDraftState  = "draft"
)

type voiceAssetAdapter struct {
	client *Client
}

type voiceCloneRequest struct {
	SpeakerID   string          `json:"speaker_id"`
	Audio       voiceCloneAudio `json:"audio"`
	Language    *int            `json:"language,omitempty"`
	Text        string          `json:"text,omitempty"`
	ExtraParams map[string]any  `json:"extra_params,omitempty"`
}

type voiceCloneAudio struct {
	Data   string `json:"data"`
	Format string `json:"format"`
}

type voiceDesignRequest struct {
	SpeakerID string            `json:"speaker_id"`
	Text      string            `json:"text"`
	Prompt    voiceDesignPrompt `json:"prompt"`
}

type voiceDesignPrompt struct {
	TextPrompt string         `json:"text_prompt,omitempty"`
	Image      map[string]any `json:"image_prompt,omitempty"`
}

type voiceUpgradeRequest struct {
	SpeakerID string `json:"speaker_id"`
}

type bytedanceVoiceOperationResponse struct {
	Code                   int                    `json:"code"`
	Message                string                 `json:"message"`
	AvailableTrainingTimes int                    `json:"available_training_times"`
	CreateTime             int64                  `json:"create_time"`
	Language               any                    `json:"language"`
	SpeakerID              string                 `json:"speaker_id"`
	Status                 any                    `json:"status"`
	DemoAudio              string                 `json:"demo_audio"`
	SpeakerStatus          []bytedanceSpeakerDemo `json:"speaker_status"`
}

type bytedanceSpeakerDemo struct {
	DemoAudio string `json:"demo_audio"`
	ModelType any    `json:"model_type"`
}

type bytedanceVoiceAssetListRequest struct {
	ProjectName string   `json:"ProjectName"`
	State       string   `json:"State,omitempty"`
	SpeakerIDs  []string `json:"SpeakerIDs,omitempty"`
}

func NewVoiceAssetAdapter(client *Client) modality.VoiceAssetAdapter {
	return &voiceAssetAdapter{client: client}
}

func (a *voiceAssetAdapter) ListCustomVoices(ctx context.Context, req *modality.VoiceCatalogRequest) (*modality.VoiceCatalogResponse, error) {
	items, err := a.fetchCustomVoices(ctx, req.State, nil)
	if err != nil {
		return nil, err
	}
	if req.Limit > 0 && len(items) > req.Limit {
		items = items[:req.Limit]
	}
	return &modality.VoiceCatalogResponse{
		Object:   "list",
		Scope:    "provider",
		Provider: "bytedance",
		Data:     items,
	}, nil
}

func (a *voiceAssetAdapter) GetVoice(ctx context.Context, req *modality.VoiceLookupRequest) (*modality.VoiceCatalogItem, error) {
	items, err := a.fetchCustomVoices(ctx, "", []string{req.ID})
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.ID == req.ID {
			cloned := item
			return &cloned, nil
		}
	}
	return nil, httputil.NewError(http.StatusNotFound, "invalid_request_error", "voice_not_found", "id", "Voice was not found.")
}

func (a *voiceAssetAdapter) CreateClone(ctx context.Context, req *modality.VoiceCloneRequest) (*modality.VoiceCatalogItem, error) {
	audioData, err := decodeBase64Audio(req.Audio)
	if err != nil {
		return nil, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_audio", "audio", "Field 'audio' must be valid base64 audio.")
	}
	payload := voiceCloneRequest{
		SpeakerID: strings.TrimSpace(req.VoiceID),
		Audio: voiceCloneAudio{
			Data:   base64.StdEncoding.EncodeToString(audioData),
			Format: normalizeAudioFormat(req.AudioFormat),
		},
		Text: strings.TrimSpace(req.PromptText),
	}
	if code := bytedanceLanguageCode(req.Language); code != nil {
		payload.Language = code
	}
	if extra := cloneExtraParams(req); len(extra) > 0 {
		payload.ExtraParams = extra
	}

	var response bytedanceVoiceOperationResponse
	if err := a.postSpeechJSON(ctx, defaultVoiceCloneEndpoint, payload, &response); err != nil {
		return nil, err
	}
	return a.normalizeVoiceOperationResponse(response, "clone"), nil
}

func (a *voiceAssetAdapter) CreateDesign(ctx context.Context, req *modality.VoiceDesignRequest) (*modality.VoiceCatalogItem, error) {
	payload := voiceDesignRequest{
		SpeakerID: strings.TrimSpace(req.VoiceID),
		Text:      strings.TrimSpace(req.Text),
		Prompt: voiceDesignPrompt{
			TextPrompt: strings.TrimSpace(req.PromptText),
		},
	}
	if trimmed := strings.TrimSpace(req.PromptImageURL); trimmed != "" {
		payload.Prompt.Image = map[string]any{"image_url": trimmed}
	}

	var response bytedanceVoiceOperationResponse
	if err := a.postSpeechJSON(ctx, defaultVoiceDesignEndpoint, payload, &response); err != nil {
		return nil, err
	}
	return a.normalizeVoiceOperationResponse(response, "design"), nil
}

func (a *voiceAssetAdapter) RetrainVoice(ctx context.Context, req *modality.VoiceCloneRequest) (*modality.VoiceCatalogItem, error) {
	return a.CreateClone(ctx, req)
}

func (a *voiceAssetAdapter) ActivateVoice(ctx context.Context, req *modality.VoiceLookupRequest) (*modality.VoiceCatalogItem, error) {
	payload := voiceUpgradeRequest{SpeakerID: strings.TrimSpace(req.ID)}
	var response bytedanceVoiceOperationResponse
	if err := a.postSpeechJSON(ctx, defaultVoiceUpgradeEndpoint, payload, &response); err != nil {
		return nil, err
	}
	item := a.normalizeVoiceOperationResponse(response, "activate")
	item.State = bytedanceVoiceCloneActiveState
	return item, nil
}

func (a *voiceAssetAdapter) DeleteVoice(ctx context.Context, req *modality.VoiceLookupRequest) error {
	return httputil.NewError(http.StatusBadRequest, "capability_not_supported", "voice_delete_not_supported", "id", "ByteDance does not expose voice deletion through the current Polaris runtime.")
}

func (a *voiceAssetAdapter) fetchCustomVoices(ctx context.Context, state string, speakerIDs []string) ([]modality.VoiceCatalogItem, error) {
	states := bytedanceVoiceStates(state)
	itemsByID := map[string]modality.VoiceCatalogItem{}
	for _, candidate := range states {
		var raw map[string]any
		err := a.client.speechControlJSON(ctx, bytedanceVoiceAssetListAction, bytedanceVoiceAssetListVersion, bytedanceVoiceAssetListRequest{
			ProjectName: a.client.projectName,
			State:       candidate,
			SpeakerIDs:  speakerIDs,
		}, &raw)
		if err != nil {
			return nil, err
		}
		for _, entry := range findVoiceAssetEntries(raw) {
			item := normalizeVoiceAssetListEntry(entry)
			if strings.TrimSpace(item.ID) == "" {
				continue
			}
			if existing, ok := itemsByID[item.ID]; !ok || voiceStateRank(item.State) > voiceStateRank(existing.State) {
				itemsByID[item.ID] = item
			}
		}
	}

	items := make([]modality.VoiceCatalogItem, 0, len(itemsByID))
	for _, item := range itemsByID {
		items = append(items, item)
	}
	slices.SortFunc(items, func(aItem, bItem modality.VoiceCatalogItem) int {
		return strings.Compare(aItem.ID, bItem.ID)
	})
	return items, nil
}

func (a *voiceAssetAdapter) postSpeechJSON(ctx context.Context, endpoint string, body any, out any) error {
	if strings.TrimSpace(a.client.speechAPIKey) == "" {
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_misconfigured", "", "ByteDance voice asset APIs require providers.bytedance.speech_api_key.")
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal bytedance voice asset request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build bytedance voice asset request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Api-Key", a.client.speechAPIKey)
	req.Header.Set("X-Api-Request-Id", newRequestID())

	resp, err := a.client.httpClient.Do(req)
	if err != nil {
		return translateTransportError(err, "ByteDance")
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance returned an unreadable voice asset response.")
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return bytedanceSpeechJSONError(resp.StatusCode, raw)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance returned an invalid JSON voice asset response.")
	}
	if envelope, ok := out.(*bytedanceVoiceOperationResponse); ok && envelope.Code != 0 && envelope.Code != bytedanceSpeechSuccessCode {
		return bytedanceSpeechJSONCodeError(envelope.Code, envelope.Message)
	}
	return nil
}

func decodeBase64Audio(value string) ([]byte, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, fmt.Errorf("missing audio")
	}
	if strings.HasPrefix(trimmed, "data:") {
		if comma := strings.Index(trimmed, ","); comma >= 0 {
			trimmed = trimmed[comma+1:]
		}
	}
	data, err := base64.StdEncoding.DecodeString(trimmed)
	if err == nil {
		return data, nil
	}
	return base64.RawStdEncoding.DecodeString(trimmed)
}

func normalizeAudioFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "wav", "mp3", "aac", "flac", "ogg", "opus":
		return strings.ToLower(strings.TrimSpace(format))
	default:
		return "wav"
	}
}

func bytedanceLanguageCode(value string) *int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "zh", "zh-cn", "zh_cn":
		code := 0
		return &code
	case "en", "en-us", "en_us":
		code := 1
		return &code
	default:
		return nil
	}
}

func cloneExtraParams(req *modality.VoiceCloneRequest) map[string]any {
	extra := map[string]any{}
	if req.Denoise != nil {
		extra["enable_audio_denoise"] = *req.Denoise
	}
	if req.CheckPromptTextQuality != nil {
		extra["enable_check_prompt_text_quality"] = *req.CheckPromptTextQuality
	}
	if req.CheckAudioQuality != nil {
		extra["enable_check_audio_quality"] = *req.CheckAudioQuality
	}
	if req.EnableSourceSeparation != nil {
		extra["voice_clone_enable_mss"] = *req.EnableSourceSeparation
	}
	if trimmed := strings.TrimSpace(req.DenoiseModel); trimmed != "" {
		extra["voice_clone_denoise_model_id"] = trimmed
	}
	return extra
}

func bytedanceVoiceStates(filter string) []string {
	if trimmed := strings.TrimSpace(filter); trimmed != "" {
		return []string{trimmed}
	}
	return []string{"Training", "Success", "Active", "Expired", "Reclaimed"}
}

func findVoiceAssetEntries(node any) []map[string]any {
	var out []map[string]any
	switch typed := node.(type) {
	case map[string]any:
		if _, ok := typed["SpeakerID"]; ok {
			out = append(out, typed)
		}
		for _, value := range typed {
			out = append(out, findVoiceAssetEntries(value)...)
		}
	case []any:
		for _, value := range typed {
			out = append(out, findVoiceAssetEntries(value)...)
		}
	}
	return out
}

func normalizeVoiceAssetListEntry(entry map[string]any) modality.VoiceCatalogItem {
	item := modality.VoiceCatalogItem{
		ID:       firstMapString(entry, "SpeakerID", "speaker_id"),
		Provider: "bytedance",
		Type:     "clone",
		State:    normalizeVoiceState(firstMapString(entry, "State", "state")),
		Name:     firstMapString(entry, "Alias", "alias"),
	}
	if item.Name == "" {
		item.Name = item.ID
	}
	item.PreviewURL = firstMapString(entry, "DemoAudio", "demo_audio")
	item.Metadata = map[string]any{}
	copyMapField(item.Metadata, entry, "InstanceNO")
	copyMapField(item.Metadata, entry, "Version")
	copyMapField(item.Metadata, entry, "CreateTime")
	copyMapField(item.Metadata, entry, "ExpireTime")
	copyMapField(item.Metadata, entry, "OrderTime")
	copyMapField(item.Metadata, entry, "IsActivable")
	copyMapField(item.Metadata, entry, "AvailableTrainingTimes")
	return item
}

func (a *voiceAssetAdapter) normalizeVoiceOperationResponse(response bytedanceVoiceOperationResponse, kind string) *modality.VoiceCatalogItem {
	item := &modality.VoiceCatalogItem{
		ID:         strings.TrimSpace(response.SpeakerID),
		Provider:   "bytedance",
		Type:       kind,
		State:      normalizeVoiceStatusCode(response.Status),
		PreviewURL: strings.TrimSpace(response.DemoAudio),
		Metadata:   map[string]any{},
	}
	if len(response.SpeakerStatus) > 0 && item.PreviewURL == "" {
		item.PreviewURL = strings.TrimSpace(response.SpeakerStatus[0].DemoAudio)
	}
	item.Metadata["available_training_times"] = response.AvailableTrainingTimes
	if response.CreateTime > 0 {
		item.Metadata["create_time"] = response.CreateTime
		item.Metadata["created_at"] = time.UnixMilli(response.CreateTime).UTC().Format(time.RFC3339)
	}
	if response.Language != nil {
		item.Metadata["language"] = response.Language
	}
	item.Name = item.ID
	if item.State == "" {
		item.State = bytedanceVoiceCloneTrainState
	}
	return item
}

func normalizeVoiceState(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "training":
		return bytedanceVoiceCloneTrainState
	case "success":
		return bytedanceVoiceCloneReadyState
	case "active":
		return bytedanceVoiceCloneActiveState
	case "expired":
		return bytedanceVoiceCloneFailedState
	case "reclaimed":
		return bytedanceVoiceCloneDeleteState
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func normalizeVoiceStatusCode(value any) string {
	switch typed := value.(type) {
	case string:
		return normalizeVoiceState(typed)
	case float64:
		return normalizeVoiceStatusNumber(int(typed))
	case int:
		return normalizeVoiceStatusNumber(typed)
	case json.Number:
		if parsed, err := typed.Int64(); err == nil {
			return normalizeVoiceStatusNumber(int(parsed))
		}
	}
	return bytedanceVoiceCloneTrainState
}

func normalizeVoiceStatusNumber(value int) string {
	switch value {
	case 0:
		return bytedanceVoiceCloneDraftState
	case 1:
		return bytedanceVoiceCloneTrainState
	case 2:
		return bytedanceVoiceCloneReadyState
	case 3:
		return bytedanceVoiceCloneActiveState
	case 4:
		return bytedanceVoiceCloneFailedState
	case 5:
		return bytedanceVoiceCloneDeleteState
	default:
		return bytedanceVoiceCloneTrainState
	}
}

func voiceStateRank(value string) int {
	switch value {
	case bytedanceVoiceCloneActiveState:
		return 4
	case bytedanceVoiceCloneReadyState:
		return 3
	case bytedanceVoiceCloneTrainState:
		return 2
	case bytedanceVoiceCloneDraftState:
		return 1
	case bytedanceVoiceCloneFailedState, bytedanceVoiceCloneDeleteState:
		return 0
	default:
		return 0
	}
}

func firstMapString(entry map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := entry[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			if trimmed := strings.TrimSpace(typed); trimmed != "" {
				return trimmed
			}
		case json.Number:
			return typed.String()
		case float64:
			return strconv.FormatFloat(typed, 'f', -1, 64)
		case int:
			return strconv.Itoa(typed)
		case fmt.Stringer:
			if trimmed := strings.TrimSpace(typed.String()); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func copyMapField(target map[string]any, entry map[string]any, key string) {
	if value, ok := entry[key]; ok && value != nil {
		target[strings.ToLower(key)] = value
	}
}

func bytedanceSpeechJSONError(status int, raw []byte) error {
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err == nil {
		if code, ok := envelope["code"]; ok {
			if numeric := parseLooseInt(code); numeric != 0 {
				return bytedanceSpeechJSONCodeError(numeric, firstMapString(envelope, "message", "Message"))
			}
		}
	}
	return httputil.ProviderAPIError("ByteDance", status, httputil.ProviderErrorDetails{
		Message: strings.TrimSpace(string(raw)),
		Body:    string(raw),
	})
}

func bytedanceSpeechJSONCodeError(code int, message string) error {
	message = firstNonEmpty(strings.TrimSpace(message), "ByteDance returned a speech API error.")
	switch code {
	case 45001107:
		return httputil.NewError(http.StatusNotFound, "invalid_request_error", "voice_not_found", "id", message)
	case 45001123:
		return httputil.NewError(http.StatusConflict, "invalid_request_error", "voice_limit_reached", "", message)
	default:
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_error", "", message)
	}
}

func parseLooseInt(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case int64:
		return int(typed)
	case json.Number:
		if parsed, err := typed.Int64(); err == nil {
			return int(parsed)
		}
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
			return parsed
		}
	}
	return 0
}
