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

func TestNumericLogAttributeOwnersUseTypedDatabaseDecoders(t *testing.T) {
	for _, scope := range []string{"resource", "scope", "log"} {
		t.Run(scope, func(t *testing.T) {
			params := []search.NamedParam{}
			resolved, err := mapLogAttributeExpressions(
				&search.FieldDefinition{Name: "number", SearchScope: "attribute", AttributeScope: scope, Type: "float64"},
				&search.Query{FieldOperator: ">", Value: "2"},
				&params,
			)
			require.NoError(t, err)
			require.Len(t, resolved, 1)
			assert.Equal(t, search.AttributeDoubleOperand, resolved[0].OperandMode)
			assert.Contains(t, resolved[0].SQL, "attribute_double(a.value)")
			assert.Contains(t, resolved[0].SQL, "json_extract_string(a.value, '$.kind') = attr_kind_1")
		})
	}
}
