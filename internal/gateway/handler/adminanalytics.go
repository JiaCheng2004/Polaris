package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/gin-gonic/gin"
)

// AdminAnalyticsHandler serves admin-gated read endpoints the operator console
// needs: project/global usage with rich breakdowns, and the audit trail. These
// are additive and never alter the existing self-scoped GET /v1/usage.
type AdminAnalyticsHandler struct {
	store store.Store
}

func NewAdminAnalyticsHandler(appStore store.Store) *AdminAnalyticsHandler {
	return &AdminAnalyticsHandler{store: appStore}
}

// allowedGroupBy is the caller-facing set. "provider" is derived in Go from the
// "model" grouping (model IDs are "provider/model"); the rest map to store dimensions.
var allowedGroupBy = map[string]bool{
	"day": true, "model": true, "provider": true, "modality": true,
	"status": true, "token_source": true, "cost_source": true,
	"error_type": true, "interface_family": true,
}

func badRequest(c *gin.Context, code, param, msg string) {
	httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", code, param, msg))
}

func (h *AdminAnalyticsHandler) Usage(c *gin.Context) {
	from := time.Now().UTC().Add(-30 * 24 * time.Hour)
	to := time.Now().UTC()
	var err error
	if raw := c.Query("from"); raw != "" {
		if from, err = time.Parse(time.RFC3339, raw); err != nil {
			badRequest(c, "invalid_from", "from", "Query parameter 'from' must be RFC3339.")
			return
		}
	}
	if raw := c.Query("to"); raw != "" {
		if to, err = time.Parse(time.RFC3339, raw); err != nil {
			badRequest(c, "invalid_to", "to", "Query parameter 'to' must be RFC3339.")
			return
		}
	}
	if from.After(to) {
		badRequest(c, "invalid_range", "from", "'from' must be earlier than 'to'.")
		return
	}

	scope := c.DefaultQuery("scope", "global")
	filter := store.UsageFilter{
		Model:    c.Query("model"),
		Provider: c.Query("provider"),
		From:     &from,
		To:       &to,
	}
	switch scope {
	case "global":
	case "project":
		filter.ProjectID = c.Query("project_id")
		if filter.ProjectID == "" {
			badRequest(c, "missing_project_id", "project_id", "scope=project requires 'project_id'.")
			return
		}
	case "key":
		filter.KeyID = c.Query("key_id")
		if filter.KeyID == "" {
			badRequest(c, "missing_key_id", "key_id", "scope=key requires 'key_id'.")
			return
		}
	default:
		badRequest(c, "invalid_scope", "scope", "Query parameter 'scope' must be 'global', 'project', or 'key'.")
		return
	}
	if raw := c.Query("modality"); raw != "" {
		m := modality.Modality(raw)
		if !m.Valid() {
			badRequest(c, "invalid_modality", "modality", "Query parameter 'modality' is invalid.")
			return
		}
		filter.Modality = m
	}

	groupBy := c.DefaultQuery("group_by", "day")
	if !allowedGroupBy[groupBy] {
		badRequest(c, "invalid_group_by", "group_by", "Unsupported 'group_by' value.")
		return
	}

	ctx := c.Request.Context()
	totalsRows, err := h.store.SummarizeUsage(ctx, filter, "")
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	dimension := groupBy
	if groupBy == "provider" {
		dimension = "model"
	}
	rows, err := h.store.SummarizeUsage(ctx, filter, dimension)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}
	if groupBy == "provider" {
		rows = foldByProvider(rows)
	}

	totals := store.UsageSummaryRow{Key: "total"}
	if len(totalsRows) > 0 {
		totals = totalsRows[0]
	}

	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		out = append(out, usageRowJSON(r))
	}

	c.JSON(http.StatusOK, gin.H{
		"from":     from.Format(time.RFC3339),
		"to":       to.Format(time.RFC3339),
		"scope":    scope,
		"group_by": groupBy,
		"totals":   usageRowJSON(totals),
		"rows":     out,
	})
}

func foldByProvider(rows []store.UsageSummaryRow) []store.UsageSummaryRow {
	idx := map[string]int{}
	var out []store.UsageSummaryRow
	for _, r := range rows {
		provider := r.Key
		if i := strings.IndexByte(provider, '/'); i >= 0 {
			provider = provider[:i]
		}
		pos, ok := idx[provider]
		if !ok {
			idx[provider] = len(out)
			r.Key = provider
			out = append(out, r)
			continue
		}
		acc := &out[pos]
		acc.Requests += r.Requests
		acc.InputTokens += r.InputTokens
		acc.OutputTokens += r.OutputTokens
		acc.CachedInputTokens += r.CachedInputTokens
		acc.CacheWriteTokens += r.CacheWriteTokens
		acc.TotalTokens += r.TotalTokens
		acc.CostUSD += r.CostUSD
		acc.Errors += r.Errors
		acc.ProviderLatencySumMs += r.ProviderLatencySumMs
		acc.TotalLatencySumMs += r.TotalLatencySumMs
	}
	return out
}

func usageRowJSON(r store.UsageSummaryRow) gin.H {
	avgProvider := 0.0
	avgTotal := 0.0
	if r.Requests > 0 {
		avgProvider = float64(r.ProviderLatencySumMs) / float64(r.Requests)
		avgTotal = float64(r.TotalLatencySumMs) / float64(r.Requests)
	}
	return gin.H{
		"key":                     r.Key,
		"requests":                r.Requests,
		"input_tokens":            r.InputTokens,
		"output_tokens":           r.OutputTokens,
		"cached_input_tokens":     r.CachedInputTokens,
		"cache_write_tokens":      r.CacheWriteTokens,
		"total_tokens":            r.TotalTokens,
		"cost_usd":                r.CostUSD,
		"errors":                  r.Errors,
		"avg_provider_latency_ms": avgProvider,
		"avg_total_latency_ms":    avgTotal,
	}
}

func (h *AdminAnalyticsHandler) AuditEvents(c *gin.Context) {
	filter := store.AuditFilter{
		ProjectID:    c.Query("project_id"),
		ActorKeyID:   c.Query("actor_key_id"),
		Kind:         c.Query("kind"),
		ResourceType: c.Query("resource_type"),
	}
	var err error
	if raw := c.Query("from"); raw != "" {
		t, e := time.Parse(time.RFC3339, raw)
		if e != nil {
			badRequest(c, "invalid_from", "from", "Query parameter 'from' must be RFC3339.")
			return
		}
		filter.From = &t
	}
	if raw := c.Query("to"); raw != "" {
		t, e := time.Parse(time.RFC3339, raw)
		if e != nil {
			badRequest(c, "invalid_to", "to", "Query parameter 'to' must be RFC3339.")
			return
		}
		filter.To = &t
	}
	if raw := c.Query("cursor"); raw != "" {
		t, e := time.Parse(time.RFC3339Nano, raw)
		if e != nil {
			badRequest(c, "invalid_cursor", "cursor", "Query parameter 'cursor' must be an RFC3339 timestamp.")
			return
		}
		filter.Before = &t
	}
	if raw := c.Query("limit"); raw != "" {
		n, e := strconv.Atoi(raw)
		if e != nil || n < 1 {
			badRequest(c, "invalid_limit", "limit", "Query parameter 'limit' must be a positive integer.")
			return
		}
		filter.Limit = n
	}

	events, err := h.store.ListAuditEvents(c.Request.Context(), filter)
	if err != nil {
		httputil.WriteError(c, err)
		return
	}

	body := gin.H{"object": "list", "data": events}
	if len(events) > 0 {
		body["next_cursor"] = events[len(events)-1].CreatedAt.UTC().Format(time.RFC3339Nano)
	}
	c.JSON(http.StatusOK, body)
}
