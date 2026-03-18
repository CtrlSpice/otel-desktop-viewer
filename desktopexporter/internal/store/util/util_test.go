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
