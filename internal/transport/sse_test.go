package transport

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func collectFrames(t *testing.T, body string) []SSEFrame {
	t.Helper()
	reader := NewSSEReader(strings.NewReader(body))
	var frames []SSEFrame
	for {
		frame, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return frames
		}
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
		frames = append(frames, frame)
	}
}

func TestSSEReaderOpenAIStyle(t *testing.T) {
	body := "data: {\"a\":1}\n\ndata: {\"b\":2}\n\ndata: [DONE]\n\n"
	frames := collectFrames(t, body)
	if len(frames) != 3 {
		t.Fatalf("frames = %d, want 3", len(frames))
	}
	if frames[0].Data != `{"a":1}` || frames[1].Data != `{"b":2}` || frames[2].Data != "[DONE]" {
		t.Fatalf("frames = %#v", frames)
	}
}

func TestSSEReaderAnthropicStyleWithEvents(t *testing.T) {
	body := "event: message_start\ndata: {\"type\":\"message_start\"}\n\n" +
		"event: content_block_delta\ndata: {\"delta\":\"hi\"}\n\n"
	frames := collectFrames(t, body)
	if len(frames) != 2 {
		t.Fatalf("frames = %d, want 2", len(frames))
	}
	if frames[0].Event != "message_start" || frames[1].Event != "content_block_delta" {
		t.Fatalf("events = %q,%q", frames[0].Event, frames[1].Event)
	}
	if frames[1].Data != `{"delta":"hi"}` {
		t.Fatalf("data = %q", frames[1].Data)
	}
}

func TestSSEReaderHandlesCRLFCommentsAndTrailingFrame(t *testing.T) {
	// CRLF line endings, a comment line, and a final frame with no trailing blank line.
	body := ": keep-alive\r\ndata: {\"x\":1}\r\n\r\ndata: {\"y\":2}"
	frames := collectFrames(t, body)
	if len(frames) != 2 {
		t.Fatalf("frames = %d, want 2 (%#v)", len(frames), frames)
	}
	if frames[0].Data != `{"x":1}` || frames[1].Data != `{"y":2}` {
		t.Fatalf("frames = %#v", frames)
	}
}

func TestSSEReaderMultiLineData(t *testing.T) {
	body := "data: line1\ndata: line2\n\n"
	frames := collectFrames(t, body)
	if len(frames) != 1 || frames[0].Data != "line1\nline2" {
		t.Fatalf("frames = %#v", frames)
	}
}

func TestSSEReaderFrameSizeGuard(t *testing.T) {
	reader := NewSSEReader(strings.NewReader("data: " + strings.Repeat("x", (1<<20)+10) + "\n\n"))
	if _, err := reader.Next(); err == nil {
		t.Fatal("Next() error = nil, want frame-size error")
	}
}

func FuzzSSEReader(f *testing.F) {
	f.Add("data: {\"a\":1}\n\ndata: [DONE]\n\n")
	f.Add("event: x\ndata: y\n\n")
	f.Add(": comment\r\ndata: partial")
	f.Add("")
	f.Fuzz(func(t *testing.T, body string) {
		reader := NewSSEReader(strings.NewReader(body))
		for i := 0; i < 10000; i++ {
			_, err := reader.Next()
			if err != nil {
				return
			}
		}
	})
}
