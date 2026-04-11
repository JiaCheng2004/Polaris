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
			"method", c.Request.Method,
			"path", c.FullPath(),
			"status", c.Writer.Status(),
			"latency_ms", time.Since(start).Milliseconds(),
			"key_id", auth.KeyID,
			"key_prefix", auth.KeyPrefix,
			"model", outcome.Model,
			"provider", outcome.Provider,
			"modality", outcome.Modality,
			"tokens", outcome.TotalTokens,
			"error_type", outcome.ErrorType,
		)
	}
}
