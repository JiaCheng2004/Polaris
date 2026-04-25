package modality

import (
	"context"
	"encoding/json"
)

type TokenCountSource string

const (
	TokenCountSourceProviderReported TokenCountSource = "provider_reported"
	TokenCountSourceEstimated        TokenCountSource = "estimated"
	TokenCountSourceUnavailable      TokenCountSource = "unavailable"
)

type TokenCountRequest struct {
	Model              string          `json:"model"`
	Routing            *RoutingOptions `json:"routing,omitempty"`
	Messages           []ChatMessage   `json:"messages,omitempty"`
	Input              json.RawMessage `json:"input,omitempty"`
	ToolContext        json.RawMessage `json:"tool_context,omitempty"`
	RequestedInterface string          `json:"requested_interface,omitempty"`
	MaxOutputTokens    int             `json:"max_output_tokens,omitempty"`
}

type TokenCountResponse struct {
	Model                string           `json:"model"`
	InputTokens          int              `json:"input_tokens"`
	OutputTokensEstimate int              `json:"output_tokens_estimate"`
	Source               TokenCountSource `json:"source"`
	Notes                []string         `json:"notes,omitempty"`
}

type TokenCountResult struct {
	InputTokens int              `json:"input_tokens"`
	Source      TokenCountSource `json:"source"`
	Notes       []string         `json:"notes,omitempty"`
}

type ConversationTokenCounter interface {
	CountTokens(ctx context.Context, req *ChatRequest) (*TokenCountResult, error)
}
