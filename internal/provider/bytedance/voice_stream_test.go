package bytedance

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/gorilla/websocket"
)

func TestStreamingTranscriptionAdapterRoundTrip(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Api-App-Key"); got != "app-123" {
			t.Fatalf("unexpected X-Api-App-Key %q", got)
		}
		if got := r.Header.Get("X-Api-Access-Key"); got != "speech-token" {
			t.Fatalf("unexpected X-Api-Access-Key %q", got)
		}
		if got := r.Header.Get("X-Api-Resource-Id"); got != "volc.seedasr.sauc.duration" {
			t.Fatalf("unexpected X-Api-Resource-Id %q", got)
		}
		if got := r.Header.Get("X-Api-Connect-Id"); got == "" {
			t.Fatalf("expected X-Api-Connect-Id header")
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Upgrade() error = %v", err)
		}
		defer func() {
			_ = conn.Close()
		}()

		requestFrame := expectStreamingASRFrame(t, conn)
		if requestFrame.MessageType != streamingASRMessageTypeFullClient {
			t.Fatalf("unexpected request message type %d", requestFrame.MessageType)
		}
		if requestFrame.Compression != streamingASRCompressionGzip {
			t.Fatalf("unexpected request compression %d", requestFrame.Compression)
		}

		var request streamingASRRequest
		if err := json.Unmarshal(requestFrame.Payload, &request); err != nil {
			t.Fatalf("unmarshal streaming request: %v", err)
		}
		if request.Audio.Format != "pcm" || request.Audio.Codec != "raw" || request.Audio.Rate != 16000 {
			t.Fatalf("unexpected request audio config %#v", request.Audio)
		}
		if request.Request.ModelName != "bigmodel" || !request.Request.ShowUtterances || request.Request.ResultType != "full" {
			t.Fatalf("unexpected request config %#v", request.Request)
		}

		audioFrame := expectStreamingASRFrame(t, conn)
		if audioFrame.MessageType != streamingASRMessageTypeAudioClient || audioFrame.Flags != streamingASRFlagNoSequence {
			t.Fatalf("unexpected audio append frame %#v", audioFrame)
		}
		if string(audioFrame.Payload) != string([]byte{0x01, 0x02, 0x03, 0x04}) {
			t.Fatalf("unexpected audio append payload %v", audioFrame.Payload)
		}

		finalFrame := expectStreamingASRFrame(t, conn)
		if finalFrame.MessageType != streamingASRMessageTypeAudioClient || finalFrame.Flags != streamingASRFlagLastPacket {
			t.Fatalf("unexpected final audio frame %#v", finalFrame)
		}
		if len(finalFrame.Payload) != 0 {
			t.Fatalf("expected empty final payload, got %v", finalFrame.Payload)
		}

		writeStreamingASRServerFrame(t, conn, streamingASRFrame{
			MessageType:   streamingASRMessageTypeFullServer,
			Flags:         streamingASRFlagSequencePositive,
			Sequence:      int32Ptr(1),
			Serialization: streamingASRSerializationJSON,
			Compression:   streamingASRCompressionGzip,
			Payload: mustGzipJSON(t, sttResponse{
				AudioInfo: sttAudioInfo{Duration: 2500},
				Result: sttResult{
					Text: "Hello",
				},
			}),
		})
		writeStreamingASRServerFrame(t, conn, streamingASRFrame{
			MessageType:   streamingASRMessageTypeFullServer,
			Flags:         streamingASRFlagSequenceLast,
			Sequence:      int32Ptr(-1),
			Serialization: streamingASRSerializationJSON,
			Compression:   streamingASRCompressionGzip,
			Payload: mustGzipJSON(t, sttResponse{
				AudioInfo: sttAudioInfo{Duration: 2500},
				Result: sttResult{
					Text: "Hello, ByteDance",
					Utterances: []sttUtterance{{
						StartTime: 0,
						EndTime:   2500,
						Text:      "Hello, ByteDance",
						Definite:  true,
					}},
				},
			}),
		})
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		AppID:             "app-123",
		SpeechAccessToken: "speech-token",
		Timeout:           time.Second,
	})
	adapter := NewStreamingTranscriptionAdapter(client, "bytedance/doubao-streaming-asr-2.0", "ws"+strings.TrimPrefix(server.URL, "http"))

	session, err := adapter.ConnectStreamingTranscription(context.Background(), &modality.StreamingTranscriptionSessionConfig{
		Model:            "bytedance/doubao-streaming-asr-2.0",
		InputAudioFormat: modality.AudioFormatPCM16,
		SampleRateHz:     16000,
		InterimResults:   boolPtr(true),
		ReturnUtterances: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("ConnectStreamingTranscription() error = %v", err)
	}
	defer func() {
		_ = session.Close()
	}()

	if err := session.Send(modality.StreamingTranscriptionClientEvent{
		Type:  modality.StreamingTranscriptionClientEventInputAudioAppend,
		Audio: base64.StdEncoding.EncodeToString([]byte{0x01, 0x02, 0x03, 0x04}),
	}); err != nil {
		t.Fatalf("Send(input_audio.append) error = %v", err)
	}
	if err := session.Send(modality.StreamingTranscriptionClientEvent{
		Type: modality.StreamingTranscriptionClientEventInputAudioCommit,
	}); err != nil {
		t.Fatalf("Send(input_audio.commit) error = %v", err)
	}

	events := collectStreamingEventsUntilCompleted(t, session.Events())
	if got := events[modality.StreamingTranscriptionServerEventInputAudioCommitted].Type; got != modality.StreamingTranscriptionServerEventInputAudioCommitted {
		t.Fatalf("missing committed event, got %#v", events)
	}
	if got := events[modality.StreamingTranscriptionServerEventTranscriptDelta].Text; got != "Hello" && got != ", ByteDance" && got != "Hello, ByteDance" {
		t.Fatalf("unexpected transcript delta %#v", events[modality.StreamingTranscriptionServerEventTranscriptDelta])
	}
	segment := events[modality.StreamingTranscriptionServerEventTranscriptSegment].Segment
	if segment == nil || segment.Text != "Hello, ByteDance" || !segment.Final {
		t.Fatalf("unexpected transcript segment %#v", events[modality.StreamingTranscriptionServerEventTranscriptSegment])
	}
	completed := events[modality.StreamingTranscriptionServerEventTranscriptCompleted]
	if completed.Transcript == nil || completed.Transcript.Text != "Hello, ByteDance" {
		t.Fatalf("unexpected transcript completed %#v", completed)
	}
	if len(completed.Transcript.Segments) != 1 || completed.Transcript.Segments[0].Text != "Hello, ByteDance" {
		t.Fatalf("unexpected completed segments %#v", completed.Transcript.Segments)
	}
}

func expectStreamingASRFrame(t *testing.T, conn *websocket.Conn) streamingASRFrame {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	messageType, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	if messageType != websocket.BinaryMessage {
		t.Fatalf("unexpected websocket message type %d", messageType)
	}
	frame, err := decodeStreamingASRFrame(payload)
	if err != nil {
		t.Fatalf("decodeStreamingASRFrame() error = %v", err)
	}
	return frame
}

func writeStreamingASRServerFrame(t *testing.T, conn *websocket.Conn, frame streamingASRFrame) {
	t.Helper()
	payload, err := encodeStreamingASRFrame(frame)
	if err != nil {
		t.Fatalf("encodeStreamingASRFrame() error = %v", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
		t.Fatalf("WriteMessage() error = %v", err)
	}
}

func mustGzipJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var payload bytes.Buffer
	writer := gzip.NewWriter(&payload)
	if _, err := writer.Write(raw); err != nil {
		t.Fatalf("gzip.Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("gzip.Close() error = %v", err)
	}
	return payload.Bytes()
}

func collectStreamingEventsUntilCompleted(t *testing.T, events <-chan modality.StreamingTranscriptionServerEvent) map[string]modality.StreamingTranscriptionServerEvent {
	t.Helper()
	deadline := time.After(2 * time.Second)
	collected := map[string]modality.StreamingTranscriptionServerEvent{}
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for streaming transcription completion; collected=%#v", collected)
		case event := <-events:
			if event.Type == "" {
				continue
			}
			collected[event.Type] = event
			if event.Type == modality.StreamingTranscriptionServerEventTranscriptCompleted {
				return collected
			}
		}
	}
}

func int32Ptr(value int32) *int32 {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}
