package gateway

import (
	"github.com/JiaCheng2004/Polaris/internal/gateway/handler"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/gin-gonic/gin"
)

func registerRoutes(engine *gin.Engine, deps Dependencies) {
	engine.Use(
		middleware.Recovery(deps.Logger),
		middleware.RequestID(),
		middleware.CORS(),
		middleware.Logger(deps.Logger),
	)

	healthHandler := handler.NewHealthHandler(deps.Store, deps.Cache, deps.Registry)
	chatHandler := handler.NewChatHandler(deps.Registry)
	modelsHandler := handler.NewModelsHandler(deps.Registry)
	usageHandler := handler.NewUsageHandler(deps.Store)

	engine.GET("/health", healthHandler.Liveness)
	engine.GET("/ready", healthHandler.Readiness)

	v1 := engine.Group("/v1")
	v1.Use(
		middleware.Auth(deps.Config, deps.Logger),
		middleware.RateLimit(deps.Config, deps.Cache, deps.Logger),
		middleware.Usage(deps.RequestLogger, deps.Logger),
	)
	v1.GET("/models", modelsHandler.List)
	v1.GET("/usage", usageHandler.Get)
	v1.POST("/chat/completions", chatHandler.Complete)
}
