package handler

import (
	"net/http"

	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/JiaCheng2004/Polaris/internal/store/cache"
	"github.com/gin-gonic/gin"
)

type HealthHandler struct {
	store   store.Store
	cache   cache.Cache
	runtime *gwruntime.Holder
}

func NewHealthHandler(store store.Store, cache cache.Cache, runtime *gwruntime.Holder) *HealthHandler {
	return &HealthHandler{
		store:   store,
		cache:   cache,
		runtime: runtime,
	}
}

func (h *HealthHandler) Liveness(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *HealthHandler) Readiness(c *gin.Context) {
	statusCode := http.StatusOK
	status := gin.H{
		"status":    "ready",
		"store":     "ok",
		"cache":     "ok",
		"providers": 0,
	}

	if h.store == nil {
		statusCode = http.StatusServiceUnavailable
		status["status"] = "not_ready"
		status["store"] = "store not configured"
	} else if err := h.store.Ping(c.Request.Context()); err != nil {
		statusCode = http.StatusServiceUnavailable
		status["status"] = "not_ready"
		status["store"] = err.Error()
	}

	if h.cache == nil {
		statusCode = http.StatusServiceUnavailable
		status["status"] = "not_ready"
		status["cache"] = "cache not configured"
	} else if err := h.cache.Ping(c.Request.Context()); err != nil {
		statusCode = http.StatusServiceUnavailable
		status["status"] = "not_ready"
		status["cache"] = err.Error()
	}

	count := 0
	if snapshot := middleware.RuntimeSnapshot(c, h.runtime); snapshot != nil && snapshot.Registry != nil {
		count = snapshot.Registry.ProviderCount()
	}
	status["providers"] = count
	if count == 0 {
		statusCode = http.StatusServiceUnavailable
		status["status"] = "not_ready"
	}

	c.JSON(statusCode, status)
}
