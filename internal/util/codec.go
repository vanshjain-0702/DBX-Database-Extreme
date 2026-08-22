// Package util provides shared utilities for DBX internals.
package util

import (
	"encoding/binary"
	"math"
)

// EncodeString writes a length-prefixed string into buf at offset, returning new offset.
func EncodeString(buf []byte, offset int, s string) int {
	l := len(s)
	binary.BigEndian.PutUint32(buf[offset:], uint32(l))
	offset += 4
	copy(buf[offset:], s)
	return offset + l
}

// DecodeString reads a length-prefixed string from buf at offset, returning string and new offset.
func DecodeString(buf []byte, offset int) (string, int) {
	l := int(binary.BigEndian.Uint32(buf[offset:]))
	offset += 4
	return string(buf[offset : offset+l]), offset + l
}

// EncodeInt64 encodes int64 as 8 big-endian bytes.
func EncodeInt64(v int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(v))
	return b
}

// DecodeInt64 decodes 8 big-endian bytes to int64.
func DecodeInt64(b []byte) int64 {
	return int64(binary.BigEndian.Uint64(b))
}

// EncodeFloat64 encodes float64 as 8 bytes (IEEE 754).
func EncodeFloat64(f float64) []byte {
	return EncodeInt64(int64(math.Float64bits(f)))
}

// DecodeFloat64 decodes 8 bytes to float64.
func DecodeFloat64(b []byte) float64 {
	return math.Float64frombits(uint64(DecodeInt64(b)))
}

// EncodeUint16 encodes uint16 as 2 big-endian bytes.
func EncodeUint16(v uint16) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, v)
	return b
}

// DecodeUint16 decodes 2 big-endian bytes to uint16.
func DecodeUint16(b []byte) uint16 {
	return binary.BigEndian.Uint16(b)
}
