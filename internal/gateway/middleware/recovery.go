package middleware

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/gin-gonic/gin"
)

func Recovery(logger *slog.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}

	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("panic recovered", "request_id", GetRequestID(c), "panic", fmt.Sprint(recovered))
				httputil.WriteError(c, httputil.NewError(http.StatusInternalServerError, "internal_error", "internal_error", "", "An internal error occurred."))
			}
		}()
		c.Next()
	}
}
