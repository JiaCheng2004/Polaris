package gateway

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/metrics"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/JiaCheng2004/Polaris/internal/store/cache"
	"github.com/JiaCheng2004/Polaris/internal/tooling"
	"github.com/gin-gonic/gin"
)

type Dependencies struct {
	Config          *config.Config
	Logger          *slog.Logger
	Store           store.Store
	Cache           cache.Cache
	Registry        *provider.Registry
	Runtime         *gwruntime.Holder
	Metrics         *metrics.Recorder
	RequestLogger   *store.AsyncRequestLogger
	AuthCache       *middleware.APIKeyCache
	VirtualKeyCache *middleware.VirtualKeyCache
	AuditLogger     *store.AsyncAuditLogger
	ToolRegistry    *tooling.Registry
}

func NewEngine(deps Dependencies) (*gin.Engine, error) {
	if deps.Config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if deps.Runtime == nil {
		deps.Runtime = gwruntime.NewHolder(deps.Config, deps.Registry)
	}
	if deps.Metrics == nil {
		deps.Metrics = metrics.NewRecorder()
	}
	if deps.AuthCache == nil {
		deps.AuthCache = middleware.NewAPIKeyCache(60 * time.Second)
	}
	if deps.VirtualKeyCache == nil {
		deps.VirtualKeyCache = middleware.NewVirtualKeyCache(60 * time.Second)
	}
	if deps.ToolRegistry == nil {
		deps.ToolRegistry = tooling.NewRegistry()
	}

	engine := gin.New()
	engine.HandleMethodNotAllowed = true
	registerRoutes(engine, deps)
	return engine, nil
}

func NewHTTPServer(deps Dependencies) (*http.Server, error) {
	engine, err := NewEngine(deps)
	if err != nil {
		return nil, err
	}

	return &http.Server{
		Addr:         deps.Config.Address(),
		Handler:      engine,
		ReadTimeout:  deps.Config.Server.ReadTimeout,
		WriteTimeout: deps.Config.Server.WriteTimeout,
	}, nil
}
