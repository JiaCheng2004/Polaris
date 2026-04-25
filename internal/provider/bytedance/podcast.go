package bytedance

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/gorilla/websocket"
)

const (
	defaultPodcastURL              = "wss://openspeech.bytedance.com/api/v3/sami/podcasttts"
	defaultPodcastResourceID       = "volc.service_type.10050"
	defaultPodcastAppKey           = "aGjiRDfUWi"
	podcastReadyTimeout            = 10 * time.Second
	podcastSuccessCode             = 20000000
	podcastProtocolVersion         = 0x1
	podcastHeaderSize              = 0x1
	podcastSerializationRaw        = 0x0
	podcastSerializationJSON       = 0x1
	podcastCompressionNone         = 0x0
	podcastCompressionGzip         = 0x1
	podcastClientMessageType       = 0x1
	podcastServerMessageType       = 0x9
	podcastAudioMessageType        = 0xB
	podcastErrorMessageType        = 0xF
	podcastFlagEventPresent        = 0x4
	podcastFlagPositiveSeq         = 0x1
	podcastFlagNegativeSeq         = 0x3
	podcastEventStartConnection    = 1
	podcastEventFinishConnection   = 2
	podcastEventConnectionStarted  = 50
	podcastEventConnectionFailed   = 51
	podcastEventConnectionFinished = 52
	podcastEventStartSession       = 100
	podcastEventFinishSession      = 102
	podcastEventSessionStarted     = 150
	podcastEventSessionFailed      = 153
	podcastEventUsageResponse      = 154
	podcastEventRoundStart         = 360
	podcastEventRoundAudio         = 361
	podcastEventRoundEnd           = 362
	podcastEventPodcastEnd         = 363
	podcastEventSessionDone        = 152
)

type podcastAdapter struct {
	client     *Client
	model      string
	url        string
	resourceID string
	appKey     string
}

type podcastFrame struct {
	MessageType   byte
	Flags         byte
	Serialization byte
	Compression   byte
	Code          *uint32
	Event         *uint32
	SessionID     string
	ConnectID     string
	Sequence      *int32
	Payload       []byte
}

type podcastRequest struct {
	InputID      string                  `json:"input_id"`
	Action       int                     `json:"action"`
	UseHeadMusic bool                    `json:"use_head_music,omitempty"`
	InputInfo    podcastInputInfo        `json:"input_info"`
	AudioConfig  podcastAudioConfig      `json:"audio_config"`
	NLPTexts     []podcastRequestSegment `json:"nlp_texts"`
}

type podcastInputInfo struct {
	ReturnAudioURL bool `json:"return_audio_url"`
}

type podcastAudioConfig struct {
	Format     string `json:"format"`
	SampleRate int    `json:"sample_rate"`
	SpeechRate int    `json:"speech_rate"`
}

type podcastRequestSegment struct {
	Speaker string `json:"speaker"`
	Text    string `json:"text"`
}

type podcastUsageEnvelope struct {
	Usage struct {
		InputTextTokens   int `json:"input_text_tokens"`
		OutputAudioTokens int `json:"output_audio_tokens"`
	} `json:"usage"`
}

type podcastEndEnvelope struct {
	MetaInfo struct {
		AudioURL string `json:"audio_url"`
	} `json:"meta_info"`
}

func NewPodcastAdapter(client *Client, model string, endpoint string) modality.PodcastAdapter {
	return &podcastAdapter{
		client:     client,
		model:      model,
		url:        strings.TrimSpace(endpoint),
		resourceID: defaultPodcastResourceID,
		appKey:     defaultPodcastAppKey,
	}
}

func (a *podcastAdapter) GeneratePodcast(ctx context.Context, req *modality.PodcastRequest) (*modality.PodcastResult, error) {
	if a == nil || a.client == nil {
		return nil, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "adapter_unavailable", "", "Podcast adapter is unavailable.")
	}
	if strings.TrimSpace(a.client.appID) == "" || strings.TrimSpace(a.client.speechToken) == "" {
		return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_misconfigured", "", "ByteDance podcast generation requires providers.bytedance.app_id and providers.bytedance.speech_access_token.")
	}

	dialer := websocket.Dialer{HandshakeTimeout: minDuration(a.client.httpClient.Timeout, 15*time.Second)}
	headers := http.Header{}
	headers.Set("X-Api-App-Id", a.client.appID)
	headers.Set("X-Api-Access-Key", a.client.speechToken)
	headers.Set("X-Api-Resource-Id", a.resourceID)
	headers.Set("X-Api-App-Key", a.appKey)
	headers.Set("X-Api-Connect-Id", newRealtimeSessionID())

	conn, _, err := dialer.DialContext(ctx, a.streamURL(), headers)
	if err != nil {
		return nil, translateTransportError(err, "ByteDance")
	}
	defer func() {
		_ = conn.Close()
	}()

	startConnectionFrame, err := encodePodcastConnectionFrame(podcastEventStartConnection, map[string]any{})
	if err != nil {
		return nil, httputil.NewError(http.StatusInternalServerError, "internal_error", "internal_error", "", "Failed to encode ByteDance podcast connection request.")
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, startConnectionFrame); err != nil {
		return nil, translateTransportError(err, "ByteDance")
	}

	deadline := time.Now().Add(podcastReadyTimeout)
	_ = conn.SetReadDeadline(deadline)

	var (
		audioBuf bytes.Buffer
		audioURL string
		usage    modality.Usage
		started  bool
	)
	usage.Source = modality.TokenCountSourceProviderReported

	if _, err := a.waitForPodcastEvent(conn, podcastServerMessageType, podcastEventConnectionStarted); err != nil {
		return nil, err
	}

	sessionID := newRealtimeSessionID()
	startFrame, err := encodePodcastJSONFrame(podcastClientMessageType, podcastEventStartSession, sessionID, a.buildRequest(req))
	if err != nil {
		return nil, httputil.NewError(http.StatusInternalServerError, "internal_error", "internal_error", "", "Failed to encode ByteDance podcast request.")
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, startFrame); err != nil {
		return nil, translateTransportError(err, "ByteDance")
	}
	if _, err := a.waitForPodcastEvent(conn, podcastServerMessageType, podcastEventSessionStarted); err != nil {
		return nil, err
	}

	finishSessionFrame, err := encodePodcastJSONFrame(podcastClientMessageType, podcastEventFinishSession, sessionID, map[string]any{})
	if err != nil {
		return nil, httputil.NewError(http.StatusInternalServerError, "internal_error", "internal_error", "", "Failed to encode ByteDance podcast finish request.")
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, finishSessionFrame); err != nil {
		return nil, translateTransportError(err, "ByteDance")
	}
	_ = conn.SetReadDeadline(time.Time{})

	for {
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			if !started {
				return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_error", "", "ByteDance podcast handshake failed.")
			}
			return nil, translateTransportError(err, "ByteDance")
		}
		if messageType != websocket.BinaryMessage {
			continue
		}
		frame, err := decodePodcastFrame(payload)
		if err != nil {
			return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance podcast returned an invalid frame.")
		}
		if frame.MessageType == podcastErrorMessageType {
			return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_error", "", firstNonEmpty(strings.TrimSpace(string(frame.Payload)), "ByteDance podcast generation failed."))
		}
		if frame.Event == nil {
			continue
		}
		switch *frame.Event {
		case podcastEventConnectionFailed:
			return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_error", "", firstNonEmpty(strings.TrimSpace(string(frame.Payload)), "ByteDance podcast connection failed."))
		case podcastEventSessionFailed:
			return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_error", "", firstNonEmpty(strings.TrimSpace(string(frame.Payload)), "ByteDance podcast generation failed."))
		case podcastEventRoundStart:
			started = true
		case podcastEventUsageResponse:
			var env podcastUsageEnvelope
			if err := json.Unmarshal(frame.Payload, &env); err == nil {
				usage.PromptTokens += env.Usage.InputTextTokens
				usage.CompletionTokens += env.Usage.OutputAudioTokens
				usage.TotalTokens += env.Usage.InputTextTokens + env.Usage.OutputAudioTokens
			}
		case podcastEventRoundAudio:
			audioBuf.Write(frame.Payload)
		case podcastEventPodcastEnd:
			var env podcastEndEnvelope
			if err := json.Unmarshal(frame.Payload, &env); err == nil {
				audioURL = strings.TrimSpace(env.MetaInfo.AudioURL)
			}
		case podcastEventConnectionFinished:
			if len(audioBuf.Bytes()) == 0 && audioURL == "" {
				continue
			}
		case podcastEventSessionDone:
			audio := audioBuf.Bytes()
			contentType := podcastContentType(req.OutputFormat)
			if len(audio) == 0 && audioURL != "" {
				downloaded, downloadedType, err := a.fetchPodcastAudio(ctx, audioURL)
				if err != nil {
					return nil, err
				}
				audio = downloaded
				if strings.TrimSpace(downloadedType) != "" {
					contentType = downloadedType
				}
			}
			if len(audio) == 0 {
				return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance podcast generation returned no audio.")
			}
			return &modality.PodcastResult{
				Audio:       append([]byte(nil), audio...),
				ContentType: contentType,
				Usage:       usage,
				Metadata: map[string]any{
					"audio_url": audioURL,
				},
			}, nil
		}
	}
}

func (a *podcastAdapter) waitForPodcastEvent(conn *websocket.Conn, messageType byte, eventID uint32) (*podcastFrame, error) {
	for {
		rawType, payload, err := conn.ReadMessage()
		if err != nil {
			return nil, translateTransportError(err, "ByteDance")
		}
		if rawType != websocket.BinaryMessage {
			continue
		}
		frame, err := decodePodcastFrame(payload)
		if err != nil {
			return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance podcast returned an invalid frame.")
		}
		if frame.MessageType == podcastErrorMessageType {
			return nil, httputil.NewError(http.StatusBadGateway, "provider_error", "provider_error", "", firstNonEmpty(strings.TrimSpace(string(frame.Payload)), "ByteDance podcast generation failed."))
		}
		if frame.Event == nil {
			continue
		}
		if frame.MessageType == messageType && *frame.Event == eventID {
			return &frame, nil
		}
	}
}

func (a *podcastAdapter) buildRequest(req *modality.PodcastRequest) podcastRequest {
	segments := make([]podcastRequestSegment, 0, len(req.Segments))
	for _, segment := range req.Segments {
		speaker := strings.TrimSpace(segment.Voice)
		if speaker == "" {
			speaker = strings.TrimSpace(segment.Speaker)
		}
		segments = append(segments, podcastRequestSegment{
			Speaker: speaker,
			Text:    strings.TrimSpace(segment.Text),
		})
	}
	useHeadMusic := false
	if req.UseHeadMusic != nil {
		useHeadMusic = *req.UseHeadMusic
	}
	return podcastRequest{
		InputID:      "polaris_podcast_" + newRequestID(),
		Action:       3,
		UseHeadMusic: useHeadMusic,
		InputInfo: podcastInputInfo{
			ReturnAudioURL: true,
		},
		AudioConfig: podcastAudioConfig{
			Format:     podcastFormat(req.OutputFormat),
			SampleRate: podcastSampleRate(req.SampleRateHz),
			SpeechRate: 0,
		},
		NLPTexts: segments,
	}
}

func (a *podcastAdapter) fetchPodcastAudio(ctx context.Context, rawURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("build bytedance podcast asset request: %w", err)
	}
	resp, err := a.client.httpClient.Do(req)
	if err != nil {
		return nil, "", translateTransportError(err, "ByteDance")
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, "", httputil.ProviderAPIError("ByteDance", resp.StatusCode, httputil.ProviderErrorDetails{
			Message: "ByteDance podcast audio download failed.",
		})
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance podcast audio could not be read.")
	}
	return data, resp.Header.Get("Content-Type"), nil
}

func (a *podcastAdapter) streamURL() string {
	if strings.TrimSpace(a.url) != "" {
		return a.url
	}
	return defaultPodcastURL
}

func podcastFormat(candidate string) string {
	switch strings.ToLower(strings.TrimSpace(candidate)) {
	case "", "mp3":
		return "mp3"
	case "ogg_opus", "opus":
		return "ogg_opus"
	case "pcm":
		return "pcm"
	case "aac":
		return "aac"
	default:
		return "mp3"
	}
}

func podcastSampleRate(candidate int) int {
	switch candidate {
	case 16000, 24000, 48000:
		return candidate
	default:
		return 24000
	}
}

func podcastContentType(format string) string {
	switch podcastFormat(format) {
	case "ogg_opus":
		return "audio/ogg"
	case "pcm":
		return "audio/pcm"
	case "aac":
		return "audio/aac"
	default:
		return "audio/mpeg"
	}
}

func encodePodcastJSONFrame(messageType byte, eventID uint32, sessionID string, payload any) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal podcast payload: %w", err)
	}
	return encodePodcastFrame(podcastFrame{
		MessageType:   messageType,
		Flags:         podcastFlagEventPresent,
		Serialization: podcastSerializationJSON,
		Compression:   podcastCompressionNone,
		Event:         &eventID,
		SessionID:     sessionID,
		Payload:       raw,
	})
}

func encodePodcastConnectionFrame(eventID uint32, payload any) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal podcast connection payload: %w", err)
	}
	return encodePodcastFrame(podcastFrame{
		MessageType:   podcastClientMessageType,
		Flags:         podcastFlagEventPresent,
		Serialization: podcastSerializationJSON,
		Compression:   podcastCompressionNone,
		Event:         &eventID,
		Payload:       raw,
	})
}

func encodePodcastFrame(frame podcastFrame) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte(byte(podcastProtocolVersion<<4 | podcastHeaderSize))
	buf.WriteByte(byte(frame.MessageType<<4 | (frame.Flags & 0x0F)))
	buf.WriteByte(byte((frame.Serialization << 4) | (frame.Compression & 0x0F)))
	buf.WriteByte(0)
	if frame.Code != nil {
		writePodcastUint32(&buf, *frame.Code)
	}
	if frame.Sequence != nil {
		writePodcastInt32(&buf, *frame.Sequence)
	}
	if frame.Flags&podcastFlagEventPresent != 0 {
		if frame.Event == nil {
			return nil, fmt.Errorf("podcast frame requires event id")
		}
		writePodcastUint32(&buf, *frame.Event)
	}
	if podcastFrameHasSessionID(frame.Event) {
		writePodcastUint32(&buf, uint32(len(frame.SessionID)))
		buf.WriteString(frame.SessionID)
	}
	if podcastFrameHasConnectID(frame.Event) {
		writePodcastUint32(&buf, uint32(len(frame.ConnectID)))
		buf.WriteString(frame.ConnectID)
	}
	writePodcastUint32(&buf, uint32(len(frame.Payload)))
	buf.Write(frame.Payload)
	return buf.Bytes(), nil
}

func decodePodcastFrame(payload []byte) (podcastFrame, error) {
	if len(payload) < 8 {
		return podcastFrame{}, fmt.Errorf("podcast frame is too short")
	}
	reader := bytes.NewReader(payload)
	b0, _ := reader.ReadByte()
	b1, _ := reader.ReadByte()
	b2, _ := reader.ReadByte()
	_, _ = reader.ReadByte()
	frame := podcastFrame{
		MessageType:   b1 >> 4,
		Flags:         b1 & 0x0F,
		Serialization: b2 >> 4,
		Compression:   b2 & 0x0F,
	}
	if b0>>4 != podcastProtocolVersion || b0&0x0F != podcastHeaderSize {
		return podcastFrame{}, fmt.Errorf("unsupported podcast frame header")
	}
	if frame.MessageType == podcastErrorMessageType {
		code, err := readPodcastUint32(reader)
		if err != nil {
			return podcastFrame{}, err
		}
		frame.Code = &code
	}
	if frame.MessageType != podcastErrorMessageType && (frame.Flags == podcastFlagPositiveSeq || frame.Flags == podcastFlagNegativeSeq) {
		sequence, err := readPodcastInt32(reader)
		if err != nil {
			return podcastFrame{}, err
		}
		frame.Sequence = &sequence
	}
	if frame.Flags&podcastFlagEventPresent != 0 {
		eventID, err := readPodcastUint32(reader)
		if err != nil {
			return podcastFrame{}, err
		}
		frame.Event = &eventID
	}
	if podcastFrameHasSessionID(frame.Event) {
		sessionID, err := readPodcastString(reader)
		if err != nil {
			return podcastFrame{}, err
		}
		frame.SessionID = sessionID
	}
	if podcastFrameHasConnectID(frame.Event) {
		connectID, err := readPodcastString(reader)
		if err != nil {
			return podcastFrame{}, err
		}
		frame.ConnectID = connectID
	}
	size, err := readPodcastUint32(reader)
	if err != nil {
		return podcastFrame{}, err
	}
	if size > uint32(reader.Len()) {
		return podcastFrame{}, fmt.Errorf("podcast payload size exceeds frame body")
	}
	frame.Payload = make([]byte, int(size))
	if _, err := io.ReadFull(reader, frame.Payload); err != nil {
		return podcastFrame{}, err
	}
	if frame.Compression == podcastCompressionGzip && len(frame.Payload) > 0 {
		decompressed, err := gunzipPodcast(frame.Payload)
		if err != nil {
			return podcastFrame{}, err
		}
		frame.Payload = decompressed
	}
	return frame, nil
}

func writePodcastUint32(buf *bytes.Buffer, value uint32) {
	var raw [4]byte
	binary.BigEndian.PutUint32(raw[:], value)
	buf.Write(raw[:])
}

func writePodcastInt32(buf *bytes.Buffer, value int32) {
	var raw [4]byte
	binary.BigEndian.PutUint32(raw[:], uint32(value))
	buf.Write(raw[:])
}

func readPodcastUint32(reader *bytes.Reader) (uint32, error) {
	var raw [4]byte
	if _, err := io.ReadFull(reader, raw[:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(raw[:]), nil
}

func readPodcastInt32(reader *bytes.Reader) (int32, error) {
	var raw [4]byte
	if _, err := io.ReadFull(reader, raw[:]); err != nil {
		return 0, err
	}
	return int32(binary.BigEndian.Uint32(raw[:])), nil
}

func readPodcastString(reader *bytes.Reader) (string, error) {
	size, err := readPodcastUint32(reader)
	if err != nil {
		return "", err
	}
	if size > uint32(reader.Len()) {
		return "", fmt.Errorf("podcast string length exceeds frame body")
	}
	value := make([]byte, int(size))
	if _, err := io.ReadFull(reader, value); err != nil {
		return "", err
	}
	return string(value), nil
}

func gunzipPodcast(payload []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = reader.Close()
	}()
	return io.ReadAll(reader)
}

func podcastFrameHasSessionID(eventID *uint32) bool {
	if eventID == nil {
		return false
	}
	switch *eventID {
	case podcastEventStartConnection, podcastEventFinishConnection, podcastEventConnectionStarted, podcastEventConnectionFailed, podcastEventConnectionFinished:
		return false
	default:
		return true
	}
}

func podcastFrameHasConnectID(eventID *uint32) bool {
	if eventID == nil {
		return false
	}
	switch *eventID {
	case podcastEventConnectionStarted, podcastEventConnectionFailed, podcastEventConnectionFinished:
		return true
	default:
		return false
	}
}

func minDuration(a time.Duration, b time.Duration) time.Duration {
	switch {
	case a <= 0:
		return b
	case b <= 0:
		return a
	case a < b:
		return a
	default:
		return b
	}
}
