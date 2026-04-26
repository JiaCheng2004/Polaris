package middleware

import (
	"net/http"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/gin-gonic/gin"
)

func BodyLimit(runtime *gwruntime.Holder) gin.HandlerFunc {
	return func(c *gin.Context) {
		maxBytes := config.DefaultMaxBodyBytes
		if snapshot := RuntimeSnapshot(c, runtime); snapshot != nil && snapshot.Config != nil {
			maxBytes = config.EffectiveMaxBodyBytes(snapshot.Config.Server.MaxBodyBytes)
		}

		if c.Request.ContentLength > maxBytes {
			httputil.WriteError(c, httputil.RequestBodyTooLargeError(maxBytes))
			return
		}
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		}
		c.Next()
	}
}
