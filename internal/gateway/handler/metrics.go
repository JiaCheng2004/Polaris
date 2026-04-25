package handler

import (
	"net/http"

	"github.com/JiaCheng2004/Polaris/internal/gateway/metrics"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/gin-gonic/gin"
)

type MetricsHandler struct {
	runtime  *gwruntime.Holder
	recorder *metrics.Recorder
}

func NewMetricsHandler(runtime *gwruntime.Holder, recorder *metrics.Recorder) *MetricsHandler {
	return &MetricsHandler{
		runtime:  runtime,
		recorder: recorder,
	}
}

func (h *MetricsHandler) Serve(c *gin.Context) {
	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	if snapshot == nil || snapshot.Config == nil || !snapshot.Config.Observability.Metrics.Enabled {
		c.Status(http.StatusNotFound)
		return
	}
	gin.WrapH(h.recorder.Handler())(c)
}
