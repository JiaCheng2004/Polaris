package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/JiaCheng2004/Polaris/internal/store/cache"
	"github.com/JiaCheng2004/Polaris/internal/store/sqlite"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "polaris: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", config.DefaultConfigPath(), "Path to polaris config")
	port := flag.Int("port", 0, "Override listen port")
	logLevel := flag.String("log-level", "", "Override log level")
	flag.Parse()

	cfg, warnings, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	config.ApplyRuntimeOverrides(cfg, config.RuntimeOverrides{
		Port:     *port,
		LogLevel: *logLevel,
	})

	logger := newLogger(cfg)
	slog.SetDefault(logger)

	for _, warning := range warnings {
		logger.Warn("config warning", "warning", warning)
	}

	appStore, err := bootstrapStore(cfg)
	if err != nil {
		return err
	}
	defer appStore.Close()

	if err := appStore.Migrate(context.Background()); err != nil {
		return err
	}

	appCache, err := bootstrapCache(cfg)
	if err != nil {
		return err
	}
	defer appCache.Close()

	registry, registryWarnings, err := provider.New(cfg)
	if err != nil {
		return err
	}
	for _, warning := range registryWarnings {
		logger.Warn("registry warning", "warning", warning)
	}

	requestLogger := store.NewAsyncRequestLogger(appStore, logger, store.NewLoggerConfig(cfg.Store.LogBufferSize, cfg.Store.LogFlushInterval))
	defer requestLogger.Close(context.Background())

	server, err := gateway.NewHTTPServer(gateway.Dependencies{
		Config:        cfg,
		Logger:        logger,
		Store:         appStore,
		Cache:         appCache,
		Registry:      registry,
		RequestLogger: requestLogger,
	})
	if err != nil {
		return err
	}

	go func() {
		logger.Info("starting polaris", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server exited", "error", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()
	return server.Shutdown(ctx)
}

func bootstrapStore(cfg *config.Config) (store.Store, error) {
	switch cfg.Store.Driver {
	case "sqlite":
		return sqlite.New(cfg.Store)
	default:
		return nil, fmt.Errorf("store driver %q is not implemented in this runtime kernel", cfg.Store.Driver)
	}
}

func bootstrapCache(cfg *config.Config) (cache.Cache, error) {
	switch cfg.Cache.Driver {
	case "memory":
		return cache.NewMemory(), nil
	default:
		return nil, fmt.Errorf("cache driver %q is not implemented in this runtime kernel", cfg.Cache.Driver)
	}
}

func newLogger(cfg *config.Config) *slog.Logger {
	level := new(slog.LevelVar)
	switch strings.ToLower(cfg.Observability.Logging.Level) {
	case "debug":
		level.Set(slog.LevelDebug)
	case "warn":
		level.Set(slog.LevelWarn)
	case "error":
		level.Set(slog.LevelError)
	default:
		level.Set(slog.LevelInfo)
	}

	options := &slog.HandlerOptions{Level: level}
	switch strings.ToLower(cfg.Observability.Logging.Format) {
	case "text":
		return slog.New(slog.NewTextHandler(os.Stdout, options))
	default:
		return slog.New(slog.NewJSONHandler(os.Stdout, options))
	}
}
