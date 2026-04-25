package gateway

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/JiaCheng2004/Polaris/internal/store/cache"
	"github.com/gorilla/websocket"
)

func TestStreamingTranscriptionSessionLifecycleByteDance(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Upgrade() error = %v", err)
		}
		defer func() {
			_ = conn.Close()
		}()

		request := expectStreamingASRFrame(t, conn)
		if request.MessageType != 0x1 {
			t.Fatalf("unexpected upstream request %#v", request)
		}
		_ = expectStreamingASRFrame(t, conn)
		_ = expectStreamingASRFrame(t, conn)

		writeStreamingASRServerFrame(t, conn, streamingASRTestFrame{
			MessageType:   0x9,
			Flags:         0x3,
			Sequence:      int32Ptr(-1),
			Serialization: 0x1,
			Compression:   0x1,
			Payload: mustGzipJSON(t, streamingASRTestResponse{
				AudioInfo: streamingASRTestAudioInfo{Duration: 2100},
				Result: streamingASRTestResult{
					Text: "Polaris streaming works",
					Utterances: []streamingASRTestUtterance{{
						StartTime: 0,
						EndTime:   2100,
						Text:      "Polaris streaming works",
						Definite:  true,
					}},
				},
			}),
		})
	}))
	defer upstream.Close()

	cfg := testConfig(t)
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{{
		Name:          "test-key",
		KeyHash:       middleware.HashAPIKey("secret"),
		RateLimit:     "100/min",
		AllowedModels: []string{"bytedance/*"},
	}}
	cfg.Providers = map[string]config.ProviderConfig{
		"bytedance": {
			SpeechAPIKey:      "speech-key",
			AppID:             "app-123",
			SpeechAccessToken: "speech-token",
			BaseURL:           "https://ark.cn-beijing.volces.com/api/v3",
			Timeout:           time.Second,
			Models: map[string]config.ModelConfig{
				"doubao-streaming-asr-2.0": {
					Modality:     modality.ModalityVoice,
					Capabilities: []modality.Capability{modality.CapabilityStreaming},
					Endpoint:     "ws" + strings.TrimPrefix(upstream.URL, "http"),
				},
			},
		},
	}
	cfg.Routing.Aliases = map[string]string{
		"bytedance-streaming-asr": "bytedance/doubao-streaming-asr-2.0",
	}

	registry, warnings, err := provider.New(cfg)
	if err != nil {
		t.Fatalf("provider.New() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no registry warnings, got %v", warnings)
	}

	engine, err := NewEngine(Dependencies{
		Config:   cfg,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:    testSQLiteStore(t),
		Cache:    cache.NewMemory(),
		Registry: registry,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	server := httptest.NewServer(engine)
	defer server.Close()

	createReq, err := http.NewRequest(http.MethodPost, server.URL+"/v1/audio/transcriptions/stream", strings.NewReader(`{
		"model":"bytedance-streaming-asr",
		"sample_rate_hz":16000
	}`))
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	createReq.Header.Set("Authorization", "Bearer secret")
	createReq.Header.Set("Content-Type", "application/json")
	createRes, err := server.Client().Do(createReq)
	if err != nil {
		t.Fatalf("server.Client().Do() error = %v", err)
	}
	defer func() {
		_ = createRes.Body.Close()
	}()
	if createRes.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(createRes.Body)
		t.Fatalf("expected 200, got %d body=%s", createRes.StatusCode, string(body))
	}

	var session modality.StreamingTranscriptionSessionDescriptor
	if err := json.NewDecoder(createRes.Body).Decode(&session); err != nil {
		t.Fatalf("json.Unmarshal(session) error = %v", err)
	}
	if session.ID == "" || session.ClientSecret == "" {
		t.Fatalf("unexpected session descriptor %#v", session)
	}

	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+session.ClientSecret)
	wsURL := strings.Replace(session.WebSocketURL, "http://", "ws://", 1)
	if parsed, err := url.Parse(wsURL); err == nil && parsed.Host == "example.com" {
		wsURL = strings.Replace(server.URL, "http://", "ws://", 1) + parsed.Path
	}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	var created modality.StreamingTranscriptionServerEvent
	if err := conn.ReadJSON(&created); err != nil {
		t.Fatalf("ReadJSON(session.created) error = %v", err)
	}
	if created.Type != modality.StreamingTranscriptionServerEventSessionCreated {
		t.Fatalf("unexpected created event %#v", created)
	}

	if err := conn.WriteJSON(modality.StreamingTranscriptionClientEvent{
		Type:  modality.StreamingTranscriptionClientEventInputAudioAppend,
		Audio: base64.StdEncoding.EncodeToString([]byte{0x01, 0x02, 0x03, 0x04}),
	}); err != nil {
		t.Fatalf("WriteJSON(input_audio.append) error = %v", err)
	}
	if err := conn.WriteJSON(modality.StreamingTranscriptionClientEvent{
		Type: modality.StreamingTranscriptionClientEventInputAudioCommit,
	}); err != nil {
		t.Fatalf("WriteJSON(input_audio.commit) error = %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	var completed modality.StreamingTranscriptionServerEvent
	for time.Now().Before(deadline) {
		if err := conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
			t.Fatalf("SetReadDeadline() error = %v", err)
		}
		var event modality.StreamingTranscriptionServerEvent
		if err := conn.ReadJSON(&event); err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				break
			}
			continue
		}
		if event.Type == modality.StreamingTranscriptionServerEventTranscriptCompleted {
			completed = event
			break
		}
	}
	if completed.Transcript == nil || completed.Transcript.Text != "Polaris streaming works" {
		t.Fatalf("unexpected completed event %#v", completed)
	}
}

type streamingASRTestFrame struct {
	MessageType   byte
	Flags         byte
	Serialization byte
	Compression   byte
	Sequence      *int32
	Payload       []byte
}

type streamingASRTestResponse struct {
	AudioInfo streamingASRTestAudioInfo `json:"audio_info"`
	Result    streamingASRTestResult    `json:"result"`
}

type streamingASRTestAudioInfo struct {
	Duration float64 `json:"duration"`
}

type streamingASRTestResult struct {
	Text       string                      `json:"text"`
	Utterances []streamingASRTestUtterance `json:"utterances"`
}

type streamingASRTestUtterance struct {
	StartTime int    `json:"start_time"`
	EndTime   int    `json:"end_time"`
	Text      string `json:"text"`
	Definite  bool   `json:"definite"`
}

func expectStreamingASRFrame(t *testing.T, conn *websocket.Conn) streamingASRTestFrame {
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
	frame, err := decodeStreamingASRTestFrame(payload)
	if err != nil {
		t.Fatalf("decodeStreamingASRFrame() error = %v", err)
	}
	return frame
}

func writeStreamingASRServerFrame(t *testing.T, conn *websocket.Conn, frame streamingASRTestFrame) {
	t.Helper()
	payload, err := encodeStreamingASRTestFrame(frame)
	if err != nil {
		t.Fatalf("encodeStreamingASRFrame() error = %v", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
		t.Fatalf("WriteMessage() error = %v", err)
	}
}

func encodeStreamingASRTestFrame(frame streamingASRTestFrame) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte(0x11)
	buf.WriteByte(byte(frame.MessageType<<4 | (frame.Flags & 0x0F)))
	buf.WriteByte(byte((frame.Serialization << 4) | (frame.Compression & 0x0F)))
	buf.WriteByte(0)
	switch frame.Flags & 0x3 {
	case 0x1, 0x3:
		if frame.Sequence == nil {
			return nil, fmt.Errorf("sequence is required")
		}
		writeTestInt32(&buf, *frame.Sequence)
	}
	writeTestUint32(&buf, uint32(len(frame.Payload)))
	buf.Write(frame.Payload)
	return buf.Bytes(), nil
}

func decodeStreamingASRTestFrame(payload []byte) (streamingASRTestFrame, error) {
	if len(payload) < 8 {
		return streamingASRTestFrame{}, fmt.Errorf("frame too short")
	}
	reader := bytes.NewReader(payload)
	b0, _ := reader.ReadByte()
	b1, _ := reader.ReadByte()
	b2, _ := reader.ReadByte()
	_, _ = reader.ReadByte()
	if b0 != 0x11 {
		return streamingASRTestFrame{}, fmt.Errorf("unexpected protocol byte %x", b0)
	}
	frame := streamingASRTestFrame{
		MessageType:   b1 >> 4,
		Flags:         b1 & 0x0F,
		Serialization: b2 >> 4,
		Compression:   b2 & 0x0F,
	}
	switch frame.Flags & 0x3 {
	case 0x1, 0x3:
		value, err := readTestInt32(reader)
		if err != nil {
			return streamingASRTestFrame{}, err
		}
		frame.Sequence = &value
	}
	size, err := readTestUint32(reader)
	if err != nil {
		return streamingASRTestFrame{}, err
	}
	if size > uint32(reader.Len()) {
		return streamingASRTestFrame{}, fmt.Errorf("payload size exceeds remaining bytes")
	}
	frame.Payload = make([]byte, int(size))
	if _, err := io.ReadFull(reader, frame.Payload); err != nil {
		return streamingASRTestFrame{}, err
	}
	if frame.Compression == 0x1 && len(frame.Payload) > 0 {
		decompressed, err := gunzipTestBytes(frame.Payload)
		if err != nil {
			return streamingASRTestFrame{}, err
		}
		frame.Payload = decompressed
	}
	return frame, nil
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

func writeTestUint32(buf *bytes.Buffer, value uint32) {
	var raw [4]byte
	binary.BigEndian.PutUint32(raw[:], value)
	buf.Write(raw[:])
}

func writeTestInt32(buf *bytes.Buffer, value int32) {
	writeTestUint32(buf, uint32(value))
}

func readTestUint32(reader *bytes.Reader) (uint32, error) {
	var raw [4]byte
	if _, err := io.ReadFull(reader, raw[:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(raw[:]), nil
}

func readTestInt32(reader *bytes.Reader) (int32, error) {
	value, err := readTestUint32(reader)
	if err != nil {
		return 0, err
	}
	return int32(value), nil
}

func gunzipTestBytes(payload []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = reader.Close()
	}()
	return io.ReadAll(reader)
}

func int32Ptr(value int32) *int32 {
	return &value
}
