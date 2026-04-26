package middleware

import (
	"net/http"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/gin-gonic/gin"
)

func MCPEnabled(runtime *gwruntime.Holder) gin.HandlerFunc {
	return func(c *gin.Context) {
		snapshot := RuntimeSnapshot(c, runtime)
		if snapshot == nil || snapshot.Config == nil {
			httputil.WriteError(c, httputil.NewError(http.StatusInternalServerError, "internal_error", "runtime_unavailable", "", "Runtime configuration is unavailable."))
			return
		}
		if !snapshot.Config.MCP.Enabled {
			httputil.WriteError(c, httputil.NewError(http.StatusNotFound, "invalid_request_error", "mcp_disabled", "", "MCP broker endpoints are disabled."))
			return
		}
		c.Next()
	}
}
