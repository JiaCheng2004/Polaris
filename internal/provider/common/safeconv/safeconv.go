package safeconv

import (
	"fmt"
	"math"
)

func Uint32FromInt(label string, value int) (uint32, error) {
	if value < 0 || uint64(value) > math.MaxUint32 {
		return 0, fmt.Errorf("%s %d exceeds uint32 range", label, value)
	}
	return uint32(value), nil
}

func IntFromUint32(label string, value uint32) (int, error) {
	if uint64(value) > uint64(math.MaxInt) {
		return 0, fmt.Errorf("%s %d exceeds int range", label, value)
	}
	return int(value), nil
}

func Int32FromInt(label string, value int) (int32, error) {
	if value < math.MinInt32 || value > math.MaxInt32 {
		return 0, fmt.Errorf("%s %d exceeds int32 range", label, value)
	}
	return int32(value), nil
}

func Int32FromUint64(label string, value uint64) (int32, error) {
	if value > math.MaxInt32 {
		return 0, fmt.Errorf("%s %d exceeds int32 range", label, value)
	}
	return int32(value), nil
}

func Uint32FromInt32Bits(value int32) uint32 {
	return uint32(value)
}

func Int32FromUint32Bits(value uint32) int32 {
	return int32(value)
}

func Int16FromUint16Bits(value uint16) int16 {
	return int16(value)
}

func Uint16FromInt16Bits(value int16) uint16 {
	return uint16(value)
}

func LowByteFromInt16Bits(value int16) byte {
	return byte(value)
}
