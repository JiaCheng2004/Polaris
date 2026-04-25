package handler

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/gateway/telemetry"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	cachepkg "github.com/JiaCheng2004/Polaris/internal/store/cache"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
)

const cacheHeader = "X-Polaris-Cache"

type responseCache struct {
	cache  cachepkg.Cache
	config config.ResponseCache
}

type cachedResponse struct {
	StatusCode  int    `json:"status_code"`
	ContentType string `json:"content_type"`
	Body        string `json:"body"`
}

type semanticChatIndexEntry struct {
	Key          string `json:"key"`
	SettingsHash string `json:"settings_hash"`
	Query        string `json:"query"`
}

type semanticChatCandidate struct {
	IndexKey     string
	StoreKey     string
	Query        string
	SettingsHash string
	Enabled      bool
}

func newResponseCache(c *gin.Context, runtime *gwruntime.Holder, backend cachepkg.Cache) *responseCache {
	if backend == nil {
		return nil
	}
	snapshot := middleware.RuntimeSnapshot(c, runtime)
	if snapshot == nil || snapshot.Config == nil || !snapshot.Config.Cache.ResponseCache.Enabled {
		return nil
	}
	return &responseCache{
		cache:  backend,
		config: snapshot.Config.Cache.ResponseCache,
	}
}

func (r *responseCache) markBypass(c *gin.Context) {
	if r == nil {
		return
	}
	c.Header(cacheHeader, "bypass")
}

func (r *responseCache) tryExact(c *gin.Context, key string, model provider.Model, requestModality modality.Modality) bool {
	if r == nil || key == "" {
		return false
	}
	ctx, span := telemetry.StartInternalSpan(c.Request.Context(), "cache.lookup",
		attribute.String("polaris.cache_layer", "response_cache"),
		attribute.String("polaris.cache_kind", "exact"),
		attribute.String("polaris.model", model.ID),
		attribute.String("polaris.modality", string(requestModality)),
	)
	defer span.End()
	encoded, ok, err := r.cache.Get(ctx, key)
	if err != nil || !ok {
		span.SetAttributes(attribute.String("polaris.cache_status", "miss"))
		if err != nil {
			telemetry.RecordSpanError(span, err)
		}
		c.Header(cacheHeader, "miss")
		return false
	}
	var stored cachedResponse
	if err := json.Unmarshal([]byte(encoded), &stored); err != nil {
		telemetry.RecordSpanError(span, err)
		span.SetAttributes(attribute.String("polaris.cache_status", "miss"))
		c.Header(cacheHeader, "miss")
		return false
	}
	body, err := base64.StdEncoding.DecodeString(stored.Body)
	if err != nil {
		telemetry.RecordSpanError(span, err)
		span.SetAttributes(attribute.String("polaris.cache_status", "miss"))
		c.Header(cacheHeader, "miss")
		return false
	}
	span.SetAttributes(attribute.String("polaris.cache_status", "hit"))
	c.Header(cacheHeader, "hit")
	middleware.SetRequestOutcome(c, cachedRequestOutcome(model, requestModality, stored.StatusCode, stored.ContentType, body))
	c.Data(stored.StatusCode, stored.ContentType, body)
	c.Abort()
	return true
}

func cachedRequestOutcome(model provider.Model, requestModality modality.Modality, statusCode int, contentType string, body []byte) middleware.RequestOutcome {
	outcome := middleware.RequestOutcome{
		Model:       model.ID,
		Provider:    model.Provider,
		Modality:    requestModality,
		StatusCode:  statusCode,
		CacheStatus: "hit",
		TokenSource: modality.TokenCountSourceUnavailable,
	}

	if !strings.Contains(strings.ToLower(contentType), "json") {
		return outcome
	}

	switch requestModality {
	case modality.ModalityChat:
		var response modality.ChatResponse
		if err := json.Unmarshal(body, &response); err != nil {
			return outcome
		}
		response.Usage = normalizeUsage(response.Usage)
		outcome.PromptTokens = response.Usage.PromptTokens
		outcome.CompletionTokens = response.Usage.CompletionTokens
		outcome.TotalTokens = response.Usage.TotalTokens
		outcome.TokenSource = response.Usage.Source
	case modality.ModalityEmbed:
		var response modality.EmbedResponse
		if err := json.Unmarshal(body, &response); err != nil {
			return outcome
		}
		response.Usage = normalizeEmbedUsage(response.Usage)
		outcome.PromptTokens = response.Usage.PromptTokens
		outcome.TotalTokens = response.Usage.TotalTokens
		outcome.TokenSource = response.Usage.Source
	}

	return outcome
}

func (r *responseCache) storeJSON(c *gin.Context, key string, statusCode int, body any) {
	if r == nil || key == "" {
		return
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return
	}
	r.storeRaw(c, key, statusCode, "application/json; charset=utf-8", raw)
}

func (r *responseCache) storeRaw(c *gin.Context, key string, statusCode int, contentType string, body []byte) {
	if r == nil || key == "" || statusCode >= 400 {
		return
	}
	ctx, span := telemetry.StartInternalSpan(c.Request.Context(), "cache.store",
		attribute.String("polaris.cache_layer", "response_cache"),
		attribute.String("polaris.content_type", contentType),
	)
	defer span.End()
	payload, err := json.Marshal(cachedResponse{
		StatusCode:  statusCode,
		ContentType: contentType,
		Body:        base64.StdEncoding.EncodeToString(body),
	})
	if err != nil {
		telemetry.RecordSpanError(span, err)
		return
	}
	if err := r.cache.Set(ctx, key, string(payload), r.config.TTL); err != nil {
		telemetry.RecordSpanError(span, err)
	}
}

func exactCacheKey(prefix string, modelID string, payload any) string {
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(append([]byte(prefix+":"+modelID+":"), raw...))
	return "resp:exact:" + prefix + ":" + modelID + ":" + hex.EncodeToString(sum[:])
}

func hashBytes(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func (r *responseCache) prepareSemanticChat(model provider.Model, req *modality.ChatRequest) semanticChatCandidate {
	if r == nil || req == nil || req.Stream || len(req.Messages) == 0 || len(req.Messages) > 4 || len(req.Tools) > 0 || req.ResponseFormat != nil || len(req.Stop) > 0 {
		return semanticChatCandidate{}
	}
	var userTexts []string
	var systemTexts []string
	for _, message := range req.Messages {
		if message.Content.Text == nil || len(message.Content.Parts) > 0 {
			return semanticChatCandidate{}
		}
		switch message.Role {
		case "user":
			userTexts = append(userTexts, *message.Content.Text)
		case "system":
			systemTexts = append(systemTexts, *message.Content.Text)
		default:
			return semanticChatCandidate{}
		}
	}
	query := normalizeSemanticText(strings.Join(userTexts, "\n"))
	if query == "" {
		return semanticChatCandidate{}
	}
	settingsHash := exactCacheKey("chat-settings", model.ID, map[string]any{
		"system":      strings.Join(systemTexts, "\n"),
		"temperature": req.Temperature,
		"top_p":       req.TopP,
		"max_tokens":  req.MaxTokens,
	})
	return semanticChatCandidate{
		IndexKey:     "resp:semantic:index:" + model.ID,
		StoreKey:     exactCacheKey("chat-semantic", model.ID, map[string]any{"settings_hash": settingsHash, "query": query}),
		Query:        query,
		SettingsHash: settingsHash,
		Enabled:      true,
	}
}

func (r *responseCache) trySemanticChat(c *gin.Context, model provider.Model, requestModality modality.Modality, candidate semanticChatCandidate) bool {
	if r == nil || !candidate.Enabled {
		return false
	}
	ctx, span := telemetry.StartInternalSpan(c.Request.Context(), "cache.lookup",
		attribute.String("polaris.cache_layer", "response_cache"),
		attribute.String("polaris.cache_kind", "semantic"),
		attribute.String("polaris.model", model.ID),
		attribute.String("polaris.modality", string(requestModality)),
	)
	defer span.End()
	indexRaw, ok, err := r.cache.Get(ctx, candidate.IndexKey)
	if err != nil || !ok {
		span.SetAttributes(attribute.String("polaris.cache_status", "miss"))
		if err != nil {
			telemetry.RecordSpanError(span, err)
		}
		c.Header(cacheHeader, "miss")
		return false
	}
	var index []semanticChatIndexEntry
	if err := json.Unmarshal([]byte(indexRaw), &index); err != nil {
		telemetry.RecordSpanError(span, err)
		span.SetAttributes(attribute.String("polaris.cache_status", "miss"))
		c.Header(cacheHeader, "miss")
		return false
	}
	bestKey := ""
	bestScore := 0.0
	for _, entry := range index {
		if entry.SettingsHash != candidate.SettingsHash || entry.Key == "" {
			continue
		}
		score := semanticSimilarity(candidate.Query, entry.Query)
		if score >= r.config.SimilarityThreshold && score > bestScore {
			bestScore = score
			bestKey = entry.Key
		}
	}
	if bestKey == "" {
		span.SetAttributes(attribute.String("polaris.cache_status", "miss"))
		c.Header(cacheHeader, "miss")
		return false
	}
	span.SetAttributes(attribute.String("polaris.cache_status", "hit"))
	return r.tryExact(c, bestKey, model, requestModality)
}

func (r *responseCache) storeSemanticChat(c *gin.Context, candidate semanticChatCandidate, statusCode int, body any) {
	if r == nil || !candidate.Enabled || candidate.StoreKey == "" || statusCode >= 400 {
		return
	}
	ctx, span := telemetry.StartInternalSpan(c.Request.Context(), "cache.store",
		attribute.String("polaris.cache_layer", "response_cache"),
		attribute.String("polaris.cache_kind", "semantic"),
	)
	defer span.End()
	r.storeJSON(c, candidate.StoreKey, statusCode, body)
	indexRaw, _, _ := r.cache.Get(ctx, candidate.IndexKey)
	var index []semanticChatIndexEntry
	_ = json.Unmarshal([]byte(indexRaw), &index)
	index = append(index, semanticChatIndexEntry{
		Key:          candidate.StoreKey,
		SettingsHash: candidate.SettingsHash,
		Query:        candidate.Query,
	})
	maxEntries := r.config.MaxEntriesPerModel
	if maxEntries > 0 && len(index) > maxEntries {
		index = index[len(index)-maxEntries:]
	}
	raw, err := json.Marshal(index)
	if err != nil {
		telemetry.RecordSpanError(span, err)
		return
	}
	if err := r.cache.Set(ctx, candidate.IndexKey, string(raw), r.config.TTL); err != nil {
		telemetry.RecordSpanError(span, err)
	}
}

func normalizeSemanticText(value string) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(value) {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		default:
			builder.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(builder.String()), " ")
}

func semanticSimilarity(a string, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}
	setA := make(map[string]struct{})
	for _, token := range strings.Fields(a) {
		setA[token] = struct{}{}
	}
	setB := make(map[string]struct{})
	for _, token := range strings.Fields(b) {
		setB[token] = struct{}{}
	}
	if len(setA) == 0 || len(setB) == 0 {
		return 0
	}
	intersection := 0
	for token := range setA {
		if _, ok := setB[token]; ok {
			intersection++
		}
	}
	return (2 * float64(intersection)) / float64(len(setA)+len(setB))
}
