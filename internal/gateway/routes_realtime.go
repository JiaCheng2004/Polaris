package gateway

import (
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/gin-gonic/gin"
)

func registerRealtimeRoutes(engine *gin.Engine, deps Dependencies, handlers routeHandlers) {
	audioSessions := engine.Group("/v1/audio/sessions")
	audioSessions.Use(middleware.Usage(deps.RequestLogger, deps.Logger))
	audioSessions.GET("/:id/ws", handlers.audio.WebSocket)

	interpretingSessions := engine.Group("/v1/audio/interpreting/sessions")
	interpretingSessions.Use(middleware.Usage(deps.RequestLogger, deps.Logger))
	interpretingSessions.GET("/:id/ws", handlers.interpreting.WebSocket)

	streamingTranscriptions := engine.Group("/v1/audio/transcriptions/stream")
	streamingTranscriptions.Use(middleware.Usage(deps.RequestLogger, deps.Logger))
	streamingTranscriptions.GET("/:id/ws", handlers.voice.StreamingTranscriptionWebSocket)
}
