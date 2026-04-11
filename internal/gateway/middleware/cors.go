package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		headers := c.Writer.Header()
		headers.Set("Access-Control-Allow-Origin", "*")
		headers.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
		headers.Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		headers.Set("Access-Control-Expose-Headers", "X-Request-ID, X-RateLimit-Remaining, Retry-After, X-Polaris-Fallback")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
