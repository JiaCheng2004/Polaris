package gateway

import (
	"net/http"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/JiaCheng2004/Polaris/internal/mcp"
	"github.com/gin-gonic/gin"
)

func registerMCPRoutes(engine *gin.Engine, deps Dependencies, handlers routeHandlers) {
	apiKeyAuth := middleware.Auth(deps.Runtime, deps.Store, deps.AuthCache, deps.VirtualKeyCache, deps.Logger)

	mcpAuth := apiKeyAuth
	if mcpConfig := deps.Config.MCP; sliceContains(mcpConfig.Auth, "oauth") && mcpConfig.OAuth.ResourceURI != "" {
		rs := mcp.NewResourceServer(mcp.OAuthConfig{
			ResourceURI:          mcpConfig.OAuth.ResourceURI,
			AuthorizationServers: mcpConfig.OAuth.AuthorizationServers,
			JWKSCacheTTL:         mcpConfig.OAuth.JWKSCacheTTL,
		}, nil)
		var fallback gin.HandlerFunc
		if sliceContains(mcpConfig.Auth, "api_key") {
			fallback = apiKeyAuth
		}
		metadataURL := metadataURLFor(mcpConfig.OAuth.ResourceURI)
		engine.GET("/.well-known/oauth-protected-resource", func(c *gin.Context) {
			c.JSON(http.StatusOK, rs.ProtectedResourceMetadata())
		})
		mcpAuth = middleware.MCPOAuth(rs, metadataURL, fallback)
	}

	group := engine.Group("/mcp")
	group.Use(
		middleware.MCPEnabled(deps.Runtime),
		mcpAuth,
		middleware.Budget(deps.Runtime, deps.Store, deps.Metrics, deps.AuditLogger, deps.Logger),
	)
	group.GET("", handlers.mcp.ServeAggregate)
	group.POST("", handlers.mcp.ServeAggregate)
	group.Any("/:binding_id/*path", handlers.mcp.Serve)
	group.Any("/:binding_id", handlers.mcp.Serve)
}

func sliceContains(s []string, v string) bool {
	for _, item := range s {
		if item == v {
			return true
		}
	}
	return false
}

func metadataURLFor(resourceURI string) string {
	base := strings.TrimRight(resourceURI, "/")
	if i := strings.LastIndex(base, "/"); i > strings.Index(base, "://")+2 {
		base = base[:i]
	}
	return base + "/.well-known/oauth-protected-resource"
}
