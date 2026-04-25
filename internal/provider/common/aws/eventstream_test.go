package aws

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"testing"
)

func TestReadEventStreamPayload(t *testing.T) {
	t.Parallel()

	want := []byte(`{"messageStart":{"role":"assistant"}}`)
	frame := encodeEventStreamFrame(t, want)

	got, err := ReadEventStreamPayload(bytes.NewReader(frame))
	if err != nil {
		t.Fatalf("ReadEventStreamPayload() error = %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("payload = %q, want %q", got, want)
	}
}

func encodeEventStreamFrame(t *testing.T, payload []byte) []byte {
	t.Helper()

	headersLength := 0
	totalLength := 16 + headersLength + len(payload)
	frame := make([]byte, totalLength)

	binary.BigEndian.PutUint32(frame[0:4], uint32(totalLength))
	binary.BigEndian.PutUint32(frame[4:8], uint32(headersLength))
	binary.BigEndian.PutUint32(frame[8:12], crc32.ChecksumIEEE(frame[0:8]))
	copy(frame[12:12+len(payload)], payload)
	binary.BigEndian.PutUint32(frame[len(frame)-4:], crc32.ChecksumIEEE(frame[:len(frame)-4]))
	return frame
}
