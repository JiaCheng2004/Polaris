package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JiaCheng2004/Polaris/internal/gateway/drain"
	"github.com/gin-gonic/gin"
)

func TestReadinessReportsDrainingDuringShutdown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	drainer := drain.NewRegistry()
	// nil store/cache/runtime are never reached: the draining check short-circuits.
	h := NewHealthHandler(nil, nil, nil, drainer, nil)

	drainer.SetDraining()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/ready", nil)
	h.Readiness(c)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("draining /ready = %d, want 503", w.Code)
	}
	if !strings.Contains(w.Body.String(), "draining") {
		t.Fatalf("draining /ready body = %s, want draining", w.Body.String())
	}
}
