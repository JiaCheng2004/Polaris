package aws

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
)

const maxEventStreamMessageSize = 4 << 20

func DecodeEventStream(r io.Reader, fn func(payload []byte) error) error {
	for {
		payload, err := ReadEventStreamPayload(r)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if len(payload) == 0 {
			continue
		}
		if err := fn(payload); err != nil {
			return err
		}
	}
}

func ReadEventStreamPayload(r io.Reader) ([]byte, error) {
	prelude := make([]byte, 12)
	if _, err := io.ReadFull(r, prelude); err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil, io.EOF
		}
		return nil, err
	}

	totalLength := int(binary.BigEndian.Uint32(prelude[0:4]))
	headersLength := int(binary.BigEndian.Uint32(prelude[4:8]))
	preludeCRC := binary.BigEndian.Uint32(prelude[8:12])

	if crc32.ChecksumIEEE(prelude[:8]) != preludeCRC {
		return nil, fmt.Errorf("invalid AWS event stream prelude CRC")
	}
	if totalLength < 16 || totalLength > maxEventStreamMessageSize {
		return nil, fmt.Errorf("invalid AWS event stream message size %d", totalLength)
	}
	if headersLength < 0 || headersLength > totalLength-16 {
		return nil, fmt.Errorf("invalid AWS event stream headers size %d", headersLength)
	}

	remaining := make([]byte, totalLength-12)
	if _, err := io.ReadFull(r, remaining); err != nil {
		return nil, err
	}

	messageCRC := binary.BigEndian.Uint32(remaining[len(remaining)-4:])
	fullMessage := append(append([]byte{}, prelude...), remaining[:len(remaining)-4]...)
	if crc32.ChecksumIEEE(fullMessage) != messageCRC {
		return nil, fmt.Errorf("invalid AWS event stream message CRC")
	}

	payloadStart := headersLength
	payloadEnd := len(remaining) - 4
	if payloadStart > payloadEnd {
		return nil, fmt.Errorf("invalid AWS event stream payload bounds")
	}
	return append([]byte{}, remaining[payloadStart:payloadEnd]...), nil
}
