package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

func Logger(logger *slog.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}

	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		auth := GetAuthContext(c)
		outcome, _ := GetRequestOutcome(c)
		logger.Info("http request",
			"request_id", GetRequestID(c),
			"trace_id", GetTraceID(c),
			"method", c.Request.Method,
			"path", c.FullPath(),
			"status", c.Writer.Status(),
			"latency_ms", time.Since(start).Milliseconds(),
			"project_id", auth.ProjectID,
			"key_id", auth.KeyID,
			"key_prefix", auth.KeyPrefix,
			"auth_source", auth.TokenSource,
			"model", outcome.Model,
			"provider", outcome.Provider,
			"modality", outcome.Modality,
			"interface_family", firstNonEmpty(outcome.InterfaceFamily, interfaceFamilyFromPath(c.FullPath())),
			"token_source", usageTokenSource(outcome),
			"cache_status", firstNonEmpty(outcome.CacheStatus, c.Writer.Header().Get("X-Polaris-Cache")),
			"fallback_model", firstNonEmpty(outcome.FallbackModel, c.Writer.Header().Get("X-Polaris-Fallback")),
			"toolset", outcome.Toolset,
			"mcp_binding", outcome.MCPBinding,
			"tokens", outcome.TotalTokens,
			"error_type", outcome.ErrorType,
		)
	}
}
