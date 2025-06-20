package util

import (
	"encoding/hex"
)

// ToTraceID converts a hex string to a 16-byte trace ID
func ToTraceID(hexStr string) [16]byte {
	bytes, _ := hex.DecodeString(hexStr)
	var result [16]byte
	copy(result[:], bytes)
	return result
}

// ToSpanID converts a hex string to an 8-byte span ID
func ToSpanID(hexStr string) [8]byte {
	bytes, _ := hex.DecodeString(hexStr)
	var result [8]byte
	copy(result[:], bytes)
	return result
}
