package handler

import (
	"net/http"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/gin-gonic/gin"
)

type UsageHandler struct {
	store store.Store
}

func NewUsageHandler(appStore store.Store) *UsageHandler {
	return &UsageHandler{store: appStore}
}

func (h *UsageHandler) Get(c *gin.Context) {
	auth := middleware.GetAuthContext(c)

	from := time.Now().UTC().Add(-30 * 24 * time.Hour)
	to := time.Now().UTC()
	var err error

	if raw := c.Query("from"); raw != "" {
		from, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_from", "from", "Query parameter 'from' must be RFC3339."))
			return
		}
	}
	if raw := c.Query("to"); raw != "" {
		to, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_to", "to", "Query parameter 'to' must be RFC3339."))
			return
		}
	}
	if from.After(to) {
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_range", "from", "'from' must be earlier than 'to'."))
		return
	}

	filter := store.UsageFilter{
		KeyID: auth.KeyID,
		Model: c.Query("model"),
		From:  &from,
		To:    &to,
	}
	if raw := c.Query("modality"); raw != "" {
		m := modality.Modality(raw)
		if !m.Valid() {
			httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_modality", "modality", "Query parameter 'modality' is invalid."))
			return
		}
		filter.Modality = m
	}

	groupBy := c.DefaultQuery("group_by", "day")

	var report store.UsageReport
	switch groupBy {
	case "day":
		report, err = h.store.GetUsage(c.Request.Context(), filter)
	case "model":
		report, err = h.store.GetUsageByModel(c.Request.Context(), filter)
	default:
		httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_group_by", "group_by", "Query parameter 'group_by' must be 'day' or 'model'."))
		return
	}
	if err != nil {
		httputil.WriteError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"from":                  from.Format(time.RFC3339),
		"to":                    to.Format(time.RFC3339),
		"total_requests":        report.TotalRequests,
		"total_tokens":          report.TotalTokens,
		"total_cost_usd":        report.TotalCost,
		"cost_source_breakdown": report.CostSourceBreakdown,
		"by_day":                report.ByDay,
		"by_model":              report.ByModel,
	})
}
