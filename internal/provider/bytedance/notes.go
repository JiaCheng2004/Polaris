package bytedance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

const (
	defaultNotesSubmitEndpoint = "https://openspeech.bytedance.com/api/v3/auc/lark/submit"
	defaultNotesQueryEndpoint  = "https://openspeech.bytedance.com/api/v3/auc/lark/query"
	bytedanceNotesResourceID   = "volc.lark.minutes"
	bytedanceNotesSuccessCode  = 0
)

type audioNotesAdapter struct {
	client    *Client
	model     string
	submitURL string
	queryURL  string
}

type bytedanceNotesSubmitRequest struct {
	Input  bytedanceNotesInput  `json:"Input"`
	Params bytedanceNotesParams `json:"Params"`
}

type bytedanceNotesInput struct {
	Offline bytedanceNotesOfflineInput `json:"Offline"`
}

type bytedanceNotesOfflineInput struct {
	FileURL  string `json:"FileURL"`
	FileType string `json:"FileType"`
}

type bytedanceNotesParams struct {
	AudioTranscriptionEnable     bool                               `json:"AudioTranscriptionEnable"`
	SourceLang                   string                             `json:"SourceLang,omitempty"`
	TranslationEnable            bool                               `json:"TranslationEnable,omitempty"`
	TranslationParams            *bytedanceNotesTranslationParams   `json:"TranslationParams,omitempty"`
	InformationExtractionEnabled bool                               `json:"InformationExtractionEnabled,omitempty"`
	InformationExtractionParams  *bytedanceNotesExtractionParams    `json:"InformationExtractionParams,omitempty"`
	SummarizationEnabled         bool                               `json:"SummarizationEnabled,omitempty"`
	SummarizationParams          *bytedanceNotesSummarizationParams `json:"SummarizationParams,omitempty"`
	ChapterEnabled               bool                               `json:"ChapterEnabled,omitempty"`
}

type bytedanceNotesTranslationParams struct {
	TargetLang string `json:"TargetLang"`
}

type bytedanceNotesExtractionParams struct {
	Types []string `json:"Types"`
}

type bytedanceNotesSummarizationParams struct {
	Types []string `json:"Types"`
}

type bytedanceNotesEnvelope struct {
	Code    int                     `json:"Code"`
	Message string                  `json:"Message"`
	Data    bytedanceNotesTaskState `json:"Data"`
}

type bytedanceNotesTaskState struct {
	TaskID     string                  `json:"TaskID"`
	Status     string                  `json:"Status"`
	ErrCode    int                     `json:"ErrCode"`
	ErrMessage string                  `json:"ErrMessage"`
	Result     bytedanceNotesTaskFiles `json:"Result"`
}

type bytedanceNotesTaskFiles struct {
	AudioTranscriptionFile    string `json:"AudioTranscriptionFile"`
	ChapterFile               string `json:"ChapterFile"`
	InformationExtractionFile string `json:"InformationExtractionFile"`
	SummarizationFile         string `json:"SummarizationFile"`
	TranslationFile           string `json:"TranslationFile"`
}

func NewAudioNotesAdapter(client *Client, model string, endpoint string) modality.AudioNotesAdapter {
	submitURL := strings.TrimSpace(endpoint)
	queryURL := defaultNotesQueryEndpoint
	if submitURL == "" {
		submitURL = defaultNotesSubmitEndpoint
	}
	return &audioNotesAdapter{
		client:    client,
		model:     model,
		submitURL: submitURL,
		queryURL:  queryURL,
	}
}

func (a *audioNotesAdapter) SubmitNotes(ctx context.Context, req *modality.AudioNoteRequest) (*modality.AudioNoteJob, error) {
	if strings.TrimSpace(a.client.appID) == "" || strings.TrimSpace(a.client.speechToken) == "" {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_misconfigured", "", "ByteDance audio notes require providers.bytedance.app_id and providers.bytedance.speech_access_token.")
	}

	body := bytedanceNotesSubmitRequest{
		Input: bytedanceNotesInput{
			Offline: bytedanceNotesOfflineInput{
				FileURL:  strings.TrimSpace(req.SourceURL),
				FileType: normalizeNotesFileType(req.FileType),
			},
		},
		Params: bytedanceNotesParams{
			AudioTranscriptionEnable: true,
			SourceLang:               normalizeNotesLanguage(req.Language),
			ChapterEnabled:           req.IncludeChapters,
		},
	}
	if target := normalizeNotesLanguage(req.TargetLanguage); target != "" {
		body.Params.TranslationEnable = true
		body.Params.TranslationParams = &bytedanceNotesTranslationParams{TargetLang: target}
	}
	if req.IncludeActionItems || req.IncludeQAPairs {
		types := make([]string, 0, 2)
		if req.IncludeActionItems {
			types = append(types, "todo_list")
		}
		if req.IncludeQAPairs {
			types = append(types, "question_answer")
		}
		body.Params.InformationExtractionEnabled = true
		body.Params.InformationExtractionParams = &bytedanceNotesExtractionParams{Types: types}
	}
	if req.IncludeSummary {
		body.Params.SummarizationEnabled = true
		body.Params.SummarizationParams = &bytedanceNotesSummarizationParams{Types: []string{"summary"}}
	}

	var envelope bytedanceNotesEnvelope
	if err := a.postNotesJSON(ctx, a.submitURL, body, &envelope); err != nil {
		return nil, err
	}
	return &modality.AudioNoteJob{
		ID:     strings.TrimSpace(envelope.Data.TaskID),
		Object: "audio.note",
		Model:  firstNonEmpty(req.Model, a.model),
		Status: normalizeNotesStatus(envelope.Data.Status),
	}, nil
}

func (a *audioNotesAdapter) GetAudioNote(ctx context.Context, req *modality.AudioNoteStatusRequest) (*modality.AudioNoteJob, error) {
	if strings.TrimSpace(a.client.appID) == "" || strings.TrimSpace(a.client.speechToken) == "" {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_misconfigured", "", "ByteDance audio notes require providers.bytedance.app_id and providers.bytedance.speech_access_token.")
	}

	var envelope bytedanceNotesEnvelope
	if err := a.postNotesJSON(ctx, a.queryURL, map[string]string{"TaskID": strings.TrimSpace(req.TaskID)}, &envelope); err != nil {
		return nil, err
	}
	job := &modality.AudioNoteJob{
		ID:     strings.TrimSpace(envelope.Data.TaskID),
		Object: "audio.note",
		Model:  firstNonEmpty(req.Model, a.model),
		Status: normalizeNotesStatus(envelope.Data.Status),
	}
	if envelope.Data.ErrCode != 0 {
		job.Error = &modality.AudioError{
			Code:    fmt.Sprintf("%d", envelope.Data.ErrCode),
			Message: firstNonEmpty(strings.TrimSpace(envelope.Data.ErrMessage), "ByteDance audio notes failed."),
		}
	}
	if job.Status == modality.AudioNoteStatusSuccess {
		result, err := a.fetchAudioNoteResult(ctx, envelope.Data.Result)
		if err != nil {
			return nil, err
		}
		job.Result = result
	}
	return job, nil
}

func (a *audioNotesAdapter) postNotesJSON(ctx context.Context, endpoint string, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal bytedance audio notes request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build bytedance audio notes request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Api-App-Key", a.client.appID)
	req.Header.Set("X-Api-Access-Key", a.client.speechToken)
	req.Header.Set("X-Api-Resource-Id", bytedanceNotesResourceID)
	req.Header.Set("X-Api-Request-Id", newRequestID())
	req.Header.Set("X-Api-Sequence", "-1")

	resp, err := a.client.httpClient.Do(req)
	if err != nil {
		return translateTransportError(err, "ByteDance")
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance audio notes returned an unreadable response.")
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return httputil.ProviderAPIError("ByteDance", resp.StatusCode, httputil.ProviderErrorDetails{
			Message: strings.TrimSpace(string(raw)),
			Body:    string(raw),
		})
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance audio notes returned an invalid JSON response.")
	}
	if envelope, ok := out.(*bytedanceNotesEnvelope); ok && envelope.Code != bytedanceNotesSuccessCode {
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_error", "", firstNonEmpty(strings.TrimSpace(envelope.Message), "ByteDance audio notes request failed."))
	}
	return nil
}

func (a *audioNotesAdapter) fetchAudioNoteResult(ctx context.Context, files bytedanceNotesTaskFiles) (*modality.AudioNoteResult, error) {
	result := &modality.AudioNoteResult{
		Metadata: map[string]any{},
	}
	var transcriptPayload any
	if trimmed := strings.TrimSpace(files.AudioTranscriptionFile); trimmed != "" {
		if err := a.fetchJSONURL(ctx, trimmed, &transcriptPayload); err != nil {
			return nil, err
		}
		result.Transcript = notesTranscriptText(transcriptPayload)
		result.Metadata["transcript_raw"] = transcriptPayload
	}
	if trimmed := strings.TrimSpace(files.ChapterFile); trimmed != "" {
		var chapterPayload any
		if err := a.fetchJSONURL(ctx, trimmed, &chapterPayload); err != nil {
			return nil, err
		}
		result.Chapters = notesChapters(chapterPayload)
		result.Metadata["chapters_raw"] = chapterPayload
	}
	if trimmed := strings.TrimSpace(files.InformationExtractionFile); trimmed != "" {
		var extractionPayload any
		if err := a.fetchJSONURL(ctx, trimmed, &extractionPayload); err != nil {
			return nil, err
		}
		result.ActionItems = notesActionItems(extractionPayload)
		result.QAPairs = notesQAPairs(extractionPayload)
		result.Metadata["information_extraction_raw"] = extractionPayload
	}
	if trimmed := strings.TrimSpace(files.SummarizationFile); trimmed != "" {
		var summaryPayload any
		if err := a.fetchJSONURL(ctx, trimmed, &summaryPayload); err != nil {
			return nil, err
		}
		result.Summary = notesSummary(summaryPayload)
		result.Metadata["summary_raw"] = summaryPayload
	}
	if trimmed := strings.TrimSpace(files.TranslationFile); trimmed != "" {
		var translationPayload any
		if err := a.fetchJSONURL(ctx, trimmed, &translationPayload); err != nil {
			return nil, err
		}
		result.Translation = notesSummary(translationPayload)
		result.Metadata["translation_raw"] = translationPayload
	}
	return result, nil
}

func (a *audioNotesAdapter) fetchJSONURL(ctx context.Context, rawURL string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("build bytedance audio notes asset request: %w", err)
	}
	resp, err := a.client.httpClient.Do(req)
	if err != nil {
		return translateTransportError(err, "ByteDance")
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= http.StatusBadRequest {
		return httputil.ProviderAPIError("ByteDance", resp.StatusCode, httputil.ProviderErrorDetails{
			Message: "ByteDance audio notes asset download failed.",
		})
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func normalizeNotesFileType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "video":
		return "video"
	default:
		return "audio"
	}
}

func normalizeNotesLanguage(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "zh", "zh-cn", "zh_cn":
		return "zh_cn"
	case "en", "en-us", "en_us":
		return "en_us"
	default:
		return ""
	}
}

func normalizeNotesStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "running":
		return modality.AudioNoteStatusRunning
	case "success":
		return modality.AudioNoteStatusSuccess
	case "failed":
		return modality.AudioNoteStatusFailed
	default:
		return modality.AudioNoteStatusQueued
	}
}

func notesTranscriptText(payload any) string {
	switch typed := payload.(type) {
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if entry, ok := item.(map[string]any); ok {
				if text := firstMapString(entry, "content", "text"); text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.TrimSpace(strings.Join(parts, " "))
	case map[string]any:
		return firstMapString(typed, "content", "text", "summary")
	default:
		return ""
	}
}

func notesChapters(payload any) []modality.AudioNoteChapter {
	entries := flattenObjectArray(payload)
	items := make([]modality.AudioNoteChapter, 0, len(entries))
	for _, entry := range entries {
		title := firstMapString(entry, "title", "summary", "content")
		if title == "" {
			continue
		}
		items = append(items, modality.AudioNoteChapter{
			Title: title,
			Start: looseFloat(entry["start_time"]),
			End:   looseFloat(entry["end_time"]),
			Text:  firstMapString(entry, "content", "text"),
		})
	}
	return items
}

func notesActionItems(payload any) []modality.AudioNoteActionItem {
	entries := extractNamedArray(payload, "todo_list")
	items := make([]modality.AudioNoteActionItem, 0, len(entries))
	for _, entry := range entries {
		items = append(items, modality.AudioNoteActionItem{
			Content:   firstMapString(entry, "content", "text", "summary"),
			Executor:  looseStringSlice(entry["executor"]),
			Due:       looseStringSlice(entry["deadline"]),
			StartTime: looseFloat(entry["start_time"]),
		})
	}
	return items
}

func notesQAPairs(payload any) []modality.AudioNoteQAPair {
	entries := extractNamedArray(payload, "question_answer", "qa_pairs", "question")
	items := make([]modality.AudioNoteQAPair, 0, len(entries))
	for _, entry := range entries {
		question := firstMapString(entry, "question", "content")
		answer := firstMapString(entry, "answer", "summary")
		if question == "" && answer == "" {
			continue
		}
		items = append(items, modality.AudioNoteQAPair{
			Question: question,
			Answer:   answer,
		})
	}
	return items
}

func notesSummary(payload any) string {
	switch typed := payload.(type) {
	case map[string]any:
		return firstMapString(typed, "summary", "content", "text")
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if entry, ok := item.(map[string]any); ok {
				if text := firstMapString(entry, "summary", "content", "text"); text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.TrimSpace(strings.Join(parts, " "))
	default:
		return ""
	}
}

func flattenObjectArray(payload any) []map[string]any {
	switch typed := payload.(type) {
	case []any:
		items := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if entry, ok := item.(map[string]any); ok {
				items = append(items, entry)
			}
		}
		return items
	case map[string]any:
		for _, value := range typed {
			if nested := flattenObjectArray(value); len(nested) > 0 {
				return nested
			}
		}
	}
	return nil
}

func extractNamedArray(payload any, names ...string) []map[string]any {
	switch typed := payload.(type) {
	case map[string]any:
		for _, name := range names {
			if value, ok := typed[name]; ok {
				return flattenObjectArray(value)
			}
		}
		for _, value := range typed {
			if nested := extractNamedArray(value, names...); len(nested) > 0 {
				return nested
			}
		}
	case []any:
		for _, value := range typed {
			if nested := extractNamedArray(value, names...); len(nested) > 0 {
				return nested
			}
		}
	}
	return nil
}

func looseStringSlice(value any) []string {
	switch typed := value.(type) {
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" && text != "<nil>" {
				items = append(items, text)
			}
		}
		return items
	case string:
		if trimmed := strings.TrimSpace(typed); trimmed != "" {
			return []string{trimmed}
		}
	}
	return nil
}

func looseFloat(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		if parsed, err := typed.Float64(); err == nil {
			return parsed
		}
	}
	return 0
}
