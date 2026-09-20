package metrics

import (
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaticMetricFieldsResolveOrAreDeliberatelyExcluded(t *testing.T) {
	fields := map[string]search.OperandMode{
		"resource.droppedAttributesCount": search.NativeSignedIntegerOperand,
		"scope.name":                      search.TextOperand,
		"scope.version":                   search.TextOperand,
		"scope.droppedAttributesCount":    search.NativeSignedIntegerOperand,
		"name":                            search.TextOperand,
		"description":                     search.TextOperand,
		"unit":                            search.TextOperand,
		"type":                            search.TextOperand,
	}

	for name, wantMode := range fields {
		t.Run(name, func(t *testing.T) {
			resolved, err := mapMetricFieldExpression(&search.FieldDefinition{Name: name, SearchScope: "field"})
			require.NoError(t, err)
			assert.NotEmpty(t, resolved.SQL)
			assert.Equal(t, wantMode, resolved.OperandMode)
		})
	}

	resolved, err := mapMetricFieldExpression(&search.FieldDefinition{Name: "received", SearchScope: "field"})
	assert.ErrorIs(t, err, ErrInvalidMetricQuery)
	assert.Empty(t, resolved.SQL)
}
