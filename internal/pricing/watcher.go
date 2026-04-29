package pricing

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"
)

func WatchFile(ctx context.Context, holder *Holder, path string, interval time.Duration, logger *slog.Logger) {
	if holder == nil || strings.TrimSpace(path) == "" || interval <= 0 {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var lastMod time.Time
	if info, err := os.Stat(path); err == nil {
		lastMod = info.ModTime()
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			info, err := os.Stat(path)
			if err != nil {
				logger.Warn("pricing file stat failed", "path", path, "error", err)
				continue
			}
			if !lastMod.IsZero() && !info.ModTime().After(lastMod) {
				continue
			}
			catalog, warnings, err := Load(path)
			if err != nil {
				logger.Error("pricing reload failed", "path", path, "error", err)
				lastMod = info.ModTime()
				continue
			}
			for _, warning := range warnings {
				logger.Warn("pricing warning", "warning", warning)
			}
			holder.Swap(catalog)
			lastMod = info.ModTime()
			logger.Info("pricing reloaded", "path", path, "sources", catalog.Sources())
		}
	}
}
