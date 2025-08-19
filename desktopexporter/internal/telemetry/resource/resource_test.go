package resource

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResourceValidation(t *testing.T) {
	tests := []struct {
		name     string
		resource *ResourceData
		validate func(t *testing.T, resource *ResourceData)
	}{
		{
			name: "validates currency service resource",
			resource: &ResourceData{
				DroppedAttributesCount: 0,
				Attributes: map[string]any{
					"service.name":           "sample.currencyservice",
					"telemetry.sdk.language": "cpp",
					"telemetry.sdk.name":     "opentelemetry",
					"telemetry.sdk.version":  "1.5.0",
					"array.example":          []any{"example1", "example2", "example3"},
				},
			},
			validate: func(t *testing.T, resource *ResourceData) {
				assert.Equal(t, uint32(0), resource.DroppedAttributesCount)

				expectedAttrs := map[string]any{
					"service.name":           "sample.currencyservice",
					"telemetry.sdk.language": "cpp",
					"telemetry.sdk.name":     "opentelemetry",
					"telemetry.sdk.version":  "1.5.0",
					"array.example":          []any{"example1", "example2", "example3"},
				}

				for key, expected := range expectedAttrs {
					assert.Equal(t, expected, resource.Attributes[key], "resource attribute %s", key)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.validate(t, tt.resource)
		})
	}
}
