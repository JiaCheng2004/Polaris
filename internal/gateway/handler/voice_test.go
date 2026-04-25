package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/gin-gonic/gin"
)

func TestWriteAudioResponseUsesRequestedFormat(t *testing.T) {
	t.Setenv("GIN_MODE", gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	writeAudioResponse(c, "wav", &modality.AudioResponse{Data: []byte("abc")})

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "audio/wav" {
		t.Fatalf("expected audio/wav content type, got %q", got)
	}
	if got := recorder.Body.String(); got != "abc" {
		t.Fatalf("expected body abc, got %q", got)
	}
}

func TestWriteTranscriptResponseUsesRawBodyForTextFormats(t *testing.T) {
	t.Setenv("GIN_MODE", gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	writeTranscriptResponse(c, "srt", &modality.TranscriptResponse{
		Raw: []byte("1\n00:00:00,000 --> 00:00:01,000\nHello\n"),
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/x-subrip; charset=utf-8" {
		t.Fatalf("unexpected Content-Type %q", got)
	}
	if got := recorder.Body.String(); got != "1\n00:00:00,000 --> 00:00:01,000\nHello\n" {
		t.Fatalf("unexpected body %q", got)
	}
}
