package spans

import (
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaticTraceFieldsResolveToScalarOperandModes(t *testing.T) {
	fields := map[string]search.OperandMode{
		"resource.droppedAttributesCount": search.NativeSignedIntegerOperand,
		"scope.name":                      search.TextOperand,
		"scope.version":                   search.TextOperand,
		"scope.droppedAttributesCount":    search.NativeSignedIntegerOperand,
		"traceID":                         search.WireIDOperand,
		"traceState":                      search.TextOperand,
		"spanID":                          search.WireIDOperand,
		"parentSpanID":                    search.WireIDOperand,
		"name":                            search.TextOperand,
		"kind":                            search.TextOperand,
		"startTime":                       search.NativeSignedIntegerOperand,
		"endTime":                         search.NativeSignedIntegerOperand,
		"duration":                        search.DurationOperand,
		"droppedAttributesCount":          search.NativeSignedIntegerOperand,
		"droppedEventsCount":              search.NativeSignedIntegerOperand,
		"droppedLinksCount":               search.NativeSignedIntegerOperand,
		"statusCode":                      search.TextOperand,
		"statusMessage":                   search.TextOperand,
		"event.name":                      search.TextOperand,
		"event.timestamp":                 search.NativeSignedIntegerOperand,
		"event.droppedAttributesCount":    search.NativeSignedIntegerOperand,
		"flags":                           search.NativeSignedIntegerOperand,
		"link.flags":                      search.NativeSignedIntegerOperand,
		"link.traceID":                    search.WireIDOperand,
		"link.spanID":                     search.WireIDOperand,
		"link.traceState":                 search.TextOperand,
		"link.droppedAttributesCount":     search.NativeSignedIntegerOperand,
	}

	for name, wantMode := range fields {
		t.Run(name, func(t *testing.T) {
			resolved, err := mapTraceFieldExpression(&search.FieldDefinition{Name: name, SearchScope: "field"})
			require.NoError(t, err)
			assert.NotEmpty(t, resolved.SQL)
			assert.Equal(t, wantMode, resolved.OperandMode)
		})
	}
}
