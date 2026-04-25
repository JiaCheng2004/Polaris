package modality

import (
	"bytes"
	"context"
	"encoding/json"
)

type TranslationAdapter interface {
	Translate(ctx context.Context, req *TranslationRequest) (*TranslationResponse, error)
}

type TranslationRequest struct {
	Model          string            `json:"model"`
	Routing        *RoutingOptions   `json:"routing,omitempty"`
	Input          TranslationInput  `json:"input"`
	TargetLanguage string            `json:"target_language"`
	SourceLanguage string            `json:"source_language,omitempty"`
	Glossary       map[string]string `json:"glossary,omitempty"`
}

type TranslationInput struct {
	Single *string
	Many   []string
}

func NewSingleTranslationInput(value string) TranslationInput {
	return TranslationInput{Single: &value}
}

func NewMultiTranslationInput(values ...string) TranslationInput {
	return TranslationInput{Many: append([]string(nil), values...)}
}

func (i TranslationInput) Values() []string {
	if i.Single != nil {
		return []string{*i.Single}
	}
	return append([]string(nil), i.Many...)
}

func (i TranslationInput) Empty() bool {
	if i.Single != nil {
		return len(bytes.TrimSpace([]byte(*i.Single))) == 0
	}
	return len(i.Many) == 0
}

func (i TranslationInput) MarshalJSON() ([]byte, error) {
	if i.Single != nil {
		return json.Marshal(*i.Single)
	}
	return json.Marshal(i.Many)
}

func (i *TranslationInput) UnmarshalJSON(data []byte) error {
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

type TranslationResponse struct {
	Object       string              `json:"object"`
	Model        string              `json:"model"`
	Translations []TranslationResult `json:"translations"`
	Usage        Usage               `json:"usage"`
}

type TranslationResult struct {
	Index                  int    `json:"index"`
	Text                   string `json:"text"`
	DetectedSourceLanguage string `json:"detected_source_language,omitempty"`
}
