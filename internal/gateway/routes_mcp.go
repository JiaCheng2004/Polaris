package gateway

import (
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/gin-gonic/gin"
)

func registerMCPRoutes(engine *gin.Engine, deps Dependencies, handlers routeHandlers) {
	mcp := engine.Group("/mcp")
	mcp.Use(
		middleware.MCPEnabled(deps.Runtime),
		middleware.Auth(deps.Runtime, deps.Store, deps.AuthCache, deps.VirtualKeyCache, deps.Logger),
		middleware.Budget(deps.Runtime, deps.Store, deps.Metrics, deps.AuditLogger, deps.Logger),
	)
	mcp.Any("/:binding_id/*path", handlers.mcp.Serve)
	mcp.Any("/:binding_id", handlers.mcp.Serve)
}
