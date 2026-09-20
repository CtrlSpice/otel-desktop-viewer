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
		"kindCode":                        search.NativeSignedIntegerOperand,
		"startTime":                       search.TimestampOperand,
		"endTime":                         search.TimestampOperand,
		"duration":                        search.DurationOperand,
		"droppedAttributesCount":          search.NativeSignedIntegerOperand,
		"droppedEventsCount":              search.NativeSignedIntegerOperand,
		"droppedLinksCount":               search.NativeSignedIntegerOperand,
		"statusCode":                      search.TextOperand,
		"statusCodeValue":                 search.NativeSignedIntegerOperand,
		"statusMessage":                   search.TextOperand,
		"event.name":                      search.TextOperand,
		"event.timestamp":                 search.TimestampOperand,
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

func TestNumericTraceAttributeOwnersUseTypedDatabaseDecoders(t *testing.T) {
	for _, scope := range []string{"resource", "scope", "span", "event", "link"} {
		t.Run(scope, func(t *testing.T) {
			params := []search.NamedParam{}
			resolved, err := mapTraceAttributeExpressions(
				&search.FieldDefinition{Name: "number", SearchScope: "attribute", AttributeScope: scope, Type: "int64"},
				&search.Query{FieldOperator: ">", Value: "2"},
				&params,
			)
			require.NoError(t, err)
			require.Len(t, resolved, 1)
			assert.Equal(t, search.AttributeSignedIntegerOperand, resolved[0].OperandMode)
			assert.Contains(t, resolved[0].SQL, "attribute_int64(a.value)")
			assert.Contains(t, resolved[0].SQL, "json_extract_string(a.value, '$.kind') = attr_kind_1")
		})
	}
}
