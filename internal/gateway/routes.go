package gateway

import (
	"github.com/JiaCheng2004/Polaris/internal/gateway/handler"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/gin-gonic/gin"
)

type routeHandlers struct {
	audio        *handler.AudioHandler
	chat         *handler.ChatHandler
	controlPlane *handler.ControlPlaneHandler
	embed        *handler.EmbedHandler
	health       *handler.HealthHandler
	image        *handler.ImageHandler
	interpreting *handler.InterpretingHandler
	keys         *handler.KeysHandler
	mcp          *handler.MCPHandler
	metrics      *handler.MetricsHandler
	models       *handler.ModelsHandler
	music        *handler.MusicHandler
	notes        *handler.NotesHandler
	podcast      *handler.PodcastHandler
	tokens       *handler.TokensHandler
	translation  *handler.TranslationHandler
	usage        *handler.UsageHandler
	video        *handler.VideoHandler
	voice        *handler.VoiceHandler
	voices       *handler.VoicesHandler
}

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

	handlers := buildRouteHandlers(deps)
	registerCoreRoutes(engine, deps, handlers)
	registerRealtimeRoutes(engine, deps, handlers)
	registerMCPRoutes(engine, deps, handlers)
	registerV1Routes(engine, deps, handlers)
}

func buildRouteHandlers(deps Dependencies) routeHandlers {
	chatHandler := handler.NewChatHandler(deps.Runtime, deps.Metrics, deps.Cache)
	return routeHandlers{
		audio:        handler.NewAudioHandler(deps.Runtime),
		chat:         chatHandler,
		controlPlane: handler.NewControlPlaneHandler(deps.Runtime, deps.Store, deps.VirtualKeyCache, deps.AuditLogger, deps.ToolRegistry),
		embed:        handler.NewEmbedHandler(deps.Runtime, deps.Cache),
		health:       handler.NewHealthHandler(deps.Store, deps.Cache, deps.Runtime),
		image:        handler.NewImageHandler(deps.Runtime, deps.Cache),
		interpreting: handler.NewInterpretingHandler(deps.Runtime),
		keys:         handler.NewKeysHandler(deps.Runtime, deps.Store, deps.AuthCache, deps.VirtualKeyCache),
		mcp:          handler.NewMCPHandler(deps.Runtime, deps.Store, deps.ToolRegistry, deps.Metrics),
		metrics:      handler.NewMetricsHandler(deps.Runtime, deps.Metrics),
		models:       handler.NewModelsHandler(deps.Runtime),
		music:        handler.NewMusicHandler(deps.Runtime, deps.Cache, deps.RequestLogger),
		notes:        handler.NewNotesHandler(deps.Runtime),
		podcast:      handler.NewPodcastHandler(deps.Runtime, deps.Cache, deps.RequestLogger),
		tokens:       handler.NewTokensHandler(chatHandler),
		translation:  handler.NewTranslationHandler(deps.Runtime),
		usage:        handler.NewUsageHandler(deps.Store),
		video:        handler.NewVideoHandler(deps.Runtime),
		voice:        handler.NewVoiceHandler(deps.Runtime, deps.Cache),
		voices:       handler.NewVoicesHandler(deps.Runtime, deps.Store),
	}
}
