package bytedance

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"

	"github.com/JiaCheng2004/Polaris/internal/provider/common/safeconv"
)

const (
	streamingASRProtocolVersion = 0x1
	streamingASRHeaderSize      = 0x1

	streamingASRSerializationNone = 0x0
	streamingASRSerializationJSON = 0x1

	streamingASRCompressionNone = 0x0
	streamingASRCompressionGzip = 0x1

	streamingASRMessageTypeFullClient  = 0x1
	streamingASRMessageTypeAudioClient = 0x2
	streamingASRMessageTypeFullServer  = 0x9
	streamingASRMessageTypeError       = 0xF

	streamingASRFlagNoSequence       = 0x0
	streamingASRFlagSequencePositive = 0x1
	streamingASRFlagLastPacket       = 0x2
	streamingASRFlagSequenceLast     = 0x3
)

type streamingASRFrame struct {
	MessageType   byte
	Flags         byte
	Serialization byte
	Compression   byte
	Sequence      *int32
	ErrorCode     *uint32
	Payload       []byte
}

func encodeStreamingASRRequest(payload any) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal streaming transcription request: %w", err)
	}
	compressed, err := gzipBytes(raw)
	if err != nil {
		return nil, err
	}
	return encodeStreamingASRFrame(streamingASRFrame{
		MessageType:   streamingASRMessageTypeFullClient,
		Flags:         streamingASRFlagNoSequence,
		Serialization: streamingASRSerializationJSON,
		Compression:   streamingASRCompressionGzip,
		Payload:       compressed,
	})
}

func encodeStreamingASRAudio(audio []byte, final bool) ([]byte, error) {
	flags := byte(streamingASRFlagNoSequence)
	if final {
		flags = streamingASRFlagLastPacket
	}
	return encodeStreamingASRFrame(streamingASRFrame{
		MessageType:   streamingASRMessageTypeAudioClient,
		Flags:         flags,
		Serialization: streamingASRSerializationNone,
		Compression:   streamingASRCompressionNone,
		Payload:       append([]byte(nil), audio...),
	})
}

func encodeStreamingASRFrame(frame streamingASRFrame) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte(byte(streamingASRProtocolVersion<<4 | streamingASRHeaderSize))
	buf.WriteByte(byte(frame.MessageType<<4 | (frame.Flags & 0x0F)))
	buf.WriteByte(byte((frame.Serialization << 4) | (frame.Compression & 0x0F)))
	buf.WriteByte(0)

	sequenceMode := frame.Flags & 0x3
	if sequenceMode == streamingASRFlagSequencePositive || sequenceMode == streamingASRFlagSequenceLast {
		if frame.Sequence == nil {
			return nil, fmt.Errorf("streaming transcription frame requires sequence for flags=%d", frame.Flags)
		}
		writeInt32(&buf, *frame.Sequence)
	}
	payloadSize, err := safeconv.Uint32FromInt("streaming transcription payload length", len(frame.Payload))
	if err != nil {
		return nil, err
	}
	writeUint32(&buf, payloadSize)
	buf.Write(frame.Payload)
	return buf.Bytes(), nil
}

func decodeStreamingASRFrame(payload []byte) (streamingASRFrame, error) {
	if len(payload) < 8 {
		return streamingASRFrame{}, fmt.Errorf("streaming transcription frame is too short")
	}
	reader := bytes.NewReader(payload)
	b0, _ := reader.ReadByte()
	b1, _ := reader.ReadByte()
	b2, _ := reader.ReadByte()
	_, _ = reader.ReadByte()

	frame := streamingASRFrame{
		MessageType:   b1 >> 4,
		Flags:         b1 & 0x0F,
		Serialization: b2 >> 4,
		Compression:   b2 & 0x0F,
	}
	if b0>>4 != streamingASRProtocolVersion {
		return streamingASRFrame{}, fmt.Errorf("unsupported streaming transcription protocol version %d", b0>>4)
	}
	if b0&0x0F != streamingASRHeaderSize {
		return streamingASRFrame{}, fmt.Errorf("unsupported streaming transcription header size %d", b0&0x0F)
	}

	if frame.MessageType == streamingASRMessageTypeError {
		code, err := readUint32(reader)
		if err != nil {
			return streamingASRFrame{}, err
		}
		frame.ErrorCode = &code
		size, err := readUint32(reader)
		if err != nil {
			return streamingASRFrame{}, err
		}
		payloadSize, err := safeconv.IntFromUint32("streaming transcription error payload size", size)
		if err != nil {
			return streamingASRFrame{}, err
		}
		if payloadSize > reader.Len() {
			return streamingASRFrame{}, fmt.Errorf("streaming transcription error payload size %d exceeds remaining bytes %d", size, reader.Len())
		}
		frame.Payload = make([]byte, payloadSize)
		if _, err := io.ReadFull(reader, frame.Payload); err != nil {
			return streamingASRFrame{}, fmt.Errorf("read streaming transcription error payload: %w", err)
		}
		return frame, nil
	}

	sequenceMode := frame.Flags & 0x3
	if sequenceMode == streamingASRFlagSequencePositive || sequenceMode == streamingASRFlagSequenceLast {
		sequence, err := readInt32(reader)
		if err != nil {
			return streamingASRFrame{}, err
		}
		frame.Sequence = &sequence
	}
	size, err := readUint32(reader)
	if err != nil {
		return streamingASRFrame{}, err
	}
	payloadSize, err := safeconv.IntFromUint32("streaming transcription payload size", size)
	if err != nil {
		return streamingASRFrame{}, err
	}
	if payloadSize > reader.Len() {
		return streamingASRFrame{}, fmt.Errorf("streaming transcription payload size %d exceeds remaining bytes %d", size, reader.Len())
	}
	frame.Payload = make([]byte, payloadSize)
	if _, err := io.ReadFull(reader, frame.Payload); err != nil {
		return streamingASRFrame{}, fmt.Errorf("read streaming transcription payload: %w", err)
	}
	if frame.Compression == streamingASRCompressionGzip && len(frame.Payload) > 0 {
		decompressed, err := gunzipBytes(frame.Payload)
		if err != nil {
			return streamingASRFrame{}, err
		}
		frame.Payload = decompressed
	}
	return frame, nil
}

func gzipBytes(payload []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err := writer.Write(payload); err != nil {
		_ = writer.Close()
		return nil, fmt.Errorf("write streaming transcription gzip payload: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close streaming transcription gzip payload: %w", err)
	}
	return buf.Bytes(), nil
}
