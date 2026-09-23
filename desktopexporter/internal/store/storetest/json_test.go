package storetest_test

import (
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRejectDuplicateJSONKeys(t *testing.T) {
	require.NoError(t, storetest.RejectDuplicateJSONKeys([]byte(
		`{"same":"key","nested":{"same":"different object"},"array":[{"same":true}]}`,
	)))

	for name, document := range map[string]string{
		"root object":         `{"duplicate":1,"duplicate":2}`,
		"nested object":       `{"nested":{"duplicate":1,"duplicate":2}}`,
		"object inside array": `[{"duplicate":1,"duplicate":2}]`,
	} {
		t.Run(name, func(t *testing.T) {
			assert.EqualError(t, storetest.RejectDuplicateJSONKeys([]byte(document)), `duplicate JSON key "duplicate"`)
		})
	}
}
