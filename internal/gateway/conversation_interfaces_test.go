package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func TestResponsesEndpointUsesOpenAINativeResponses(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream payload: %v", err)
		}
		if payload["model"] != "gpt-4o" {
			t.Fatalf("expected provider model gpt-4o, got %#v", payload["model"])
		}
		if payload["instructions"] != "Be concise." || payload["input"] != "Say hello" {
			t.Fatalf("unexpected responses payload %#v", payload)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"resp_123",
			"object":"response",
			"created_at":1744329600,
			"status":"completed",
			"model":"gpt-4o",
			"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hello from Polaris"}]}],
			"usage":{"input_tokens":12,"output_tokens":5,"total_tokens":17}
		}`))
	}))
	defer upstream.Close()

	engine := newTestEngine(t, testConfigWithOpenAIBaseURL(t, upstream.URL))

	body := strings.NewReader(`{
		"model":"default-chat",
		"instructions":"Be concise.",
		"input":"Say hello",
		"metadata":{"session":"abc"}
	}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", body)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected /v1/responses 200, got %d body=%s", res.Code, res.Body.String())
	}

	var response struct {
		Object string `json:"object"`
		Status string `json:"status"`
		Usage  struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
		Output []struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Object != "response" || response.Status != "completed" {
		t.Fatalf("unexpected responses envelope %#v", response)
	}
	if response.Usage.TotalTokens != 17 {
		t.Fatalf("unexpected responses payload %#v", response)
	}
	if len(response.Output) != 1 || response.Output[0].Type != "message" || response.Output[0].Role != "assistant" || response.Output[0].Content[0].Text != "Hello from Polaris" {
		t.Fatalf("unexpected output items %#v", response.Output)
	}
	if bytes.Contains(res.Body.Bytes(), []byte(`"model":"gpt-4o"`)) || !bytes.Contains(res.Body.Bytes(), []byte(`"model":"openai/gpt-4o"`)) {
		t.Fatalf("expected canonical model in response, got %s", res.Body.String())
	}
}

func TestMessagesEndpointUsesAnthropicNativeMessages(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream payload: %v", err)
		}
		if payload["model"] != "claude-sonnet-4-6" {
			t.Fatalf("expected anthropic provider model, got %#v", payload["model"])
		}
		if payload["system"] != "You are helpful." {
			t.Fatalf("unexpected anthropic system prompt %#v", payload["system"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"msg_123",
			"type":"message",
			"role":"assistant",
			"model":"claude-sonnet-4-6",
			"stop_reason":"tool_use",
			"content":[
				{"type":"text","text":"Need tool help"},
				{"type":"tool_use","id":"call_1","name":"lookup_weather","input":{"city":"SF"}}
			],
			"usage":{"input_tokens":20,"output_tokens":7}
		}`))
	}))
	defer upstream.Close()

	cfg := testConfig(t)
	cfg.Providers = map[string]config.ProviderConfig{
		"anthropic": {
			APIKey:  "sk-anthropic",
			BaseURL: upstream.URL,
			Timeout: time.Second,
			Models: map[string]config.ModelConfig{
				"claude-sonnet-4-6": {
					Modality: modality.ModalityChat,
					Capabilities: []modality.Capability{
						modality.CapabilityStreaming,
						modality.CapabilityFunctionCalling,
						modality.CapabilityVision,
					},
					MaxOutputTokens: 8192,
				},
			},
		},
	}
	cfg.Routing.Aliases = map[string]string{"default-chat": "anthropic/claude-sonnet-4-6"}
	engine := newTestEngine(t, cfg)

	body := strings.NewReader(`{
		"model":"default-chat",
		"system":"You are helpful.",
		"max_tokens":128,
		"messages":[{"role":"user","content":[{"type":"text","text":"What is the weather?"}]}]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", body)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected /v1/messages 200, got %d body=%s", res.Code, res.Body.String())
	}

	var response struct {
		Type       string `json:"type"`
		Role       string `json:"role"`
		StopReason string `json:"stop_reason"`
		Content    []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text,omitempty"`
			Name  string          `json:"name,omitempty"`
			Input json.RawMessage `json:"input,omitempty"`
		} `json:"content"`
		Model string `json:"model"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Type != "message" || response.Role != "assistant" || response.StopReason != "tool_use" {
		t.Fatalf("unexpected messages envelope %#v", response)
	}
	if len(response.Content) != 2 || response.Content[0].Type != "text" || response.Content[1].Type != "tool_use" {
		t.Fatalf("unexpected messages content %#v", response.Content)
	}
	if !bytes.Contains(response.Content[1].Input, []byte(`"city":"SF"`)) {
		t.Fatalf("expected tool_use input JSON, got %s", string(response.Content[1].Input))
	}
	if response.Usage.InputTokens != 20 || response.Usage.OutputTokens != 7 {
		t.Fatalf("unexpected usage %#v", response.Usage)
	}
	if response.Model != "anthropic/claude-sonnet-4-6" {
		t.Fatalf("expected canonical anthropic model, got %#v", response)
	}
}

func TestTokenCountEndpointReturnsEstimatedCounts(t *testing.T) {
	engine := newTestEngine(t, testConfigWithOpenAIBaseURL(t, "http://example.invalid"))

	body := strings.NewReader(`{
		"model":"default-chat",
		"requested_interface":"responses",
		"input":"Count these tokens please."
	}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/tokens/count", body)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected /v1/tokens/count 200, got %d body=%s", res.Code, res.Body.String())
	}

	var response struct {
		Model                string   `json:"model"`
		InputTokens          int      `json:"input_tokens"`
		OutputTokensEstimate int      `json:"output_tokens_estimate"`
		Source               string   `json:"source"`
		Notes                []string `json:"notes"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode token count response: %v", err)
	}
	if response.Model != "openai/gpt-4o" {
		t.Fatalf("unexpected model %q", response.Model)
	}
	if response.InputTokens <= 0 || response.OutputTokensEstimate != 16384 {
		t.Fatalf("unexpected token counts %#v", response)
	}
	if response.Source != "estimated" || len(response.Notes) == 0 {
		t.Fatalf("unexpected token count metadata %#v", response)
	}
}

func TestTokenCountEndpointUsesAnthropicNativeCounter(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages/count_tokens" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"input_tokens":29}`))
	}))
	defer upstream.Close()

	cfg := testConfig(t)
	cfg.Providers = map[string]config.ProviderConfig{
		"anthropic": {
			APIKey:  "sk-anthropic",
			BaseURL: upstream.URL,
			Timeout: time.Second,
			Models: map[string]config.ModelConfig{
				"claude-sonnet-4-6": {
					Modality: modality.ModalityChat,
					Capabilities: []modality.Capability{
						modality.CapabilityStreaming,
						modality.CapabilityFunctionCalling,
						modality.CapabilityVision,
					},
					MaxOutputTokens: 8192,
				},
			},
		},
	}
	cfg.Routing.Aliases = map[string]string{
		"default-chat": "anthropic/claude-sonnet-4-6",
	}

	engine := newTestEngine(t, cfg)
	body := strings.NewReader(`{
		"model":"default-chat",
		"messages":[{"role":"user","content":"Count these tokens please."}]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/tokens/count", body)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected /v1/tokens/count 200, got %d body=%s", res.Code, res.Body.String())
	}

	var response struct {
		Model                string   `json:"model"`
		InputTokens          int      `json:"input_tokens"`
		OutputTokensEstimate int      `json:"output_tokens_estimate"`
		Source               string   `json:"source"`
		Notes                []string `json:"notes"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode token count response: %v", err)
	}
	if response.Model != "anthropic/claude-sonnet-4-6" || response.InputTokens != 29 {
		t.Fatalf("unexpected token count response %#v", response)
	}
	if response.Source != "provider_reported" || response.OutputTokensEstimate != 8192 {
		t.Fatalf("unexpected token count metadata %#v", response)
	}
}

func TestTokenCountEndpointUsesGeminiNativeCounter(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-2.5-flash:countTokens" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"totalTokens":31}`))
	}))
	defer upstream.Close()

	cfg := testConfig(t)
	cfg.Providers = map[string]config.ProviderConfig{
		"google": {
			APIKey:  "google-key",
			BaseURL: upstream.URL,
			Timeout: time.Second,
			Models: map[string]config.ModelConfig{
				"gemini-2.5-flash": {
					Modality: modality.ModalityChat,
					Capabilities: []modality.Capability{
						modality.CapabilityStreaming,
						modality.CapabilityFunctionCalling,
						modality.CapabilityVision,
					},
					MaxOutputTokens: 8192,
				},
			},
		},
	}
	cfg.Routing.Aliases = map[string]string{
		"default-chat": "google/gemini-2.5-flash",
	}

	engine := newTestEngine(t, cfg)
	body := strings.NewReader(`{
		"model":"default-chat",
		"requested_interface":"responses",
		"input":"Count these tokens please."
	}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/tokens/count", body)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected /v1/tokens/count 200, got %d body=%s", res.Code, res.Body.String())
	}

	var response struct {
		Model                string   `json:"model"`
		InputTokens          int      `json:"input_tokens"`
		OutputTokensEstimate int      `json:"output_tokens_estimate"`
		Source               string   `json:"source"`
		Notes                []string `json:"notes"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode token count response: %v", err)
	}
	if response.Model != "google/gemini-2.5-flash" || response.InputTokens != 31 {
		t.Fatalf("unexpected token count response %#v", response)
	}
	if response.Source != "provider_reported" || response.OutputTokensEstimate != 8192 {
		t.Fatalf("unexpected token count metadata %#v", response)
	}
}

func TestResponsesEndpointStreaming(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\",\"object\":\"response\",\"created_at\":1744329600,\"status\":\"in_progress\",\"model\":\"gpt-4o\",\"output\":[],\"usage\":{\"input_tokens\":0,\"output_tokens\":0,\"total_tokens\":0}}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"Hello\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"object\":\"response\",\"created_at\":1744329601,\"status\":\"completed\",\"model\":\"gpt-4o\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"Hello\"}]}],\"usage\":{\"input_tokens\":8,\"output_tokens\":4,\"total_tokens\":12}}}\n\n"))
	}))
	defer upstream.Close()

	engine := newTestEngine(t, testConfigWithOpenAIBaseURL(t, upstream.URL))

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"default-chat","input":"Hello","stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected /v1/responses stream 200, got %d body=%s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	for _, needle := range []string{`"type":"response.created"`, `"type":"response.output_text.delta"`, `"type":"response.completed"`, `"model":"openai/gpt-4o"`, `"total_tokens":12`} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected stream body to contain %q, got %s", needle, body)
		}
	}
	if strings.Contains(body, `data: [DONE]`) {
		t.Fatalf("expected native /v1/responses stream to close without [DONE], got %s", body)
	}
}

func TestMessagesEndpointStreaming(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"claude-sonnet-4-6\",\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":9,\"output_tokens\":0}}}\n\n"))
		_, _ = w.Write([]byte("event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n"))
		_, _ = w.Write([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n"))
		_, _ = w.Write([]byte("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n"))
		_, _ = w.Write([]byte("event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\",\"stop_sequence\":null},\"usage\":{\"input_tokens\":9,\"output_tokens\":3}}\n\n"))
		_, _ = w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer upstream.Close()

	cfg := testConfig(t)
	cfg.Providers = map[string]config.ProviderConfig{
		"anthropic": {
			APIKey:  "sk-anthropic",
			BaseURL: upstream.URL,
			Timeout: time.Second,
			Models: map[string]config.ModelConfig{
				"claude-sonnet-4-6": {
					Modality: modality.ModalityChat,
					Capabilities: []modality.Capability{
						modality.CapabilityStreaming,
						modality.CapabilityFunctionCalling,
						modality.CapabilityVision,
					},
					MaxOutputTokens: 8192,
				},
			},
		},
	}
	cfg.Routing.Aliases = map[string]string{"default-chat": "anthropic/claude-sonnet-4-6"}
	engine := newTestEngine(t, cfg)

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"default-chat","messages":[{"role":"user","content":"Hello"}],"stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected /v1/messages stream 200, got %d body=%s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	for _, needle := range []string{`event: message_start`, `"type":"message_start"`, `"model":"anthropic/claude-sonnet-4-6"`, `event: content_block_delta`, `"type":"content_block_delta"`, `"output_tokens":3`, `event: message_stop`, `"type":"message_stop"`} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected stream body to contain %q, got %s", needle, body)
		}
	}
	if strings.Contains(body, `data: [DONE]`) {
		t.Fatalf("expected native /v1/messages stream to close without [DONE], got %s", body)
	}
}
