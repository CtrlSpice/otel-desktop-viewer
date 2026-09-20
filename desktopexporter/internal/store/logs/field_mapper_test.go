package logs

import (
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaticLogFieldsResolveToScalarOperandModes(t *testing.T) {
	fields := map[string]search.OperandMode{
		"resource.droppedAttributesCount": search.NativeSignedIntegerOperand,
		"scope.name":                      search.TextOperand,
		"scope.version":                   search.TextOperand,
		"scope.droppedAttributesCount":    search.NativeSignedIntegerOperand,
		"timestamp":                       search.TimestampOperand,
		"observedTimestamp":               search.TimestampOperand,
		"traceID":                         search.WireIDOperand,
		"spanID":                          search.WireIDOperand,
		"severityText":                    search.TextOperand,
		"severityNumber":                  search.NativeSignedIntegerOperand,
		"body":                            search.TextOperand,
		"droppedAttributesCount":          search.NativeSignedIntegerOperand,
		"flags":                           search.NativeSignedIntegerOperand,
		"eventName":                       search.TextOperand,
	}

	for name, wantMode := range fields {
		t.Run(name, func(t *testing.T) {
			resolved, err := mapLogFieldExpression(&search.FieldDefinition{Name: name, SearchScope: "field"})
			require.NoError(t, err)
			assert.NotEmpty(t, resolved.SQL)
			assert.Equal(t, wantMode, resolved.OperandMode)
		})
	}
}
