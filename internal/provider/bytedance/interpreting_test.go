package bytedance

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	eventpb "github.com/JiaCheng2004/Polaris/internal/provider/bytedance/astpb/common/event"
	rpcmetapb "github.com/JiaCheng2004/Polaris/internal/provider/bytedance/astpb/common/rpcmeta"
	astpb "github.com/JiaCheng2004/Polaris/internal/provider/bytedance/astpb/products/understanding/ast"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

func TestInterpretingAdapterSpeechToText(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Api-App-Key"); got != "app-123" {
			t.Fatalf("unexpected X-Api-App-Key %q", got)
		}
		if got := r.Header.Get("X-Api-Access-Key"); got != "speech-token" {
			t.Fatalf("unexpected X-Api-Access-Key %q", got)
		}
		if got := r.Header.Get("X-Api-Resource-Id"); got != defaultInterpretingResourceID {
			t.Fatalf("unexpected X-Api-Resource-Id %q", got)
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Upgrade() error = %v", err)
		}
		defer func() {
			_ = conn.Close()
		}()

		start := expectInterpretingFrame(t, conn)
		if start.GetEvent() != eventpb.Type_StartSession {
			t.Fatalf("unexpected start event %#v", start)
		}
		if start.GetRequest().GetMode() != "s2t" || start.GetRequest().GetSourceLanguage() != "zh" || start.GetRequest().GetTargetLanguage() != "en" {
			t.Fatalf("unexpected start request %#v", start.GetRequest())
		}
		if start.GetSourceAudio().GetFormat() != "wav" {
			t.Fatalf("unexpected source audio format %#v", start.GetSourceAudio())
		}
		writeInterpretingResponse(t, conn, &astpb.TranslateResponse{
			ResponseMeta: &rpcmetapb.ResponseMeta{
				SessionID:  start.GetRequestMeta().GetSessionID(),
				StatusCode: interpretingSuccessCode,
			},
			Event: eventpb.Type_SessionStarted,
		})

		audio := expectInterpretingFrame(t, conn)
		if audio.GetEvent() != eventpb.Type_TaskRequest {
			t.Fatalf("unexpected audio event %#v", audio)
		}
		if string(audio.GetSourceAudio().GetBinaryData()) != string([]byte("wavdata")) {
			t.Fatalf("unexpected audio payload %q", string(audio.GetSourceAudio().GetBinaryData()))
		}

		finish := expectInterpretingFrame(t, conn)
		if finish.GetEvent() != eventpb.Type_FinishSession {
			t.Fatalf("unexpected finish event %#v", finish)
		}

		writeInterpretingResponse(t, conn, &astpb.TranslateResponse{
			ResponseMeta: &rpcmetapb.ResponseMeta{
				SessionID:  start.GetRequestMeta().GetSessionID(),
				StatusCode: interpretingSuccessCode,
			},
			Event:     eventpb.Type_TranslationSubtitleResponse,
			Text:      "Hello from Polaris",
			StartTime: 0,
			EndTime:   800,
		})
		writeInterpretingResponse(t, conn, &astpb.TranslateResponse{
			ResponseMeta: &rpcmetapb.ResponseMeta{
				SessionID:  start.GetRequestMeta().GetSessionID(),
				StatusCode: interpretingSuccessCode,
			},
			Event:     eventpb.Type_TranslationSubtitleEnd,
			Text:      "Hello from Polaris",
			StartTime: 0,
			EndTime:   800,
		})
		writeInterpretingResponse(t, conn, &astpb.TranslateResponse{
			ResponseMeta: &rpcmetapb.ResponseMeta{
				SessionID:  start.GetRequestMeta().GetSessionID(),
				StatusCode: interpretingSuccessCode,
				Billing: &rpcmetapb.Billing{
					DurationMsec: 800,
					Items: []*rpcmetapb.BillingItem{
						{Unit: "input_audio_tokens", Quantity: 1},
						{Unit: "output_text_tokens", Quantity: 3},
					},
				},
			},
			Event: eventpb.Type_UsageResponse,
		})
		writeInterpretingResponse(t, conn, &astpb.TranslateResponse{
			ResponseMeta: &rpcmetapb.ResponseMeta{
				SessionID:  start.GetRequestMeta().GetSessionID(),
				StatusCode: interpretingSuccessCode,
			},
			Event: eventpb.Type_SessionFinished,
		})
	}))
	defer server.Close()

	client := NewClient(config.ProviderConfig{
		AppID:             "app-123",
		SpeechAccessToken: "speech-token",
		Timeout:           time.Second,
	})
	adapter := NewInterpretingAdapter(client, "bytedance/doubao-interpreting-2.0", "ws"+strings.TrimPrefix(server.URL, "http"))
	session, err := adapter.ConnectInterpreting(context.Background(), &modality.InterpretingSessionConfig{
		Model:            "bytedance/doubao-interpreting-2.0",
		Mode:             modality.InterpretingModeSpeechToText,
		SourceLanguage:   "zh",
		TargetLanguage:   "en",
		InputAudioFormat: modality.InterpretingAudioFormatWAV,
	})
	if err != nil {
		t.Fatalf("ConnectInterpreting() error = %v", err)
	}
	defer func() {
		_ = session.Close()
	}()

	if err := session.Send(modality.InterpretingClientEvent{
		Type:  modality.InterpretingClientEventInputAudioAppend,
		Audio: base64.StdEncoding.EncodeToString([]byte("wavdata")),
	}); err != nil {
		t.Fatalf("Send(input_audio.append) error = %v", err)
	}
	if err := session.Send(modality.InterpretingClientEvent{Type: modality.InterpretingClientEventInputAudioCommit}); err != nil {
		t.Fatalf("Send(input_audio.commit) error = %v", err)
	}

	events := collectInterpretingEventsUntilCompleted(t, session.Events())
	if events[modality.InterpretingServerEventTranslationDelta].Text != "Hello from Polaris" {
		t.Fatalf("unexpected translation delta %#v", events[modality.InterpretingServerEventTranslationDelta])
	}
	segment := events[modality.InterpretingServerEventTranslationSegment]
	if segment.Segment == nil || segment.Segment.Text != "Hello from Polaris" {
		t.Fatalf("unexpected translation segment %#v", segment)
	}
	completed := events[modality.InterpretingServerEventResponseCompleted]
	if completed.Usage == nil || completed.Usage.TotalTokens != 4 || completed.Usage.Source != modality.TokenCountSourceProviderReported {
		t.Fatalf("unexpected completed usage %#v", completed)
	}
}

func expectInterpretingFrame(t *testing.T, conn *websocket.Conn) *astpb.TranslateRequest {
	t.Helper()
	_, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	request := &astpb.TranslateRequest{}
	if err := proto.Unmarshal(payload, request); err != nil {
		t.Fatalf("proto.Unmarshal() error = %v", err)
	}
	return request
}

func writeInterpretingResponse(t *testing.T, conn *websocket.Conn, response *astpb.TranslateResponse) {
	t.Helper()
	payload, err := proto.Marshal(response)
	if err != nil {
		t.Fatalf("proto.Marshal() error = %v", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
		t.Fatalf("WriteMessage() error = %v", err)
	}
}

func collectInterpretingEventsUntilCompleted(t *testing.T, events <-chan modality.InterpretingServerEvent) map[string]modality.InterpretingServerEvent {
	t.Helper()
	collected := map[string]modality.InterpretingServerEvent{}
	deadline := time.After(2 * time.Second)
	for {
		select {
		case event := <-events:
			collected[event.Type] = event
			if event.Type == modality.InterpretingServerEventResponseCompleted {
				return collected
			}
		case <-deadline:
			t.Fatalf("timed out waiting for interpreting completion")
		}
	}
}
