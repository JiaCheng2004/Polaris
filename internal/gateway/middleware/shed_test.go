package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JiaCheng2004/Polaris/internal/reliability"
	"github.com/gin-gonic/gin"
)

func TestShedMiddlewareRejectsOverCap(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mgr := reliability.NewManager(reliability.Config{Shed: reliability.ShedConfig{Enabled: true, GlobalMaxInflight: 1}}, nil)

	// Hold the single global slot.
	release, ok := mgr.AdmitGlobal()
	if !ok {
		t.Fatal("failed to acquire the only slot")
	}

	r := gin.New()
	r.Use(Shed(mgr))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	// While the slot is held, the request is shed with 503 + Retry-After.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("over-cap status = %d, want 503", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("missing Retry-After header on shed response")
	}

	// After releasing, requests pass.
	release()
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/x", nil))
	if w2.Code != http.StatusOK {
		t.Fatalf("after release status = %d, want 200", w2.Code)
	}
}

func TestShedMiddlewareDisabledByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mgr := reliability.NewManager(reliability.Config{}, nil) // shed disabled
	r := gin.New()
	r.Use(Shed(mgr))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 20; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("request %d shed while disabled: %d", i, w.Code)
		}
	}
}

func TestShedMiddlewareNilManager(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Shed(nil))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("nil manager status = %d, want 200", w.Code)
	}
}
