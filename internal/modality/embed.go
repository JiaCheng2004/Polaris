package modality

import (
	"bytes"
	"context"
	"encoding/json"
)

type EmbedAdapter interface {
	Embed(ctx context.Context, req *EmbedRequest) (*EmbedResponse, error)
}

type EmbedRequest struct {
	Model          string          `json:"model"`
	Routing        *RoutingOptions `json:"routing,omitempty"`
	Input          EmbedInput      `json:"input"`
	Dimensions     *int            `json:"dimensions,omitempty"`
	EncodingFormat string          `json:"encoding_format,omitempty"`
	User           string          `json:"user,omitempty"`
}

type EmbedInput struct {
	Single *string
	Many   []string
}

func NewSingleEmbedInput(value string) EmbedInput {
	return EmbedInput{Single: &value}
}

func NewMultiEmbedInput(values ...string) EmbedInput {
	return EmbedInput{Many: append([]string(nil), values...)}
}

func (i EmbedInput) Values() []string {
	if i.Single != nil {
		return []string{*i.Single}
	}
	return append([]string(nil), i.Many...)
}

func (i EmbedInput) Empty() bool {
	if i.Single != nil {
		return len(bytes.TrimSpace([]byte(*i.Single))) == 0
	}
	return len(i.Many) == 0
}

func (i EmbedInput) MarshalJSON() ([]byte, error) {
	if i.Single != nil {
		return json.Marshal(*i.Single)
	}
	return json.Marshal(i.Many)
}

func (i *EmbedInput) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	switch {
	case bytes.Equal(trimmed, []byte("null")):
		i.Single = nil
		i.Many = nil
		return nil
	case len(trimmed) > 0 && trimmed[0] == '"':
		var value string
		if err := json.Unmarshal(trimmed, &value); err != nil {
			return err
		}
		i.Single = &value
		i.Many = nil
		return nil
	case len(trimmed) > 0 && trimmed[0] == '[':
		var values []string
		if err := json.Unmarshal(trimmed, &values); err != nil {
			return err
		}
		i.Single = nil
		i.Many = values
		return nil
	default:
		return json.Unmarshal(trimmed, &i.Many)
	}
}

type EmbedResponse struct {
	Object string      `json:"object"`
	Data   []Embedding `json:"data"`
	Model  string      `json:"model"`
	Usage  EmbedUsage  `json:"usage"`
}

type Embedding struct {
	Object    string          `json:"object"`
	Index     int             `json:"index"`
	Embedding EmbeddingValues `json:"embedding"`
}

type EmbeddingValues struct {
	Float32 []float32
	Base64  string
}

func (v EmbeddingValues) MarshalJSON() ([]byte, error) {
	if v.Base64 != "" {
		return json.Marshal(v.Base64)
	}
	if v.Float32 == nil {
		return json.Marshal([]float32{})
	}
	return json.Marshal(v.Float32)
}

func (v *EmbeddingValues) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	switch {
	case bytes.Equal(trimmed, []byte("null")):
		v.Float32 = nil
		v.Base64 = ""
		return nil
	case len(trimmed) > 0 && trimmed[0] == '"':
		var value string
		if err := json.Unmarshal(trimmed, &value); err != nil {
			return err
		}
		v.Base64 = value
		v.Float32 = nil
		return nil
	default:
		var values []float32
		if err := json.Unmarshal(trimmed, &values); err != nil {
			return err
		}
		v.Float32 = values
		v.Base64 = ""
		return nil
	}
}

type EmbedUsage struct {
	PromptTokens int              `json:"prompt_tokens"`
	TotalTokens  int              `json:"total_tokens"`
	Source       TokenCountSource `json:"source,omitempty"`
}
