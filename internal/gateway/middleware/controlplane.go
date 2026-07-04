package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/metrics"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/obs"
	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
)

func ControlPlaneAdmin(runtime *gwruntime.Holder) gin.HandlerFunc {
	return func(c *gin.Context) {
		snapshot := RuntimeSnapshot(c, runtime)
		if snapshot == nil || snapshot.Config == nil {
			httputil.WriteError(c, httputil.NewError(http.StatusInternalServerError, "internal_error", "runtime_unavailable", "", "Runtime configuration is unavailable."))
			return
		}
		auth := GetAuthContext(c)
		if !auth.IsAdmin {
			httputil.WriteError(c, httputil.NewError(http.StatusForbidden, "permission_error", "admin_required", "", "Admin access is required for this endpoint."))
			return
		}
		c.Next()
	}
}

func ControlPlaneEnabled(runtime *gwruntime.Holder) gin.HandlerFunc {
	return func(c *gin.Context) {
		snapshot := RuntimeSnapshot(c, runtime)
		if snapshot == nil || snapshot.Config == nil {
			httputil.WriteError(c, httputil.NewError(http.StatusInternalServerError, "internal_error", "runtime_unavailable", "", "Runtime configuration is unavailable."))
			return
		}
		if !snapshot.Config.ControlPlane.Enabled {
			httputil.WriteError(c, httputil.NewError(http.StatusNotFound, "invalid_request_error", "control_plane_disabled", "", "Control-plane management endpoints are disabled."))
			return
		}
		c.Next()
	}
}

// controlPlaneCacheTTL bounds how stale cached control-plane config reads
// (budget limits, aggregated project policies) may be. These are admin-set
// config that changes rarely, so a short TTL removes a per-request DB query with
// eventual consistency; a config change takes effect within this window. Actual
// spend (GetUsage) is never cached, so hard-budget enforcement stays accurate.
const controlPlaneCacheTTL = 60 * time.Second

// ProjectPolicies is the aggregated, cacheable result of a project's policies.
type ProjectPolicies struct {
	Models     []string
	Modalities []modality.Modality
	Toolsets   []string
	Bindings   []string
}

func Budget(runtime *gwruntime.Holder, appStore store.Store, recorder *metrics.Recorder, auditLogger *store.AsyncAuditLogger, logger *slog.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}
	budgetCache := store.NewTTLCache[[]store.Budget](controlPlaneCacheTTL)

	return func(c *gin.Context) {
		ctx, span := obs.StartInternalSpan(c.Request.Context(), "budget.evaluate")
		defer span.End()
		c.Request = c.Request.WithContext(ctx)

		snapshot := RuntimeSnapshot(c, runtime)
		auth := GetAuthContext(c)
		if snapshot == nil || snapshot.Config == nil {
			c.Next()
			return
		}
		authMode := snapshot.Config.Auth.Mode
		if authMode != config.AuthModeVirtualKeys && authMode != config.AuthModeExternal {
			c.Next()
			return
		}
		if isControlPlaneRequest(c) {
			c.Next()
			return
		}
		if auth.ProjectID == "" || auth.TokenSource == "bootstrap_admin" {
			c.Next()
			return
		}
		if appStore == nil {
			c.Next()
			return
		}
		budgets, cached := budgetCache.Get(auth.ProjectID)
		if !cached {
			fresh, err := appStore.ListBudgets(c.Request.Context(), auth.ProjectID)
			if err != nil {
				logger.Warn("budget lookup failed", "project_id", auth.ProjectID, "error", err)
				c.Next()
				return
			}
			budgets = fresh
			budgetCache.Set(auth.ProjectID, budgets)
		}
		for _, budget := range budgets {
			if budget.Mode != store.BudgetModeHard {
				continue
			}
			filter := store.UsageFilter{ProjectID: auth.ProjectID}
			from, to := budgetWindowRange(time.Now().UTC(), budget.Window)
			if !from.IsZero() {
				filter.From = &from
			}
			if !to.IsZero() {
				filter.To = &to
			}
			report, err := appStore.GetUsage(c.Request.Context(), filter)
			if err != nil {
				logger.Warn("budget usage lookup failed", "project_id", auth.ProjectID, "budget_id", budget.ID, "error", err)
				continue
			}
			exceeded := (budget.LimitUSD > 0 && report.TotalCost >= budget.LimitUSD) ||
				(budget.LimitRequests > 0 && report.TotalRequests >= budget.LimitRequests)
			if !exceeded {
				continue
			}
			span.SetAttributes(
				attribute.String("polaris.project_id", auth.ProjectID),
				attribute.String("polaris.budget_id", budget.ID),
				attribute.Bool("polaris.budget.denied", true),
			)

			if recorder != nil {
				recorder.IncBudgetDenial(auth.ProjectID)
			}
			if auditLogger != nil {
				payload, _ := json.Marshal(map[string]any{
					"budget_id":      budget.ID,
					"budget_name":    budget.Name,
					"limit_usd":      budget.LimitUSD,
					"limit_requests": budget.LimitRequests,
					"window":         budget.Window,
					"total_cost_usd": report.TotalCost,
					"total_requests": report.TotalRequests,
					"request_path":   c.FullPath(),
					"request_method": c.Request.Method,
				})
				auditLogger.Log(store.AuditEvent{
					ProjectID:    auth.ProjectID,
					ActorKeyID:   auth.KeyID,
					Kind:         "budget.denied",
					ResourceType: "budget",
					ResourceID:   budget.ID,
					MetadataJSON: string(payload),
					CreatedAt:    time.Now().UTC(),
				})
			}
			httputil.WriteError(c, httputil.NewError(http.StatusTooManyRequests, "budget_exceeded", "budget_exceeded", "", "Project hard budget has been exceeded."))
			return
		}
		c.Next()
	}
}

func budgetWindowRange(now time.Time, window string) (time.Time, time.Time) {
	switch strings.ToLower(strings.TrimSpace(window)) {
	case "", "monthly":
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		return start, now.Add(time.Second)
	case "daily":
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		return start, now.Add(time.Second)
	case "lifetime":
		return time.Time{}, time.Time{}
	default:
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		return start, now.Add(time.Second)
	}
}
