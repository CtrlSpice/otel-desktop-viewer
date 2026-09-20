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

func TestNumericMetricAttributeOwnersUseTypedDatabaseDecoders(t *testing.T) {
	for _, scope := range []string{"resource", "metric", "scope", "datapoint", "exemplar", "metadata"} {
		t.Run(scope, func(t *testing.T) {
			params := []search.NamedParam{}
			resolved, err := mapMetricAttributeExpressions(
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
