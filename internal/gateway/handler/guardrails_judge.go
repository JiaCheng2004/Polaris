package handler

import (
	"context"
	"errors"
	"strings"

	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

// registryJudge backs the guardrails llm_judge detector. It runs a completion
// through the provider registry's chat adapter directly — not the HTTP handler —
// so the judge call never re-enters guardrails or the semantic cache. Recursion
// is therefore structurally impossible.
type registryJudge struct {
	runtime *gwruntime.Holder
}

func (j *registryJudge) Complete(ctx context.Context, model, prompt string) (string, error) {
	if j == nil || j.runtime == nil {
		return "", errors.New("judge runtime unavailable")
	}
	snapshot := j.runtime.Current()
	if snapshot == nil || snapshot.Registry == nil {
		return "", errors.New("registry unavailable")
	}
	resolution, err := snapshot.Registry.RequireResolvedModel(model, modality.ModalityChat, nil)
	if err != nil {
		return "", err
	}
	adapter, _, err := snapshot.Registry.GetChatAdapter(resolution.Model.ID)
	if err != nil {
		return "", err
	}
	resp, err := adapter.Complete(ctx, &modality.ChatRequest{
		Model:    resolution.Model.ID,
		Messages: []modality.ChatMessage{{Role: "user", Content: modality.NewTextContent(prompt)}},
	})
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", nil
	}
	content := resp.Choices[0].Message.Content
	if content.Text != nil {
		return *content.Text, nil
	}
	var b strings.Builder
	for _, p := range content.Parts {
		if p.Type == "text" {
			b.WriteString(p.Text)
		}
	}
	return b.String(), nil
}
