package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCamelToSnake(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"single capital after lowercase", "traceID", "trace_id"},
		{"consecutive capitals", "traceIDField", "trace_idfield"},
		{"scope name", "scopeName", "scope_name"},
		{"PascalCase", "ScopeVersion", "scope_version"},
		{"all lowercase", "name", "name"},
		{"digit before capital", "value2Type", "value2_type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CamelToSnake(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSpanIDConversionAndWireRendering(t *testing.T) {
	for _, tc := range []struct {
		name string
		id   [8]byte
		want uint64
		wire string
	}{
		{"leading zeros", [8]byte{7: 1}, 1, "0000000000000001"},
		{"high bit", [8]byte{0x80}, uint64(1) << 63, "8000000000000000"},
		{"max uint64", [8]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, ^uint64(0), "ffffffffffffffff"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := SpanIDUint64(tc.id)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wire, SpanIDWire(got))
		})
	}
}
