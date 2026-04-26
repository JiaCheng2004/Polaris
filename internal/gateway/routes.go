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
		middleware.Tracing(),
		middleware.Runtime(deps.Runtime),
		middleware.BodyLimit(deps.Runtime),
		middleware.CORS(deps.Runtime),
		middleware.Logger(deps.Logger),
		middleware.Metrics(deps.Metrics),
	)

	healthHandler := handler.NewHealthHandler(deps.Store, deps.Cache, deps.Runtime)
	chatHandler := handler.NewChatHandler(deps.Runtime, deps.Metrics, deps.Cache)
	controlPlaneHandler := handler.NewControlPlaneHandler(deps.Runtime, deps.Store, deps.VirtualKeyCache, deps.AuditLogger, deps.ToolRegistry)
	embedHandler := handler.NewEmbedHandler(deps.Runtime, deps.Cache)
	imageHandler := handler.NewImageHandler(deps.Runtime, deps.Cache)
	interpretingHandler := handler.NewInterpretingHandler(deps.Runtime)
	keysHandler := handler.NewKeysHandler(deps.Runtime, deps.Store, deps.AuthCache, deps.VirtualKeyCache)
	mcpHandler := handler.NewMCPHandler(deps.Runtime, deps.Store, deps.ToolRegistry, deps.Metrics)
	musicHandler := handler.NewMusicHandler(deps.Runtime, deps.Cache, deps.RequestLogger)
	metricsHandler := handler.NewMetricsHandler(deps.Runtime, deps.Metrics)
	modelsHandler := handler.NewModelsHandler(deps.Runtime)
	notesHandler := handler.NewNotesHandler(deps.Runtime)
	podcastHandler := handler.NewPodcastHandler(deps.Runtime, deps.Cache, deps.RequestLogger)
	tokensHandler := handler.NewTokensHandler(chatHandler)
	translationHandler := handler.NewTranslationHandler(deps.Runtime)
	usageHandler := handler.NewUsageHandler(deps.Store)
	videoHandler := handler.NewVideoHandler(deps.Runtime)
	voiceHandler := handler.NewVoiceHandler(deps.Runtime, deps.Cache)
	voicesHandler := handler.NewVoicesHandler(deps.Runtime, deps.Store)
	audioHandler := handler.NewAudioHandler(deps.Runtime)

	engine.GET("/health", healthHandler.Liveness)
	engine.GET("/ready", healthHandler.Readiness)
	engine.GET(deps.Config.Observability.Metrics.Path, metricsHandler.Serve)

	audioSessions := engine.Group("/v1/audio/sessions")
	audioSessions.Use(middleware.Usage(deps.RequestLogger, deps.Logger))
	audioSessions.GET("/:id/ws", audioHandler.WebSocket)

	interpretingSessions := engine.Group("/v1/audio/interpreting/sessions")
	interpretingSessions.Use(middleware.Usage(deps.RequestLogger, deps.Logger))
	interpretingSessions.GET("/:id/ws", interpretingHandler.WebSocket)

	streamingTranscriptions := engine.Group("/v1/audio/transcriptions/stream")
	streamingTranscriptions.Use(middleware.Usage(deps.RequestLogger, deps.Logger))
	streamingTranscriptions.GET("/:id/ws", voiceHandler.StreamingTranscriptionWebSocket)

	mcp := engine.Group("/mcp")
	mcp.Use(
		middleware.MCPEnabled(deps.Runtime),
		middleware.Auth(deps.Runtime, deps.Store, deps.AuthCache, deps.VirtualKeyCache, deps.Logger),
		middleware.Budget(deps.Runtime, deps.Store, deps.Metrics, deps.AuditLogger, deps.Logger),
	)
	mcp.Any("/:binding_id/*path", mcpHandler.Serve)
	mcp.Any("/:binding_id", mcpHandler.Serve)

	v1 := engine.Group("/v1")
	v1.Use(
		middleware.Auth(deps.Runtime, deps.Store, deps.AuthCache, deps.VirtualKeyCache, deps.Logger),
		middleware.RateLimit(deps.Runtime, deps.Cache, deps.Logger, deps.Metrics),
		middleware.Budget(deps.Runtime, deps.Store, deps.Metrics, deps.AuditLogger, deps.Logger),
		middleware.Usage(deps.RequestLogger, deps.Logger),
	)
	v1.POST("/keys", keysHandler.Create)
	v1.GET("/keys", keysHandler.List)
	v1.DELETE("/keys/:id", keysHandler.Delete)
	v1.GET("/models", modelsHandler.List)
	v1.GET("/voices", voicesHandler.List)
	v1.GET("/usage", usageHandler.Get)
	v1.POST("/chat/completions", chatHandler.Complete)
	v1.POST("/responses", chatHandler.Responses)
	v1.POST("/messages", chatHandler.Messages)
	v1.POST("/embeddings", embedHandler.Create)
	v1.POST("/tokens/count", tokensHandler.Count)
	v1.POST("/translations", translationHandler.Translate)
	v1.GET("/voices/:id", voicesHandler.Get)
	v1.DELETE("/voices/:id", voicesHandler.Delete)
	v1.POST("/voices/:id/archive", voicesHandler.Archive)
	v1.POST("/voices/:id/unarchive", voicesHandler.Unarchive)
	v1.POST("/voices/clones", voicesHandler.CreateClone)
	v1.POST("/voices/designs", voicesHandler.CreateDesign)
	v1.POST("/voices/:id/retrain", voicesHandler.Retrain)
	v1.POST("/voices/:id/activate", voicesHandler.Activate)
	v1.POST("/images/generations", imageHandler.Generate)
	v1.POST("/images/edits", imageHandler.Edit)
	v1.POST("/music/generations", musicHandler.Generate)
	v1.POST("/music/edits", musicHandler.Edit)
	v1.POST("/music/stems", musicHandler.Stems)
	v1.POST("/music/lyrics", musicHandler.Lyrics)
	v1.POST("/music/plans", musicHandler.Plans)
	v1.GET("/music/jobs/:id", musicHandler.GetJob)
	v1.GET("/music/jobs/:id/content", musicHandler.GetJobContent)
	v1.DELETE("/music/jobs/:id", musicHandler.CancelJob)
	v1.POST("/video/generations", videoHandler.Generate)
	v1.GET("/video/generations/:id", videoHandler.Get)
	v1.GET("/video/generations/:id/content", videoHandler.Content)
	v1.DELETE("/video/generations/:id", videoHandler.Cancel)
	v1.POST("/audio/speech", voiceHandler.Speech)
	v1.POST("/audio/transcriptions", voiceHandler.Transcribe)
	v1.POST("/audio/transcriptions/stream", voiceHandler.CreateStreamingTranscriptionSession)
	v1.POST("/audio/notes", notesHandler.Create)
	v1.GET("/audio/notes/:id", notesHandler.Get)
	v1.DELETE("/audio/notes/:id", notesHandler.Delete)
	v1.POST("/audio/podcasts", podcastHandler.Create)
	v1.GET("/audio/podcasts/:id", podcastHandler.Get)
	v1.GET("/audio/podcasts/:id/content", podcastHandler.Content)
	v1.DELETE("/audio/podcasts/:id", podcastHandler.Cancel)
	v1.POST("/audio/interpreting/sessions", interpretingHandler.Create)
	v1.POST("/audio/sessions", audioHandler.Create)

	control := v1.Group("")
	control.Use(
		middleware.ControlPlaneEnabled(deps.Runtime),
		middleware.ControlPlaneAdmin(deps.Runtime),
	)
	control.POST("/projects", controlPlaneHandler.CreateProject)
	control.GET("/projects", controlPlaneHandler.ListProjects)
	control.POST("/virtual_keys", controlPlaneHandler.CreateVirtualKey)
	control.GET("/virtual_keys", controlPlaneHandler.ListVirtualKeys)
	control.DELETE("/virtual_keys/:id", controlPlaneHandler.DeleteVirtualKey)
	control.POST("/policies", controlPlaneHandler.CreatePolicy)
	control.GET("/policies", controlPlaneHandler.ListPolicies)
	control.POST("/budgets", controlPlaneHandler.CreateBudget)
	control.GET("/budgets", controlPlaneHandler.ListBudgets)
	control.POST("/tools", controlPlaneHandler.CreateTool)
	control.GET("/tools", controlPlaneHandler.ListTools)
	control.POST("/toolsets", controlPlaneHandler.CreateToolset)
	control.GET("/toolsets", controlPlaneHandler.ListToolsets)
	control.POST("/mcp/bindings", controlPlaneHandler.CreateMCPBinding)
	control.GET("/mcp/bindings", controlPlaneHandler.ListMCPBindings)
}
