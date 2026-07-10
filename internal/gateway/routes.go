package gateway

import (
	"github.com/JiaCheng2004/Polaris/internal/gateway/handler"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/gin-gonic/gin"
)

type routeHandlers struct {
	adminAnalytics *handler.AdminAnalyticsHandler
	audio          *handler.AudioHandler
	chat           *handler.ChatHandler
	batches        *handler.BatchesHandler
	controlPlane   *handler.ControlPlaneHandler
	embed          *handler.EmbedHandler
	files          *handler.FilesHandler
	health         *handler.HealthHandler
	image          *handler.ImageHandler
	interpreting   *handler.InterpretingHandler
	keys           *handler.KeysHandler
	mcp            *handler.MCPHandler
	metrics        *handler.MetricsHandler
	models         *handler.ModelsHandler
	music          *handler.MusicHandler
	notes          *handler.NotesHandler
	podcast        *handler.PodcastHandler
	tokens         *handler.TokensHandler
	translation    *handler.TranslationHandler
	usage          *handler.UsageHandler
	video          *handler.VideoHandler
	voice          *handler.VoiceHandler
	voices         *handler.VoicesHandler
	// idempotency builds the per-route idempotency middleware for a job-submit
	// endpoint. All routes share one coordinator so concurrent duplicates of a
	// key serialize regardless of endpoint.
	idempotency func(endpoint string) gin.HandlerFunc
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
		middleware.Shed(deps.Reliability),
	)

	handlers := buildRouteHandlers(deps)
	registerCoreRoutes(engine, deps, handlers)
	registerRealtimeRoutes(engine, deps, handlers)
	registerMCPRoutes(engine, deps, handlers)
	registerV1Routes(engine, deps, handlers)
}

func buildRouteHandlers(deps Dependencies) routeHandlers {
	chatHandler := handler.NewChatHandler(deps.Runtime, deps.Metrics, deps.Cache, deps.Store, deps.Reliability)
	idempotencyCoord := middleware.NewIdempotencyCoordinator()
	return routeHandlers{
		idempotency: func(endpoint string) gin.HandlerFunc {
			return middleware.Idempotency(deps.Runtime, deps.Store, idempotencyCoord, deps.Metrics, endpoint)
		},
		adminAnalytics: handler.NewAdminAnalyticsHandler(deps.Store),
		audio:          handler.NewAudioHandler(deps.Runtime, deps.StreamDrainer),
		batches:        handler.NewBatchesHandler(deps.Runtime, deps.Store),
		chat:           chatHandler,
		controlPlane:   handler.NewControlPlaneHandler(deps.Runtime, deps.Store, deps.VirtualKeyCache, deps.AuditLogger, deps.ToolRegistry),
		embed:          handler.NewEmbedHandler(deps.Runtime, deps.Cache),
		files:          handler.NewFilesHandler(deps.Runtime, deps.Store, deps.AuditLogger, deps.Logger),
		health:         handler.NewHealthHandler(deps.Store, deps.Cache, deps.Runtime, deps.StreamDrainer, deps.Reliability),
		image:          handler.NewImageHandler(deps.Runtime, deps.Cache),
		interpreting:   handler.NewInterpretingHandler(deps.Runtime, deps.StreamDrainer),
		keys:           handler.NewKeysHandler(deps.Runtime, deps.Store, deps.AuthCache, deps.VirtualKeyCache),
		mcp:            handler.NewMCPHandler(deps.Runtime, deps.Store, deps.ToolRegistry, deps.Metrics),
		metrics:        handler.NewMetricsHandler(deps.Runtime, deps.Metrics),
		models:         handler.NewModelsHandler(deps.Runtime),
		music:          handler.NewMusicHandler(deps.Runtime, deps.Cache, deps.RequestLogger),
		notes:          handler.NewNotesHandler(deps.Runtime),
		podcast:        handler.NewPodcastHandler(deps.Runtime, deps.Cache, deps.RequestLogger),
		tokens:         handler.NewTokensHandler(chatHandler),
		translation:    handler.NewTranslationHandler(deps.Runtime),
		usage:          handler.NewUsageHandler(deps.Store),
		video:          handler.NewVideoHandler(deps.Runtime),
		voice:          handler.NewVoiceHandler(deps.Runtime, deps.Cache, deps.StreamDrainer),
		voices:         handler.NewVoicesHandler(deps.Runtime, deps.Store),
	}
}
