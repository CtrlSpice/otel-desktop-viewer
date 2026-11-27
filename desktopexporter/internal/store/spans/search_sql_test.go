package spans

import (
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/timerange"
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
			resolved, err := traceFieldMapper()(
				&search.FieldDefinition{Name: "number", SearchScope: "attribute", AttributeScope: scope, Type: "int64"},
				&search.Query{FieldOperator: ">", Value: "2"},
				&params,
			)
			require.NoError(t, err)
			require.Len(t, resolved, 1)
			assert.Equal(t, search.AttributeSignedIntegerOperand, resolved[0].OperandMode)
			predicate, err := search.BuildOperatorCondition(resolved[0], &search.Query{FieldOperator: ">", Value: "2"}, &params)
			require.NoError(t, err)
			assert.Contains(t, predicate, "attributes[$1] as bigint)")
			assert.Contains(t, predicate, "attributes[$1]) = $2")
		})
	}
}

func TestTraceTimePredicateShapes(t *testing.T) {
	start, end := uint64(10), uint64(20)
	for _, tc := range []struct {
		name      string
		timeRange timerange.TimeRange
		wantWhere string
		wantArgs  []any
	}{
		{"unbounded", timerange.TimeRange{}, "true", []any{}},
		{"end only", timerange.TimeRange{End: &end}, "s.start_time <= $1::ubigint", []any{"20"}},
		{"start only", timerange.TimeRange{Start: &start}, "s.start_time >= $1::ubigint", []any{"10"}},
		{"bounded", timerange.TimeRange{Start: &start, End: &end}, "s.start_time >= $1::ubigint and s.start_time <= $2::ubigint", []any{"10", "20"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, where, _, args, err := buildTraceSQL(nil, tc.timeRange)
			require.NoError(t, err)
			require.Equal(t, tc.wantWhere, where)
			require.Equal(t, tc.wantArgs, args)
		})
	}
}

// Hostile inputs for the two fields a caller controls: the attribute key and
// the value compared against it. Each would change the meaning of the query if
// it reached the SQL text instead of a bound parameter.
var injectionPayloads = []string{
	`' or 1=1 --`,
	`'; drop table spans; --`,
	`" or ""="`,
	`\' or 1=1`,
	`x') or (select count(*) from attributes) > 0 --`,
	// Template syntax in data must remain inert after rendering.
	`{{.MatchedJoin}}`,
	`{{template "x"}}`,
}

// TestGetTraceViewSQLBindsHostileInput verifies that caller-controlled text
// becomes a bound argument rather than SQL text.
//
// text/template does not escape SQL. Safety depends on keeping values in bound
// arguments, and template syntax in a value must not trigger another expansion.
func TestGetTraceViewSQLBindsHostileInput(t *testing.T) {
	t.Parallel()
	for _, payload := range injectionPayloads {
		t.Run(payload, func(t *testing.T) {
			// A non-equality operator so the query keeps the value comparison
			// path. Under "=" the id probe hashes the value away entirely, so
			// it never reaches the SQL either -- safe, but it would prove
			// nothing about the path where the value is actually compared.
			criteria := map[string]any{
				"id":   "n1",
				"type": "condition",
				"query": map[string]any{
					"field": map[string]any{
						"name":           payload,
						"searchScope":    "attribute",
						"attributeScope": "span",
						"type":           "string",
					},
					"fieldOperator": "CONTAINS",
					"value":         payload,
				},
			}

			query, args, err := getTraceViewSQL("00000000000000000000000000000099", criteria)
			require.NoError(t, err)

			require.NotContains(t, query, payload,
				"caller input reached the SQL text; it must be a bound argument")

			var found bool
			for _, a := range args {
				if s, ok := a.(string); ok && strings.Contains(s, payload) {
					found = true
				}
			}
			require.True(t, found, "payload should appear among the bound args")

			// The rendered SQL must carry no unexpanded template syntax,
			// whatever the input contained.
			require.NotContains(t, query, "{{", "unrendered template directive in SQL")
			require.NotContains(t, query, "<no value>", "template rendered a missing field")
		})
	}
}
