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
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/gateway/telemetry"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/JiaCheng2004/Polaris/internal/provider/verification"
	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/JiaCheng2004/Polaris/internal/store/cache"
	"github.com/JiaCheng2004/Polaris/internal/store/postgres"
	"github.com/JiaCheng2004/Polaris/internal/store/sqlite"
	"github.com/JiaCheng2004/Polaris/internal/tooling"
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
	migrate := flag.Bool("migrate", false, "Run configured store migrations and exit")
	verifyModels := flag.Bool("verify-models", false, "Print configured model verification summary and exit")
	verifyModelsJSON := flag.Bool("verify-models-json", false, "Print configured model verification summary as JSON and exit")
	flag.Parse()
	if *migrate && (*verifyModels || *verifyModelsJSON) {
		return fmt.Errorf("--migrate cannot be combined with model verification flags")
	}
	if (*verifyModels || *verifyModelsJSON) && strings.TrimSpace(os.Getenv("MINIMAX_BASE_URL")) == "" {
		_ = os.Setenv("MINIMAX_BASE_URL", "https://api.minimax.io")
	}

	cfg, warnings, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	config.ApplyRuntimeOverrides(cfg, config.RuntimeOverrides{
		Port:     *port,
		LogLevel: *logLevel,
	})
	if *verifyModels || *verifyModelsJSON {
		return runVerificationReport(cfg, warnings, *verifyModelsJSON)
	}
	if *migrate {
		return runMigrations(cfg, warnings)
	}

	logger, level := newLogger(cfg)
	slog.SetDefault(logger)

	for _, warning := range warnings {
		logger.Warn("config warning", "warning", warning)
	}

	appStore, err := bootstrapStore(cfg)
	if err != nil {
		return err
	}
	defer func() {
		if err := appStore.Close(); err != nil {
			logger.Warn("store close failed", "error", err)
		}
	}()

	if err := appStore.Migrate(context.Background()); err != nil {
		return err
	}

	appCache, err := bootstrapCache(cfg)
	if err != nil {
		return err
	}
	defer func() {
		if err := appCache.Close(); err != nil {
			logger.Warn("cache close failed", "error", err)
		}
	}()

	registry, registryWarnings, err := provider.New(cfg)
	if err != nil {
		return err
	}
	for _, warning := range registryWarnings {
		logger.Warn("registry warning", "warning", warning)
	}
	runtimeHolder := gwruntime.NewHolder(cfg, registry)

	requestLogger := store.NewAsyncRequestLogger(appStore, logger, store.NewLoggerConfig(cfg.Store.LogBufferSize, cfg.Store.LogFlushInterval))
	defer func() {
		if err := requestLogger.Close(context.Background()); err != nil {
			logger.Warn("request logger close failed", "error", err)
		}
	}()
	auditLogger := store.NewAsyncAuditLogger(appStore, logger, store.NewLoggerConfig(cfg.Store.LogBufferSize, cfg.Store.LogFlushInterval))
	if auditLogger != nil {
		defer func() {
			if err := auditLogger.Close(context.Background()); err != nil {
				logger.Warn("audit logger close failed", "error", err)
			}
		}()
	}
	toolRegistry := tooling.NewRegistry()
	tracing, err := telemetry.Setup(context.Background(), cfg.Observability.Traces)
	if err != nil {
		return err
	}
	defer func() {
		if err := tracing.Shutdown(context.Background()); err != nil {
			logger.Warn("tracing shutdown failed", "error", err)
		}
	}()

	reloader := gwruntime.NewReloader(*configPath, config.RuntimeOverrides{
		Port:     *port,
		LogLevel: *logLevel,
	}, runtimeHolder, logger, level)
	configWatcher, err := config.NewWatcher(*configPath, 250*time.Millisecond, func(trigger string) {
		if err := reloader.Reload(); err != nil {
			logger.Error("config reload failed", "trigger", trigger, "error", err)
			return
		}
		logger.Info("config reloaded", "trigger", trigger)
	}, func(err error) {
		logger.Error("config watcher error", "error", err)
	})
	if err != nil {
		return err
	}
	defer func() {
		if err := configWatcher.Close(); err != nil {
			logger.Warn("config watcher close failed", "error", err)
		}
	}()

	server, err := gateway.NewHTTPServer(gateway.Dependencies{
		Config:        cfg,
		Logger:        logger,
		Store:         appStore,
		Cache:         appCache,
		Registry:      registry,
		Runtime:       runtimeHolder,
		RequestLogger: requestLogger,
		AuditLogger:   auditLogger,
		ToolRegistry:  toolRegistry,
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

	watcherCtx, stopWatcher := context.WithCancel(context.Background())
	defer stopWatcher()

	reloadSignals := make(chan os.Signal, 1)
	signal.Notify(reloadSignals, syscall.SIGHUP)
	defer signal.Stop(reloadSignals)
	go configWatcher.Run(watcherCtx, reloadSignals)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stop)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()
	return server.Shutdown(ctx)
}

func runVerificationReport(cfg *config.Config, warnings []string, jsonOutput bool) error {
	registry, registryWarnings, err := provider.New(cfg)
	if err != nil {
		return err
	}

	report, err := verification.BuildReport(cfg, registry, append(warnings, filterVerificationWarnings(registryWarnings)...))
	if err != nil {
		return err
	}

	if jsonOutput {
		data, err := report.JSON()
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintln(os.Stdout, string(data)); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprint(os.Stdout, report.Text()); err != nil {
			return err
		}
	}

	if !report.Valid() {
		return fmt.Errorf("configured model verification failed")
	}
	return nil
}

func runMigrations(cfg *config.Config, warnings []string) error {
	logger, _ := newLogger(cfg)
	slog.SetDefault(logger)

	for _, warning := range warnings {
		logger.Warn("config warning", "warning", warning)
	}

	appStore, err := bootstrapStore(cfg)
	if err != nil {
		return err
	}
	defer func() {
		if err := appStore.Close(); err != nil {
			logger.Warn("store close failed", "error", err)
		}
	}()

	ctx := context.Background()
	if err := appStore.Migrate(ctx); err != nil {
		return fmt.Errorf("run store migrations: %w", err)
	}
	if err := appStore.Ping(ctx); err != nil {
		return fmt.Errorf("ping store after migration: %w", err)
	}

	logger.Info("store migrations completed", "driver", cfg.Store.Driver)
	return nil
}

func filterVerificationWarnings(warnings []string) []string {
	filtered := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		if strings.HasPrefix(warning, "alias ") && strings.Contains(warning, " points to unavailable model ") {
			continue
		}
		if strings.HasPrefix(warning, "selector ") && strings.Contains(warning, " did not match any enabled model") {
			continue
		}
		filtered = append(filtered, warning)
	}
	return filtered
}

func bootstrapStore(cfg *config.Config) (store.Store, error) {
	switch cfg.Store.Driver {
	case "sqlite":
		return sqlite.New(cfg.Store)
	case "postgres":
		return postgres.New(cfg.Store)
	default:
		return nil, fmt.Errorf("store driver %q is not implemented in this runtime kernel", cfg.Store.Driver)
	}
}

func bootstrapCache(cfg *config.Config) (cache.Cache, error) {
	switch cfg.Cache.Driver {
	case "memory":
		return cache.NewMemory(), nil
	case "redis":
		return cache.NewRedis(cfg.Cache.URL)
	default:
		return nil, fmt.Errorf("cache driver %q is not implemented in this runtime kernel", cfg.Cache.Driver)
	}
}

func newLogger(cfg *config.Config) (*slog.Logger, *slog.LevelVar) {
	level := new(slog.LevelVar)
	gwruntime.ApplyLogLevel(level, cfg.Observability.Logging.Level)

	options := &slog.HandlerOptions{Level: level}
	switch strings.ToLower(cfg.Observability.Logging.Format) {
	case "text":
		return slog.New(slog.NewTextHandler(os.Stdout, options)), level
	default:
		return slog.New(slog.NewJSONHandler(os.Stdout, options)), level
	}
}
