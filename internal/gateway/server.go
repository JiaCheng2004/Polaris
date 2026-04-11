package gateway

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/JiaCheng2004/Polaris/internal/store/cache"
	"github.com/gin-gonic/gin"
)

type Dependencies struct {
	Config        *config.Config
	Logger        *slog.Logger
	Store         store.Store
	Cache         cache.Cache
	Registry      *provider.Registry
	RequestLogger *store.AsyncRequestLogger
}

func NewEngine(deps Dependencies) (*gin.Engine, error) {
	if deps.Config == nil {
		return nil, fmt.Errorf("config is required")
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
