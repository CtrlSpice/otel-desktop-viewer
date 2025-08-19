package util

import (
	"fmt"
	"log"
	"math"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/errors"
)

// StringifyOnOverflow checks a single uint64 value for overflow beyond math.MaxInt64.
// If the value exceeds math.MaxInt64, returns the string representation and true.
// Otherwise, returns the int64 value and false.
func StringifyOnOverflow(attributeName string, value uint64) (any, bool) {
	if value > math.MaxInt64 {
		strVal := fmt.Sprintf("%v", value)
		log.Printf(errors.WarnUint64Overflow, attributeName, value)
		return strVal, true
	}
	return int64(value), false
}

// StringifySliceOnOverflow checks a slice of uint64 values for overflow beyond math.MaxInt64.
// If any value exceeds math.MaxInt64, converts all values to strings and returns true.
// Otherwise, converts all values to int64 and returns false.
func StringifySliceOnOverflow(attributeName string, values []uint64) (any, bool) {
	hasOverflow := false
	for _, val := range values {
		if val > math.MaxInt64 {
			hasOverflow = true
			break
		}
	}

	// No overflow - convert all values to int64
	if !hasOverflow {
		int64List := make([]int64, len(values))
		for i, val := range values {
			int64List[i] = int64(val)
		}
		return int64List, false
	}

	// Has overflow - convert all values to string
	strList := make([]string, len(values))
	for i, v := range values {
		strList[i] = fmt.Sprintf("%v", v)
	}
	log.Printf(errors.WarnUint64SliceOverflow, attributeName)
	return strList, true
}
