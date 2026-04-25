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
	defaultTranslationEndpoint      = "https://openspeech.bytedance.com/api/v3/machine_translation/matx_translate"
	bytedanceTranslationResourceID  = "volc.speech.mt"
	bytedanceTranslationSuccessCode = 20000000
)

type TranslationAdapter struct {
	client   *Client
	model    string
	endpoint string
}

type translationRequest struct {
	SourceLanguage string             `json:"source_language,omitempty"`
	TargetLanguage string             `json:"target_language"`
	TextList       []string           `json:"text_list"`
	Corpus         *translationCorpus `json:"corpus,omitempty"`
}

type translationCorpus struct {
	GlossaryList map[string]string `json:"glossary_list,omitempty"`
}

type translationResponseEnvelope struct {
	Code    int                     `json:"code"`
	Message string                  `json:"message"`
	Data    translationResponseData `json:"data"`
}

type translationResponseData struct {
	TranslationList []translationResponseItem `json:"translation_list"`
}

type translationResponseItem struct {
	Translation            string           `json:"translation"`
	DetectedSourceLanguage string           `json:"detected_source_language,omitempty"`
	Usage                  translationUsage `json:"usage"`
}

type translationUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func NewTranslationAdapter(client *Client, model string, endpoint string) *TranslationAdapter {
	return &TranslationAdapter{
		client:   client,
		model:    model,
		endpoint: endpoint,
	}
}

func (a *TranslationAdapter) Translate(ctx context.Context, req *modality.TranslationRequest) (*modality.TranslationResponse, error) {
	if strings.TrimSpace(a.client.speechAPIKey) == "" {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_misconfigured", "", "ByteDance translation requires providers.bytedance.speech_api_key.")
	}

	payload := translationRequest{
		SourceLanguage: strings.TrimSpace(req.SourceLanguage),
		TargetLanguage: strings.TrimSpace(req.TargetLanguage),
		TextList:       req.Input.Values(),
	}
	if len(req.Glossary) > 0 {
		payload.Corpus = &translationCorpus{
			GlossaryList: req.Glossary,
		}
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal bytedance translation request: %w", err)
	}

	requestID := newRequestID()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.translationURL(), bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("build bytedance translation request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("X-Api-Key", a.client.speechAPIKey)
	httpReq.Header.Set("X-Api-Resource-Id", bytedanceTranslationResourceID)
	httpReq.Header.Set("X-Api-Request-Id", requestID)

	resp, err := a.client.httpClient.Do(httpReq)
	if err != nil {
		return nil, translateTransportError(err, "ByteDance")
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance returned an invalid translation response.")
	}

	var parsed translationResponseEnvelope
	if err := json.Unmarshal(body, &parsed); err != nil {
		if resp.StatusCode >= http.StatusBadRequest {
			return nil, bytedanceTranslationError(resp.StatusCode, 0, strings.TrimSpace(string(body)))
		}
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance returned an invalid translation JSON response.")
	}

	if resp.StatusCode >= http.StatusBadRequest || parsed.Code != bytedanceTranslationSuccessCode {
		return nil, bytedanceTranslationError(resp.StatusCode, parsed.Code, parsed.Message)
	}

	result := &modality.TranslationResponse{
		Object:       "translation.list",
		Model:        firstNonEmpty(req.Model, a.model),
		Translations: make([]modality.TranslationResult, 0, len(parsed.Data.TranslationList)),
		Usage: modality.Usage{
			Source: modality.TokenCountSourceProviderReported,
		},
	}
	for index, item := range parsed.Data.TranslationList {
		result.Translations = append(result.Translations, modality.TranslationResult{
			Index:                  index,
			Text:                   item.Translation,
			DetectedSourceLanguage: item.DetectedSourceLanguage,
		})
		result.Usage.PromptTokens += item.Usage.PromptTokens
		result.Usage.CompletionTokens += item.Usage.CompletionTokens
		result.Usage.TotalTokens += item.Usage.TotalTokens
	}

	return result, nil
}

func (a *TranslationAdapter) translationURL() string {
	if trimmed := strings.TrimSpace(a.endpoint); trimmed != "" {
		return trimmed
	}
	return defaultTranslationEndpoint
}

func bytedanceTranslationError(status int, code int, message string) error {
	if strings.TrimSpace(message) == "" {
		message = "ByteDance returned a translation error."
	}

	switch code {
	case 45000001:
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "provider_bad_request", "", message)
	case 45000130:
		return httputil.NewError(http.StatusBadRequest, "invalid_request_error", "context_limit_exceeded", "", message)
	case 55000001:
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_server_error", "", message)
	}
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_auth_failed", "", message)
	case status == http.StatusTooManyRequests:
		return httputil.NewError(http.StatusTooManyRequests, "rate_limit_error", "provider_rate_limit", "", message)
	case status >= http.StatusInternalServerError:
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_server_error", "", message)
	default:
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_error", "", message)
	}
}
