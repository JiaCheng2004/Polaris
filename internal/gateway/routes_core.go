package gateway

import "github.com/gin-gonic/gin"

func registerCoreRoutes(engine *gin.Engine, deps Dependencies, handlers routeHandlers) {
	engine.GET("/health", handlers.health.Liveness)
	engine.GET("/ready", handlers.health.Readiness)
	engine.GET(deps.Config.Observability.Metrics.Path, handlers.metrics.Serve)
}
