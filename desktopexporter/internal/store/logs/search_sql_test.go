package logs

import (
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/timerange"
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
			resolved, err := logFieldMapper()(
				&search.FieldDefinition{Name: "number", SearchScope: "attribute", AttributeScope: scope, Type: "float64"},
				&search.Query{FieldOperator: ">", Value: "2"},
				&params,
			)
			require.NoError(t, err)
			require.Len(t, resolved, 1)
			assert.Equal(t, search.AttributeDoubleOperand, resolved[0].OperandMode)
			predicate, err := search.BuildOperatorCondition(resolved[0], &search.Query{FieldOperator: ">", Value: "2"}, &params)
			require.NoError(t, err)
			assert.Contains(t, predicate, "attributes[$1] as double)")
			assert.Contains(t, predicate, "attributes[$1]) = $2")
		})
	}
}

func TestLogTimePredicateShapes(t *testing.T) {
	start, end := uint64(10), uint64(20)
	for _, tc := range []struct {
		name      string
		timeRange timerange.TimeRange
		wantWhere string
		wantArgs  []any
	}{
		{"unbounded", timerange.TimeRange{}, "true", []any{}},
		{"end only", timerange.TimeRange{End: &end}, "l.log_time <= $1::ubigint", []any{"20"}},
		{"start only", timerange.TimeRange{Start: &start}, "l.log_time >= $1::ubigint", []any{"10"}},
		{"bounded", timerange.TimeRange{Start: &start, End: &end}, "l.log_time >= $1::ubigint and l.log_time <= $2::ubigint", []any{"10", "20"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, where, args, err := buildLogSQL(nil, tc.timeRange)
			require.NoError(t, err)
			require.Equal(t, tc.wantWhere, where)
			require.Equal(t, tc.wantArgs, args)
		})
	}
}

func TestGetTraceLogSummariesSQLBindsTraceID(t *testing.T) {
	t.Parallel()
	hostile := `00000000-0000-0000-0000-000000000099' or true --`

	query, args, err := getTraceLogSummariesSQL(hostile)
	require.NoError(t, err)
	require.Equal(t, []any{hostile}, args)
	require.NotContains(t, query, hostile)
	require.Contains(t, query, "where l.trace_id = ?::uuid")
	require.NotContains(t, query, " limit ")
	require.NotContains(t, query, "attrs_json")
}
