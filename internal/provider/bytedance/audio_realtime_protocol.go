package bytedance

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

const (
	dialogProtocolVersion = 0x1
	dialogHeaderSize      = 0x1

	dialogSerializationRaw  = 0x0
	dialogSerializationJSON = 0x1

	dialogCompressionNone = 0x0
	dialogCompressionGzip = 0x1

	dialogMessageTypeFullClient  = 0x1
	dialogMessageTypeAudioClient = 0x2
	dialogMessageTypeFullServer  = 0x9
	dialogMessageTypeAudioServer = 0xB
	dialogMessageTypeError       = 0xF

	dialogFlagSequencePositive = 0x1
	dialogFlagSequenceLast     = 0x2
	dialogFlagEventPresent     = 0x4

	dialogEventStartConnection   = 1
	dialogEventFinishConnection  = 2
	dialogEventStartSession      = 100
	dialogEventFinishSession     = 102
	dialogEventTaskRequest       = 200
	dialogEventUpdateConfig      = 201
	dialogEventEndASR            = 400
	dialogEventChatTextQuery     = 501
	dialogEventClientInterrupt   = 515
	dialogEventConnectionStarted = 50
	dialogEventConnectionFailed  = 51
	dialogEventConnectionClosed  = 52
	dialogEventSessionStarted    = 150
	dialogEventSessionFinished   = 152
	dialogEventSessionFailed     = 153
	dialogEventUsageResponse     = 154
	dialogEventConfigUpdated     = 251
	dialogEventTTSSentenceStart  = 350
	dialogEventTTSSentenceEnd    = 351
	dialogEventTTSResponse       = 352
	dialogEventTTSEnded          = 359
	dialogEventASRInfo           = 450
	dialogEventASRResponse       = 451
	dialogEventASREnded          = 459
	dialogEventChatResponse      = 550
	dialogEventChatConfirmed     = 553
	dialogEventChatEnded         = 559
	dialogEventDialogError       = 599
)

type dialogFrame struct {
	MessageType   byte
	Flags         byte
	Serialization byte
	Compression   byte
	Code          *uint32
	Sequence      *int32
	Event         *uint32
	SessionID     string
	ConnectID     string
	Payload       []byte
}

func encodeDialogJSONFrame(messageType byte, eventID uint32, sessionID string, connectID string, payload any) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal realtime dialogue payload: %w", err)
	}
	return encodeDialogFrame(dialogFrame{
		MessageType:   messageType,
		Flags:         dialogFlagEventPresent,
		Serialization: dialogSerializationJSON,
		Compression:   dialogCompressionNone,
		Event:         &eventID,
		SessionID:     sessionID,
		ConnectID:     connectID,
		Payload:       raw,
	})
}

func encodeDialogAudioFrame(eventID uint32, sessionID string, audio []byte) ([]byte, error) {
	payload := append([]byte(nil), audio...)
	return encodeDialogFrame(dialogFrame{
		MessageType:   dialogMessageTypeAudioClient,
		Flags:         dialogFlagEventPresent,
		Serialization: dialogSerializationRaw,
		Compression:   dialogCompressionNone,
		Event:         &eventID,
		SessionID:     sessionID,
		Payload:       payload,
	})
}

func encodeDialogFrame(frame dialogFrame) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte(byte(dialogProtocolVersion<<4 | dialogHeaderSize))
	buf.WriteByte(byte(frame.MessageType<<4 | (frame.Flags & 0x0F)))
	buf.WriteByte(byte((frame.Serialization << 4) | (frame.Compression & 0x0F)))
	buf.WriteByte(0)

	if frame.Code != nil {
		writeUint32(&buf, *frame.Code)
	}
	if sequenceMode := frame.Flags & 0x3; sequenceMode == dialogFlagSequencePositive || sequenceMode == 0x3 {
		if frame.Sequence == nil {
			return nil, fmt.Errorf("realtime dialogue frame requires sequence for flags=%d", frame.Flags)
		}
		writeInt32(&buf, *frame.Sequence)
	}
	if frame.Flags&dialogFlagEventPresent != 0 {
		if frame.Event == nil {
			return nil, fmt.Errorf("realtime dialogue frame requires event id")
		}
		writeUint32(&buf, *frame.Event)
	}
	if frame.ConnectID != "" {
		writeUint32(&buf, uint32(len(frame.ConnectID)))
		buf.WriteString(frame.ConnectID)
	}
	if eventRequiresSessionID(frame.Event) {
		writeUint32(&buf, uint32(len(frame.SessionID)))
		buf.WriteString(frame.SessionID)
	}
	writeUint32(&buf, uint32(len(frame.Payload)))
	buf.Write(frame.Payload)
	return buf.Bytes(), nil
}

func decodeDialogFrame(payload []byte) (dialogFrame, error) {
	if len(payload) < 8 {
		return dialogFrame{}, fmt.Errorf("realtime dialogue frame is too short")
	}
	reader := bytes.NewReader(payload)
	b0, _ := reader.ReadByte()
	b1, _ := reader.ReadByte()
	b2, _ := reader.ReadByte()
	_, _ = reader.ReadByte() // reserved

	frame := dialogFrame{
		MessageType:   b1 >> 4,
		Flags:         b1 & 0x0F,
		Serialization: b2 >> 4,
		Compression:   b2 & 0x0F,
	}
	if b0>>4 != dialogProtocolVersion {
		return dialogFrame{}, fmt.Errorf("unsupported realtime dialogue protocol version %d", b0>>4)
	}
	if b0&0x0F != dialogHeaderSize {
		return dialogFrame{}, fmt.Errorf("unsupported realtime dialogue header size %d", b0&0x0F)
	}

	if frame.MessageType == dialogMessageTypeError {
		code, err := readUint32(reader)
		if err != nil {
			return dialogFrame{}, err
		}
		frame.Code = &code
	}
	if sequenceMode := frame.Flags & 0x3; sequenceMode == dialogFlagSequencePositive || sequenceMode == 0x3 {
		sequence, err := readInt32(reader)
		if err != nil {
			return dialogFrame{}, err
		}
		frame.Sequence = &sequence
	}
	if frame.Flags&dialogFlagEventPresent != 0 {
		eventID, err := readUint32(reader)
		if err != nil {
			return dialogFrame{}, err
		}
		frame.Event = &eventID
	}
	if isConnectEvent(frame.Event) {
		position, _ := reader.Seek(0, io.SeekCurrent)
		connectID, ok, err := tryReadOptionalSizedString(reader)
		if err != nil {
			return dialogFrame{}, err
		}
		if ok {
			frame.ConnectID = connectID
		} else {
			if _, err := reader.Seek(position, io.SeekStart); err != nil {
				return dialogFrame{}, fmt.Errorf("reset connect id probe: %w", err)
			}
		}
	}
	if eventRequiresSessionID(frame.Event) {
		sessionID, err := readSizedString(reader)
		if err != nil {
			return dialogFrame{}, err
		}
		frame.SessionID = sessionID
	}

	size, err := readUint32(reader)
	if err != nil {
		return dialogFrame{}, err
	}
	if size > uint32(reader.Len()) {
		return dialogFrame{}, fmt.Errorf("realtime dialogue payload size %d exceeds remaining bytes %d", size, reader.Len())
	}
	frame.Payload = make([]byte, int(size))
	if _, err := io.ReadFull(reader, frame.Payload); err != nil {
		return dialogFrame{}, fmt.Errorf("read realtime dialogue payload: %w", err)
	}
	if frame.Compression == dialogCompressionGzip && len(frame.Payload) > 0 {
		decompressed, err := gunzipBytes(frame.Payload)
		if err != nil {
			return dialogFrame{}, err
		}
		frame.Payload = decompressed
	}
	return frame, nil
}

func eventRequiresSessionID(event *uint32) bool {
	if event == nil {
		return false
	}
	switch *event {
	case dialogEventStartConnection, dialogEventFinishConnection, dialogEventConnectionStarted, dialogEventConnectionFailed, dialogEventConnectionClosed:
		return false
	default:
		return true
	}
}

func isConnectEvent(event *uint32) bool {
	if event == nil {
		return false
	}
	switch *event {
	case dialogEventStartConnection, dialogEventFinishConnection, dialogEventConnectionStarted, dialogEventConnectionFailed, dialogEventConnectionClosed:
		return true
	default:
		return false
	}
}

func writeUint32(buf *bytes.Buffer, value uint32) {
	var raw [4]byte
	binary.BigEndian.PutUint32(raw[:], value)
	buf.Write(raw[:])
}

func writeInt32(buf *bytes.Buffer, value int32) {
	writeUint32(buf, uint32(value))
}

func readUint32(reader *bytes.Reader) (uint32, error) {
	var raw [4]byte
	if _, err := io.ReadFull(reader, raw[:]); err != nil {
		return 0, fmt.Errorf("read uint32: %w", err)
	}
	return binary.BigEndian.Uint32(raw[:]), nil
}

func readInt32(reader *bytes.Reader) (int32, error) {
	value, err := readUint32(reader)
	if err != nil {
		return 0, err
	}
	return int32(value), nil
}

func readSizedString(reader *bytes.Reader) (string, error) {
	size, err := readUint32(reader)
	if err != nil {
		return "", err
	}
	if size > uint32(reader.Len()) {
		return "", fmt.Errorf("realtime dialogue string size %d exceeds remaining bytes %d", size, reader.Len())
	}
	buf := make([]byte, int(size))
	if _, err := io.ReadFull(reader, buf); err != nil {
		return "", fmt.Errorf("read sized string: %w", err)
	}
	return string(buf), nil
}

func tryReadOptionalSizedString(reader *bytes.Reader) (string, bool, error) {
	size, err := readUint32(reader)
	if err != nil {
		return "", false, err
	}
	if int(size) > reader.Len()-4 {
		return "", false, nil
	}
	buf := make([]byte, int(size))
	if _, err := io.ReadFull(reader, buf); err != nil {
		return "", false, fmt.Errorf("read optional sized string: %w", err)
	}
	return string(buf), true, nil
}

func gunzipBytes(payload []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("open realtime dialogue gzip payload: %w", err)
	}
	defer func() {
		_ = reader.Close()
	}()
	raw, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read realtime dialogue gzip payload: %w", err)
	}
	return raw, nil
}
