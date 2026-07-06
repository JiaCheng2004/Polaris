package handler

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// TestSSESurvivesWriteTimeout proves R7: an SSE stream that runs longer than the
// server WriteTimeout is not truncated, because each frame resets the write
// deadline via http.ResponseController.
func TestSSESurvivesWriteTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const frames = 12
	engine := gin.New()
	engine.GET("/stream", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.Writer.Flush()
		for i := 0; i < frames; i++ {
			if err := writeRawSSEData(c, []byte(fmt.Sprintf(`{"i":%d}`, i))); err != nil {
				return
			}
			time.Sleep(120 * time.Millisecond) // total ~1.4s > the 1s WriteTimeout
		}
		_ = writeSSEDone(c)
	})

	srv := httptest.NewUnstartedServer(engine)
	srv.Config.WriteTimeout = 1 * time.Second
	srv.Start()
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/stream")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	got := strings.Count(string(body), `data: {"i":`)
	if got < frames {
		t.Fatalf("stream truncated: %d/%d frames received (R7 per-frame write-deadline reset failed)", got, frames)
	}
	if !strings.Contains(string(body), "[DONE]") {
		t.Fatal("stream did not reach [DONE] — truncated past WriteTimeout")
	}
}
