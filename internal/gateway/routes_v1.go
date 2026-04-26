package gateway

import (
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/gin-gonic/gin"
)

func registerV1Routes(engine *gin.Engine, deps Dependencies, handlers routeHandlers) {
	v1 := engine.Group("/v1")
	v1.Use(
		middleware.Auth(deps.Runtime, deps.Store, deps.AuthCache, deps.VirtualKeyCache, deps.Logger),
		middleware.RateLimit(deps.Runtime, deps.Cache, deps.Logger, deps.Metrics),
		middleware.Budget(deps.Runtime, deps.Store, deps.Metrics, deps.AuditLogger, deps.Logger),
		middleware.Usage(deps.RequestLogger, deps.Logger),
	)

	registerV1CoreRoutes(v1, handlers)
	registerV1ConversationRoutes(v1, handlers)
	registerV1MediaRoutes(v1, handlers)
	registerV1VoiceAudioRoutes(v1, handlers)
	registerV1ControlPlaneRoutes(v1, deps, handlers)
}

func registerV1CoreRoutes(v1 *gin.RouterGroup, handlers routeHandlers) {
	v1.POST("/keys", handlers.keys.Create)
	v1.GET("/keys", handlers.keys.List)
	v1.DELETE("/keys/:id", handlers.keys.Delete)
	v1.GET("/models", handlers.models.List)
	v1.GET("/usage", handlers.usage.Get)
	v1.POST("/tokens/count", handlers.tokens.Count)
}

func registerV1ConversationRoutes(v1 *gin.RouterGroup, handlers routeHandlers) {
	v1.POST("/chat/completions", handlers.chat.Complete)
	v1.POST("/responses", handlers.chat.Responses)
	v1.POST("/messages", handlers.chat.Messages)
	v1.POST("/embeddings", handlers.embed.Create)
	v1.POST("/translations", handlers.translation.Translate)
}

func registerV1MediaRoutes(v1 *gin.RouterGroup, handlers routeHandlers) {
	v1.POST("/images/generations", handlers.image.Generate)
	v1.POST("/images/edits", handlers.image.Edit)
	v1.POST("/music/generations", handlers.music.Generate)
	v1.POST("/music/edits", handlers.music.Edit)
	v1.POST("/music/stems", handlers.music.Stems)
	v1.POST("/music/lyrics", handlers.music.Lyrics)
	v1.POST("/music/plans", handlers.music.Plans)
	v1.GET("/music/jobs/:id", handlers.music.GetJob)
	v1.GET("/music/jobs/:id/content", handlers.music.GetJobContent)
	v1.DELETE("/music/jobs/:id", handlers.music.CancelJob)
	v1.POST("/video/generations", handlers.video.Generate)
	v1.GET("/video/generations/:id", handlers.video.Get)
	v1.GET("/video/generations/:id/content", handlers.video.Content)
	v1.DELETE("/video/generations/:id", handlers.video.Cancel)
}

func registerV1VoiceAudioRoutes(v1 *gin.RouterGroup, handlers routeHandlers) {
	v1.GET("/voices", handlers.voices.List)
	v1.GET("/voices/:id", handlers.voices.Get)
	v1.DELETE("/voices/:id", handlers.voices.Delete)
	v1.POST("/voices/:id/archive", handlers.voices.Archive)
	v1.POST("/voices/:id/unarchive", handlers.voices.Unarchive)
	v1.POST("/voices/clones", handlers.voices.CreateClone)
	v1.POST("/voices/designs", handlers.voices.CreateDesign)
	v1.POST("/voices/:id/retrain", handlers.voices.Retrain)
	v1.POST("/voices/:id/activate", handlers.voices.Activate)
	v1.POST("/audio/speech", handlers.voice.Speech)
	v1.POST("/audio/transcriptions", handlers.voice.Transcribe)
	v1.POST("/audio/transcriptions/stream", handlers.voice.CreateStreamingTranscriptionSession)
	v1.POST("/audio/notes", handlers.notes.Create)
	v1.GET("/audio/notes/:id", handlers.notes.Get)
	v1.DELETE("/audio/notes/:id", handlers.notes.Delete)
	v1.POST("/audio/podcasts", handlers.podcast.Create)
	v1.GET("/audio/podcasts/:id", handlers.podcast.Get)
	v1.GET("/audio/podcasts/:id/content", handlers.podcast.Content)
	v1.DELETE("/audio/podcasts/:id", handlers.podcast.Cancel)
	v1.POST("/audio/interpreting/sessions", handlers.interpreting.Create)
	v1.POST("/audio/sessions", handlers.audio.Create)
}

func registerV1ControlPlaneRoutes(v1 *gin.RouterGroup, deps Dependencies, handlers routeHandlers) {
	control := v1.Group("")
	control.Use(
		middleware.ControlPlaneEnabled(deps.Runtime),
		middleware.ControlPlaneAdmin(deps.Runtime),
	)
	control.POST("/projects", handlers.controlPlane.CreateProject)
	control.GET("/projects", handlers.controlPlane.ListProjects)
	control.POST("/virtual_keys", handlers.controlPlane.CreateVirtualKey)
	control.GET("/virtual_keys", handlers.controlPlane.ListVirtualKeys)
	control.DELETE("/virtual_keys/:id", handlers.controlPlane.DeleteVirtualKey)
	control.POST("/policies", handlers.controlPlane.CreatePolicy)
	control.GET("/policies", handlers.controlPlane.ListPolicies)
	control.POST("/budgets", handlers.controlPlane.CreateBudget)
	control.GET("/budgets", handlers.controlPlane.ListBudgets)
	control.POST("/tools", handlers.controlPlane.CreateTool)
	control.GET("/tools", handlers.controlPlane.ListTools)
	control.POST("/toolsets", handlers.controlPlane.CreateToolset)
	control.GET("/toolsets", handlers.controlPlane.ListToolsets)
	control.POST("/mcp/bindings", handlers.controlPlane.CreateMCPBinding)
	control.GET("/mcp/bindings", handlers.controlPlane.ListMCPBindings)
}
