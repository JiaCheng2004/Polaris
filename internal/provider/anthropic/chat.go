package anthropic

import (
	"context"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/common/anthropiccompat"
)

// ChatAdapter is the shared Anthropic-compatible chat adapter with Anthropic's
// native extensions enabled (thinking budget, hosted tools, per-request beta
// headers) plus the native count_tokens endpoint. Complete/Stream and the
// native CreateMessage/StreamMessage passthrough are inherited from the compat
// adapter.
type ChatAdapter struct {
	*anthropiccompat.ChatAdapter
	client *Client
	model  string
}

func NewChatAdapter(client *Client, model string, defaultMaxTokens int) *ChatAdapter {
	base := anthropiccompat.NewChatAdapterWithOptions(client.Client, model, anthropiccompat.Options{
		DefaultMaxTokens:  defaultMaxTokens,
		EnableThinking:    true,
		EnableHostedTools: true,
		Betas:             anthropicRequestBetas,
	})
	return &ChatAdapter{ChatAdapter: base, client: client, model: model}
}

// CountTokens uses Anthropic's native /v1/messages/count_tokens endpoint.
func (a *ChatAdapter) CountTokens(ctx context.Context, req *modality.ChatRequest) (*modality.TokenCountResult, error) {
	payload, err := a.CountTokensPayload(req)
	if err != nil {
		return nil, err
	}

	var response anthropiccompat.CountTokensResponse
	if _, err := a.client.JSON(ctx, "/v1/messages/count_tokens", payload, &response); err != nil {
		return nil, err
	}

	return &modality.TokenCountResult{
		InputTokens: response.InputTokens,
		Source:      modality.TokenCountSourceProviderReported,
		Notes: []string{
			"input tokens were returned by Anthropic's native token counting endpoint",
			"output_tokens_estimate remains a Polaris estimate derived from max_tokens limits",
		},
	}, nil
}

// anthropicRequestBetas derives the anthropic-beta feature flags a request needs
// from its file references and hosted tools.
func anthropicRequestBetas(req *modality.ChatRequest) []string {
	if req == nil {
		return nil
	}
	var betas []string
	add := func(beta string) {
		for _, existing := range betas {
			if existing == beta {
				return
			}
		}
		betas = append(betas, beta)
	}
	for _, message := range req.Messages {
		for _, part := range message.Content.Parts {
			if (part.Type == "file" && part.File != nil && strings.TrimSpace(part.File.FileID) != "") ||
				(part.Type == "document" && part.Document != nil && strings.TrimSpace(part.Document.FileID) != "") {
				add("files-api-2025-04-14")
			}
		}
	}
	for _, tool := range req.Tools {
		if tool.Type != "hosted" || tool.Hosted == nil {
			continue
		}
		switch strings.TrimSpace(tool.Hosted.Name) {
		case "code_interpreter":
			add("code-execution-2026-01-20")
		case "computer_use":
			add("computer-use-2025-11-24")
		case "url_context":
			add("web-fetch-2026-02-09")
		case "mcp":
			add("mcp-client-2025-04-04")
		}
	}
	return betas
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
