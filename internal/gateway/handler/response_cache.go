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
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/obs"
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
	ctx, span := obs.StartInternalSpan(c.Request.Context(), "cache.lookup",
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
			obs.RecordSpanError(span, err)
		}
		c.Header(cacheHeader, "miss")
		return false
	}
	var stored cachedResponse
	if err := json.Unmarshal([]byte(encoded), &stored); err != nil {
		obs.RecordSpanError(span, err)
		span.SetAttributes(attribute.String("polaris.cache_status", "miss"))
		c.Header(cacheHeader, "miss")
		return false
	}
	body, err := base64.StdEncoding.DecodeString(stored.Body)
	if err != nil {
		obs.RecordSpanError(span, err)
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
		outcome.CachedInputTokens = response.Usage.CachedInputTokens
		outcome.CacheWrite5mTokens = response.Usage.CacheWrite5mTokens
		outcome.CacheWrite1hTokens = response.Usage.CacheWrite1hTokens
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
	ctx, span := obs.StartInternalSpan(c.Request.Context(), "cache.store",
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
		obs.RecordSpanError(span, err)
		return
	}
	if err := r.cache.Set(ctx, key, string(payload), r.config.TTL); err != nil {
		obs.RecordSpanError(span, err)
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
