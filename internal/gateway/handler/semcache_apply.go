package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/obs"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/JiaCheng2004/Polaris/internal/semcache"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
)

const jsonContentType = "application/json; charset=utf-8"

type purgeCacheRequest struct {
	Model   string `json:"model"`
	Project string `json:"project"`
}

// PurgeSemanticCache drops in-process semantic-cache namespaces matching the
// optional model/project filters (empty body = purge all). Admin-scoped.
func (h *ChatHandler) PurgeSemanticCache(c *gin.Context) {
	var req purgeCacheRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "Request body must be valid JSON."))
		return
	}
	purged := 0
	if h.semcache != nil {
		purged = h.semcache.Purge(req.Project, req.Model)
	}
	c.JSON(http.StatusOK, gin.H{"object": "cache.purge", "purged": purged})
}

type semanticCandidate struct {
	enabled  bool
	ns       semcache.Namespace
	query    string
	storeKey string
	model    provider.Model
}

// prepareSemantic decides eligibility and builds the namespace/query for a chat
// request. It mirrors the exact-cache eligibility rules and adds a temperature
// bound (or explicit opt-in header).
func (h *ChatHandler) prepareSemantic(c *gin.Context, model provider.Model, req *modality.ChatRequest) semanticCandidate {
	if h.semcache == nil || !h.semcache.Enabled() || isInternalRequest(c) || req.Stream {
		return semanticCandidate{}
	}
	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	if snapshot == nil || snapshot.Config == nil {
		return semanticCandidate{}
	}
	rc := snapshot.Config.Cache.ResponseCache
	if !rc.Enabled || !rc.Semantic.Enabled {
		return semanticCandidate{}
	}
	if len(req.Messages) == 0 || len(req.Tools) > 0 || req.ResponseFormat != nil || len(req.Stop) > 0 {
		return semanticCandidate{}
	}
	maxTemp := rc.Semantic.MaxTemperature
	if maxTemp == 0 {
		maxTemp = 0.3
	}
	explicit := strings.Contains(strings.ToLower(c.GetHeader("X-Polaris-Cache-Control")), "semantic")
	if req.Temperature != nil && *req.Temperature > maxTemp && !explicit {
		return semanticCandidate{}
	}
	maxTurns := rc.Semantic.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 4
	}
	query, systemText, ok := extractSemanticQuery(req, maxTurns)
	if !ok || query == "" {
		return semanticCandidate{}
	}

	threshold := rc.SimilarityThreshold
	if threshold <= 0 {
		threshold = 0.92
	}
	h.semcache.Reconfigure(semcache.Config{Threshold: threshold, TTL: rc.TTL, MaxPerNamespace: rc.MaxEntriesPerModel})
	ns := semcache.Namespace{
		ModelID:      model.ID,
		SettingsHash: semanticSettingsHash(systemText, req),
		ProjectID:    middleware.GetAuthContext(c).ProjectID,
		Epoch:        semanticEpoch(h.semcache.Fingerprint(), threshold, model.ID),
	}
	return semanticCandidate{enabled: true, ns: ns, query: query, storeKey: semanticStoreKey(ns, query), model: model}
}

// trySemantic serves a cached response on a semantic hit.
func (h *ChatHandler) trySemantic(c *gin.Context, cand semanticCandidate) bool {
	if !cand.enabled {
		return false
	}
	ctx, span := obs.StartInternalSpan(c.Request.Context(), "cache.lookup",
		attribute.String("polaris.cache_layer", "semantic_cache"),
		attribute.String("polaris.cache_kind", "semantic"),
		attribute.String("polaris.model", cand.model.ID),
	)
	defer span.End()
	body, verdict, _ := h.semcache.Lookup(ctx, cand.ns, cand.query)
	span.SetAttributes(attribute.String("polaris.cache_status", semanticCacheStatus(verdict)))
	switch verdict {
	case semcache.VerdictHitSemantic:
		c.Header(cacheHeader, "hit-semantic")
		outcome := cachedRequestOutcome(cand.model, modality.ModalityChat, http.StatusOK, jsonContentType, body)
		h.metrics.AddCacheSavings(cand.model.ID, middleware.EstimateCostUSD(cand.model.ID, outcome.PromptTokens, outcome.CompletionTokens))
		middleware.SetRequestOutcome(c, outcome)
		c.Data(http.StatusOK, jsonContentType, body)
		c.Abort()
		return true
	case semcache.VerdictDegraded:
		c.Header(cacheHeader, "degraded")
	default:
		c.Header(cacheHeader, "miss")
	}
	return false
}

func semanticCacheStatus(v semcache.Verdict) string {
	switch v {
	case semcache.VerdictHitSemantic:
		return "hit"
	case semcache.VerdictDegraded:
		return "degraded"
	default:
		return "miss"
	}
}

// storeSemantic caches a successful chat response for future semantic hits.
func (h *ChatHandler) storeSemantic(c *gin.Context, cand semanticCandidate, response *modality.ChatResponse) {
	if !cand.enabled || response == nil {
		return
	}
	body, err := json.Marshal(response)
	if err != nil {
		return
	}
	ctx, span := obs.StartInternalSpan(c.Request.Context(), "cache.store",
		attribute.String("polaris.cache_layer", "semantic_cache"),
		attribute.String("polaris.cache_kind", "semantic"),
	)
	defer span.End()
	_ = h.semcache.Store(ctx, cand.ns, cand.query, cand.storeKey, body)
}

// extractSemanticQuery renders the last maxTurns user/assistant turns as the
// embedding input and the system prompt(s) for the settings hash. Multimodal or
// tool/other-role messages make the request ineligible.
func extractSemanticQuery(req *modality.ChatRequest, maxTurns int) (query, systemText string, ok bool) {
	var turns, systems []string
	for i := range req.Messages {
		m := &req.Messages[i]
		if m.Content.Text == nil || len(m.Content.Parts) > 0 {
			return "", "", false
		}
		switch m.Role {
		case "user", "assistant":
			turns = append(turns, m.Role+": "+*m.Content.Text)
		case "system":
			systems = append(systems, *m.Content.Text)
		default:
			return "", "", false
		}
	}
	if len(turns) == 0 {
		return "", "", false
	}
	if len(turns) > maxTurns {
		turns = turns[len(turns)-maxTurns:]
	}
	return strings.Join(turns, "\n"), strings.Join(systems, "\n"), true
}

func semanticSettingsHash(systemText string, req *modality.ChatRequest) string {
	return hashBytes([]byte(fmt.Sprintf("%s|%g|%g|%d", systemText, derefFloat(req.Temperature), derefFloat(req.TopP), req.MaxTokens)))
}

func derefFloat(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

func semanticEpoch(fingerprint string, threshold float64, modelID string) uint64 {
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "%s|%g|%s", fingerprint, threshold, modelID)
	return h.Sum64()
}

func semanticStoreKey(ns semcache.Namespace, query string) string {
	return "semcache:" + hashBytes([]byte(fmt.Sprintf("%d|%s|%s|%s|%s", ns.Epoch, ns.ProjectID, ns.ModelID, ns.SettingsHash, query)))
}
