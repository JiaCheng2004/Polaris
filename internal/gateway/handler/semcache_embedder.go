package handler

import (
	"context"
	"errors"

	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

// registryEmbedder backs the semantic cache. It embeds through the provider
// registry's embed adapter directly (not the HTTP handler), so the embed call
// never re-enters guardrails or the cache. The model is read from the current
// snapshot each call, so a config change takes effect (and changes Fingerprint,
// bumping the cache epoch).
type registryEmbedder struct {
	runtime *gwruntime.Holder
}

func (e *registryEmbedder) model() string {
	if e == nil || e.runtime == nil {
		return ""
	}
	snapshot := e.runtime.Current()
	if snapshot == nil || snapshot.Config == nil {
		return ""
	}
	return snapshot.Config.Cache.ResponseCache.Semantic.EmbeddingModel
}

func (e *registryEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	model := e.model()
	if model == "" {
		return nil, errors.New("semcache: embedding_model not configured")
	}
	snapshot := e.runtime.Current()
	if snapshot == nil || snapshot.Registry == nil {
		return nil, errors.New("semcache: registry unavailable")
	}
	resolution, err := snapshot.Registry.RequireResolvedModel(model, modality.ModalityEmbed, nil)
	if err != nil {
		return nil, err
	}
	adapter, _, err := snapshot.Registry.GetEmbedAdapter(resolution.Model.ID)
	if err != nil {
		return nil, err
	}
	resp, err := adapter.Embed(ctx, &modality.EmbedRequest{
		Model: resolution.Model.ID,
		Input: modality.EmbedInput{Many: texts},
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Data) == 0 {
		return nil, errors.New("semcache: embedder returned no vectors")
	}
	out := make([][]float32, len(resp.Data))
	for i, d := range resp.Data {
		if len(d.Embedding.Float32) == 0 {
			return nil, errors.New("semcache: embedder returned an empty vector")
		}
		out[i] = d.Embedding.Float32
	}
	return out, nil
}

// Fingerprint changes when the embedding model changes, bumping the cache epoch.
func (e *registryEmbedder) Fingerprint() string { return "registry:" + e.model() }
