package modality

import (
	"context"
	"strings"
)

type VoiceCatalogAdapter interface {
	ListVoices(ctx context.Context, req *VoiceCatalogRequest) (*VoiceCatalogResponse, error)
}

type VoiceCatalogRequest struct {
	Provider           string   `json:"-"`
	Model              string   `json:"-"`
	Scope              string   `json:"-"`
	Type               string   `json:"-"`
	State              string   `json:"-"`
	Limit              int      `json:"-"`
	IncludeArchived    bool     `json:"-"`
	ConfiguredVoiceIDs []string `json:"-"`
}

type VoiceCatalogResponse struct {
	Object   string             `json:"object"`
	Scope    string             `json:"scope"`
	Provider string             `json:"provider,omitempty"`
	Data     []VoiceCatalogItem `json:"data"`
}

type VoiceCatalogItem struct {
	ID          string              `json:"id"`
	Provider    string              `json:"provider,omitempty"`
	Type        string              `json:"type,omitempty"`
	Name        string              `json:"name,omitempty"`
	Gender      string              `json:"gender,omitempty"`
	Age         string              `json:"age,omitempty"`
	State       string              `json:"state,omitempty"`
	Models      []string            `json:"models,omitempty"`
	Categories  []string            `json:"categories,omitempty"`
	Emotions    []VoiceCatalogStyle `json:"emotions,omitempty"`
	PreviewURL  string              `json:"preview_url,omitempty"`
	PreviewText string              `json:"preview_text,omitempty"`
	Error       string              `json:"error,omitempty"`
	Metadata    map[string]any      `json:"metadata,omitempty"`
}

type VoiceCatalogStyle struct {
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	PreviewURL  string `json:"preview_url,omitempty"`
	PreviewText string `json:"preview_text,omitempty"`
}

func NormalizeVoiceCatalogScope(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "", "config":
		return "config"
	case "provider":
		return "provider"
	default:
		return ""
	}
}

func NormalizeVoiceCatalogType(candidate string) string {
	switch strings.ToLower(strings.TrimSpace(candidate)) {
	case "", "builtin":
		return "builtin"
	case "all", "custom":
		return strings.ToLower(strings.TrimSpace(candidate))
	default:
		return ""
	}
}
