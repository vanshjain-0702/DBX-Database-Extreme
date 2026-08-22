package persistence

import (
	"fmt"
	"hash/crc32"
)

// ComputeChecksum returns the CRC32 checksum of data.
func ComputeChecksum(data []byte) uint32 {
	return crc32.ChecksumIEEE(data)
}

// VerifyChecksum returns an error if checksum doesn't match.
func VerifyChecksum(data []byte, expected uint32) error {
	actual := ComputeChecksum(data)
	if actual != expected {
		return fmt.Errorf("checksum mismatch: expected %08x, got %08x", expected, actual)
	}
	return nil
}
