package runtime

import (
	"errors"
	"log/slog"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/provider"
)

type Reloader struct {
	path      string
	overrides config.RuntimeOverrides
	holder    *Holder
	logger    *slog.Logger
	level     *slog.LevelVar
}

func NewReloader(path string, overrides config.RuntimeOverrides, holder *Holder, logger *slog.Logger, level *slog.LevelVar) *Reloader {
	if logger == nil {
		logger = slog.Default()
	}

	return &Reloader{
		path:      path,
		overrides: overrides,
		holder:    holder,
		logger:    logger,
		level:     level,
	}
}

func (r *Reloader) Reload() error {
	if r == nil {
		return errors.New("reloader is nil")
	}
	if r.holder == nil {
		return errors.New("runtime holder is nil")
	}

	nextCfg, configWarnings, err := config.Load(r.path)
	if err != nil {
		return err
	}
	config.ApplyRuntimeOverrides(nextCfg, r.overrides)

	current := r.holder.Current()
	if current != nil {
		if err := validateReloadable(current.Config, nextCfg); err != nil {
			return err
		}
	}

	registry, registryWarnings, err := provider.New(nextCfg)
	if err != nil {
		return err
	}

	for _, warning := range configWarnings {
		r.logger.Warn("config warning", "warning", warning)
	}
	for _, warning := range registryWarnings {
		r.logger.Warn("registry warning", "warning", warning)
	}

	if r.level != nil {
		ApplyLogLevel(r.level, nextCfg.Observability.Logging.Level)
	}

	r.holder.Swap(nextCfg, registry)
	return nil
}

func validateReloadable(current *config.Config, next *config.Config) error {
	if current == nil || next == nil {
		return nil
	}

	var problems []error

	if current.Server != next.Server {
		problems = append(problems, errors.New("server settings cannot be changed by hot reload"))
	}
	if current.Store != next.Store {
		problems = append(problems, errors.New("store settings cannot be changed by hot reload"))
	}
	if current.Cache.Driver != next.Cache.Driver || current.Cache.URL != next.Cache.URL {
		problems = append(problems, errors.New("cache connection settings cannot be changed by hot reload"))
	}
	if current.Observability.Logging.Format != next.Observability.Logging.Format {
		problems = append(problems, errors.New("logging.format cannot be changed by hot reload"))
	}
	if current.Observability.Metrics.Path != next.Observability.Metrics.Path {
		problems = append(problems, errors.New("observability.metrics.path cannot be changed by hot reload"))
	}

	return errors.Join(problems...)
}

func ApplyLogLevel(level *slog.LevelVar, raw string) {
	if level == nil {
		return
	}

	switch strings.ToLower(raw) {
	case "debug":
		level.Set(slog.LevelDebug)
	case "warn":
		level.Set(slog.LevelWarn)
	case "error":
		level.Set(slog.LevelError)
	default:
		level.Set(slog.LevelInfo)
	}
}
