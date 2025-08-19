package util

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringifyOnOverflow(t *testing.T) {
	tests := []struct {
		name             string
		attributeName    string
		value            uint64
		expectedValue    any
		expectedOverflow bool
	}{
		{
			name:             "no overflow",
			attributeName:    "test_attr",
			value:            100,
			expectedValue:    int64(100),
			expectedOverflow: false,
		},
		{
			name:             "with overflow",
			attributeName:    "test_attr",
			value:            math.MaxUint64,
			expectedValue:    "18446744073709551615",
			expectedOverflow: true,
		},
		{
			name:             "at boundary (no overflow)",
			attributeName:    "test_attr",
			value:            math.MaxInt64,
			expectedValue:    int64(math.MaxInt64),
			expectedOverflow: false,
		},
		{
			name:             "just over boundary",
			attributeName:    "test_attr",
			value:            math.MaxInt64 + 1,
			expectedValue:    "9223372036854775808",
			expectedOverflow: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, hasOverflow := StringifyOnOverflow(tt.attributeName, tt.value)
			assert.Equal(t, tt.expectedOverflow, hasOverflow, "overflow detection mismatch")
			assert.Equal(t, tt.expectedValue, result, "returned value mismatch")
		})
	}
}

func TestStringifySliceOnOverflow(t *testing.T) {
	tests := []struct {
		name             string
		attributeName    string
		values           []uint64
		expectedValue    any
		expectedOverflow bool
	}{
		{
			name:             "slice no overflow",
			attributeName:    "test_attr",
			values:           []uint64{100, 200, 300},
			expectedValue:    []int64{100, 200, 300},
			expectedOverflow: false,
		},
		{
			name:             "slice with overflow",
			attributeName:    "test_attr",
			values:           []uint64{100, math.MaxUint64, 300},
			expectedValue:    []string{"100", "18446744073709551615", "300"},
			expectedOverflow: true,
		},
		{
			name:             "slice all overflow values",
			attributeName:    "test_attr",
			values:           []uint64{math.MaxUint64, math.MaxInt64 + 1},
			expectedValue:    []string{"18446744073709551615", "9223372036854775808"},
			expectedOverflow: true,
		},
		{
			name:             "empty slice",
			attributeName:    "test_attr",
			values:           []uint64{},
			expectedValue:    []int64{},
			expectedOverflow: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, hasOverflow := StringifySliceOnOverflow(tt.attributeName, tt.values)
			assert.Equal(t, tt.expectedOverflow, hasOverflow, "overflow detection mismatch")
			assert.Equal(t, tt.expectedValue, result, "returned value mismatch")
		})
	}
}
