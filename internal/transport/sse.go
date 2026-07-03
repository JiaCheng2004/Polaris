package transport

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

const defaultMaxSSEFrameBytes = 1 << 20

// SSEFrame is one server-sent-events frame: an optional event name and the
// concatenated data payload.
type SSEFrame struct {
	Event string
	Data  string
}

// SSEReader parses a text/event-stream body into frames. It is modality-neutral:
// callers translate each frame's Data (JSON) into their own chunk types. It
// handles CRLF, multi-line data accumulation, `event:` names, comment lines
// (leading ':'), and a trailing frame with no terminating blank line, and
// guards against pathologically large frames.
type SSEReader struct {
	r         *bufio.Reader
	maxBytes  int
	dataLines []string
	dataBytes int
	event     string
}

// NewSSEReader wraps r with the default max-frame guard (1 MiB).
func NewSSEReader(r io.Reader) *SSEReader {
	return &SSEReader{r: bufio.NewReader(r), maxBytes: defaultMaxSSEFrameBytes}
}

// Next returns the next frame, or io.EOF at end of stream.
func (s *SSEReader) Next() (SSEFrame, error) {
	for {
		line, err := s.r.ReadString('\n')
		if len(line) > 0 {
			trimmed := strings.TrimRight(line, "\r\n")
			switch {
			case trimmed == "":
				if frame, ok := s.flush(); ok {
					return frame, nil
				}
			case strings.HasPrefix(trimmed, "data:"):
				value := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
				s.dataBytes += len(value)
				if s.dataBytes > s.maxBytes {
					return SSEFrame{}, fmt.Errorf("sse frame exceeds %d bytes", s.maxBytes)
				}
				s.dataLines = append(s.dataLines, value)
			case strings.HasPrefix(trimmed, "event:"):
				s.event = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
				// comment (":...") and other fields (id:, retry:) are ignored
			}
		}
		if err != nil {
			if err == io.EOF {
				if frame, ok := s.flush(); ok {
					return frame, nil
				}
				return SSEFrame{}, io.EOF
			}
			return SSEFrame{}, err
		}
	}
}

func (s *SSEReader) flush() (SSEFrame, bool) {
	if s.event == "" && len(s.dataLines) == 0 {
		return SSEFrame{}, false
	}
	frame := SSEFrame{Event: s.event, Data: strings.Join(s.dataLines, "\n")}
	s.event = ""
	s.dataLines = nil
	s.dataBytes = 0
	return frame, true
}
