package scope

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScopeValidation(t *testing.T) {
	tests := []struct {
		name     string
		scope    *ScopeData
		validate func(t *testing.T, scope *ScopeData)
	}{
		{
			name: "validates currency service scope",
			scope: &ScopeData{
				Name:                   "sample.currencyservice",
				Version:                "v1.2.3",
				DroppedAttributesCount: 2,
				Attributes: map[string]any{
					"owner.name":    "Mila Ardath",
					"owner.contact": "github.com/CtrlSpice",
				},
			},
			validate: func(t *testing.T, scope *ScopeData) {
				assert.Equal(t, "sample.currencyservice", scope.Name)
				assert.Equal(t, "v1.2.3", scope.Version)
				assert.Equal(t, uint32(2), scope.DroppedAttributesCount)

				expectedAttrs := map[string]any{
					"owner.name":    "Mila Ardath",
					"owner.contact": "github.com/CtrlSpice",
				}

				for key, expected := range expectedAttrs {
					assert.Equal(t, expected, scope.Attributes[key], "scope attribute %s", key)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.validate(t, tt.scope)
		})
	}
}
