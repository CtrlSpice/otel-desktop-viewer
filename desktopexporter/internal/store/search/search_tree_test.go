package search

import (
	"database/sql"
	"math"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseQueryTree(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected *QueryNode
		wantErr  bool
	}{
		{
			name: "simple condition",
			input: map[string]any{
				"id":   "query-1",
				"type": "condition",
				"query": map[string]any{
					"field": map[string]any{
						"name":           "service.name",
						"searchScope":    "attribute",
						"attributeScope": "resource",
					},
					"fieldOperator": "CONTAINS",
					"value":         "sample",
				},
			},
			expected: &QueryNode{
				ID:   "query-1",
				Type: "condition",
				Query: &Query{
					Field: &FieldDefinition{
						Name:           "service.name",
						SearchScope:    "attribute",
						AttributeScope: "resource",
					},
					FieldOperator: "CONTAINS",
					Value:         "sample",
				},
			},
		},
		{
			name: "group with AND operator",
			input: map[string]any{
				"id":   "query-2",
				"type": "group",
				"group": map[string]any{
					"logicalOperator": "AND",
					"children": []any{
						map[string]any{
							"id":   "query-3",
							"type": "condition",
							"query": map[string]any{
								"field": map[string]any{
									"name":           "service.name",
									"searchScope":    "attribute",
									"attributeScope": "resource",
								},
								"fieldOperator": "=",
								"value":         "frontend",
							},
						},
					},
				},
			},
			expected: &QueryNode{
				ID:   "query-2",
				Type: "group",
				Group: &QueryGroup{
					LogicalOperator: "AND",
					Children: []QueryNode{
						{
							ID:   "query-3",
							Type: "condition",
							Query: &Query{
								Field: &FieldDefinition{
									Name:           "service.name",
									SearchScope:    "attribute",
									AttributeScope: "resource",
								},
								FieldOperator: "=",
								Value:         "frontend",
							},
						},
					},
				},
			},
		},
		{
			name:    "invalid JSON",
			input:   "invalid json",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseQueryTree(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildOperatorCondition(t *testing.T) {
	tests := []struct {
		name           string
		expression     string
		fieldName      string
		operator       string
		value          string
		expectedSQL    string
		expectedParams []NamedParam
		wantErr        bool
	}{
		{
			name:           "equality operator",
			expression:     "Name",
			operator:       "=",
			value:          "test-span",
			expectedSQL:    "Name = $3",
			expectedParams: []NamedParam{{"value_2", "test-span"}},
		},
		{
			name:           "contains operator",
			expression:     "Name",
			operator:       "CONTAINS",
			value:          "test",
			expectedSQL:    "Name LIKE $3",
			expectedParams: []NamedParam{{"value_2", "%test%"}},
		},
		{
			name:           "starts with operator",
			expression:     "Name",
			operator:       "^",
			value:          "test",
			expectedSQL:    "Name LIKE $3",
			expectedParams: []NamedParam{{"value_2", "test%"}},
		},
		{
			name:           "ends with operator",
			expression:     "Name",
			operator:       "$",
			value:          "span",
			expectedSQL:    "Name LIKE $3",
			expectedParams: []NamedParam{{"value_2", "%span"}},
		},
		{
			name:           "not contains operator",
			expression:     "Name",
			operator:       "NOT CONTAINS",
			value:          "test",
			expectedSQL:    "Name NOT LIKE $3",
			expectedParams: []NamedParam{{"value_2", "%test%"}},
		},
		{
			name:           "IN operator",
			expression:     "Name",
			operator:       "IN",
			value:          `["test1","test2","test3"]`,
			expectedSQL:    "Name IN $3",
			expectedParams: []NamedParam{{"value_2", []any{"test1", "test2", "test3"}}},
		},
		{
			// Null checks use dedicated operators rather than a sentinel value.
			name:           "IS NULL operator",
			expression:     "Name",
			operator:       "IS NULL",
			value:          "",
			expectedSQL:    "Name IS NULL",
			expectedParams: nil,
		},
		{
			name:           "IS NOT NULL operator",
			expression:     "Name",
			operator:       "IS NOT NULL",
			value:          "",
			expectedSQL:    "Name IS NOT NULL",
			expectedParams: nil,
		},
		{
			// The literal string NULL remains an ordinary equality value.
			name:           "the literal string NULL is searchable",
			expression:     "Name",
			operator:       "=",
			value:          "NULL",
			expectedSQL:    "Name = $3",
			expectedParams: []NamedParam{{"value_2", "NULL"}},
		},
		{
			// DuckDB uses ~ and !~ rather than infix REGEXP.
			name:           "REGEXP maps to the ~ operator",
			expression:     "Name",
			operator:       "REGEXP",
			value:          "foo.*",
			expectedSQL:    "Name ~ $3",
			expectedParams: []NamedParam{{"value_2", "foo.*"}},
		},
		{
			name:           "NOT REGEXP maps to the !~ operator",
			expression:     "Name",
			operator:       "NOT REGEXP",
			value:          "foo.*",
			expectedSQL:    "Name !~ $3",
			expectedParams: []NamedParam{{"value_2", "foo.*"}},
		},
		{
			// A JSON array element may contain a comma.
			name:           "JSON array value with an embedded comma",
			expression:     "Name",
			operator:       "IN",
			value:          `["a,b","c"]`,
			expectedSQL:    "Name IN $3",
			expectedParams: []NamedParam{{"value_2", []any{"a,b", "c"}}},
		},
		{
			name:       "IN rejects pseudo-array",
			expression: "Name",
			operator:   "IN",
			value:      "[test1,test2]",
			wantErr:    true,
		},
		{
			name:       "IN rejects null elements",
			expression: "Name",
			operator:   "IN",
			value:      `["value",null]`,
			wantErr:    true,
		},
		{
			name:       "unsupported operator",
			expression: "Name",
			operator:   "UNSUPPORTED",
			value:      "test",
			wantErr:    true,
		},
		{
			name:           "placeholder expression equality",
			expression:     "s.SearchText {COND}",
			operator:       "=",
			value:          "test",
			expectedSQL:    "s.SearchText = $3",
			expectedParams: []NamedParam{{"value_2", "test"}},
		},
		{
			name:           "placeholder expression CONTAINS",
			expression:     "s.SearchText {COND}",
			operator:       "CONTAINS",
			value:          "test",
			expectedSQL:    "s.SearchText LIKE $3",
			expectedParams: []NamedParam{{"value_2", "%test%"}},
		},
		{
			name:           "placeholder expression IS NULL",
			expression:     "s.SearchText {COND}",
			operator:       "IS NULL",
			value:          "",
			expectedSQL:    "s.SearchText IS NULL",
			expectedParams: nil,
		},
		{
			name:        "wire ID field strips dashes and lowercases",
			expression:  "replace(l.trace_id::varchar, '-', '')",
			fieldName:   "traceID",
			operator:    "=",
			value:       "ABCD-0000-0000-0000-000000000001",
			expectedSQL: "replace(l.trace_id::varchar, '-', '') = $3",
			expectedParams: []NamedParam{
				{"value_2", "abcd000000000000000000000001"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := []NamedParam{
				{"time_start", int64(1000)},
				{"time_end", int64(2000)},
			}

			query := &Query{
				Field:         &FieldDefinition{Type: "", Name: tt.fieldName},
				FieldOperator: tt.operator,
				Value:         tt.value,
			}

			resolved := Text(tt.expression)
			if tt.fieldName == "traceID" {
				resolved = WireID(tt.expression)
			}
			sql, err := BuildOperatorCondition(resolved, query, &params)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expectedSQL, sql)

			expected := []NamedParam{
				{"time_start", int64(1000)},
				{"time_end", int64(2000)},
			}
			expected = append(expected, tt.expectedParams...)
			assert.Equal(t, expected, params)
		})
	}
}

func TestAttributeNumericOperands(t *testing.T) {
	t.Run("int64 scalar remains exact above 2^53", func(t *testing.T) {
		params := []NamedParam{}
		condition, err := BuildOperatorCondition(
			AttributeExpression("a.value::bigint", "int64", AttributeSignedIntegerOperand),
			&Query{FieldOperator: ">", Value: "9007199254740993"},
			&params,
		)
		require.NoError(t, err)
		assert.Equal(t, "a.value::bigint > $1", condition)
		assert.Equal(t, []NamedParam{{"value_0", int64(9_007_199_254_740_993)}}, params)
	})

	t.Run("int64 rejects a fractional operand", func(t *testing.T) {
		_, err := BuildOperatorCondition(
			AttributeExpression("a.value::bigint", "int64", AttributeSignedIntegerOperand),
			&Query{FieldOperator: "=", Value: "42.5"},
			&[]NamedParam{},
		)
		assert.ErrorIs(t, err, ErrInvalidQuery)
	})

	t.Run("double membership binds typed values and wire bits", func(t *testing.T) {
		params := []NamedParam{}
		condition, err := BuildOperatorCondition(
			AttributeExpression("a.value::double", "double", AttributeDoubleOperand),
			&Query{FieldOperator: "IN", Value: `["1.5","0x8000000000000000"]`},
			&params,
		)
		require.NoError(t, err)
		assert.Equal(t, "a.value::double IN CAST($1 AS DOUBLE[])", condition)
		require.Len(t, params, 1)
		values := params[0].Value.([]any)
		assert.Equal(t, 1.5, values[0])
		assert.True(t, math.Signbit(values[1].(float64)))
	})

	t.Run("double rejects noncanonical special spelling", func(t *testing.T) {
		_, err := BuildOperatorCondition(
			AttributeExpression("a.value::double", "double", AttributeDoubleOperand),
			&Query{FieldOperator: "=", Value: "NaN"},
			&[]NamedParam{},
		)
		assert.ErrorIs(t, err, ErrInvalidQuery)
	})
}

func TestParseArrayValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []any
		wantErr  bool
	}{
		{
			name:     "simple array",
			input:    `["value1","value2","value3"]`,
			expected: []any{"value1", "value2", "value3"},
		},
		{
			name:     "array with spaces",
			input:    `[ "value1" , "value2" , "value3" ]`,
			expected: []any{"value1", "value2", "value3"},
		},
		{
			name:     "empty array",
			input:    "[]",
			expected: []any{},
		},
		{
			name:     "single value",
			input:    `["single"]`,
			expected: []any{"single"},
		},
		{
			name:     "array preserves empty values",
			input:    `["value1","","value3"]`,
			expected: []any{"value1", "", "value3"},
		},
		{
			name:     "array preserves a single empty value",
			input:    `[""]`,
			expected: []any{""},
		},
		{
			name:     "array preserves the quoted string NULL",
			input:    `["NULL"]`,
			expected: []any{"NULL"},
		},
		{
			name:     "array preserves embedded commas",
			input:    `["value1,value2","value3"]`,
			expected: []any{"value1,value2", "value3"},
		},
		{
			name:    "rejects pseudo-array",
			input:   "[value1,value2]",
			wantErr: true,
		},
		{
			name:    "rejects non-array JSON",
			input:   `"value1"`,
			wantErr: true,
		},
		{
			name:    "rejects null",
			input:   "null",
			wantErr: true,
		},
		{
			name:    "rejects a null element",
			input:   "[null]",
			wantErr: true,
		},
		{
			name:    "rejects a mixed null element",
			input:   `["value",null]`,
			wantErr: true,
		},
		{
			name:    "rejects non-string elements",
			input:   `[1,2]`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseArrayValue(tt.input)
			if tt.wantErr {
				assert.ErrorIs(t, err, ErrInvalidQuery)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeNativeIntegerList(t *testing.T) {
	normalized, err := NormalizeNativeIntegerList([]any{"42", "42.0", "4.2e1", "+9007199254740993", "-9223372036854775808", "9223372036854775807", "42e000000", "0e999999999999999999999"})
	require.NoError(t, err)
	assert.Equal(t, []any{"42", "42", "42", "9007199254740993", "-9223372036854775808", "9223372036854775807", "42", "0"}, normalized)

	for _, value := range []string{"42.5", "1e-1", "nope", "9223372036854775808", "-9223372036854775809", "1e999999", "1e-999999"} {
		t.Run(value, func(t *testing.T) {
			_, err := NormalizeNativeIntegerList([]any{value})
			assert.ErrorIs(t, err, ErrInvalidQuery)
		})
	}
}

func TestBuildOperatorCondition_NativeIntegerList(t *testing.T) {
	params := []NamedParam{}
	query := &Query{Field: &FieldDefinition{Type: "int64[]"}, FieldOperator: "IN", Value: `["9007199254740993","4.2e1"]`}
	sql, err := BuildOperatorCondition(NativeInteger("l.timestamp"), query, &params)
	require.NoError(t, err)
	assert.Equal(t, "l.timestamp IN CAST($1 AS BIGINT[])", sql)
	assert.Equal(t, []NamedParam{{"value_0", []any{int64(9007199254740993), int64(42)}}}, params)

	query.Value = `["42.5"]`
	_, err = BuildOperatorCondition(NativeInteger("l.timestamp"), query, &params)
	assert.ErrorIs(t, err, ErrInvalidQuery)
}

func TestBuildOperatorCondition_NativeIntegerScalar(t *testing.T) {
	for input, expected := range map[string]int64{
		"-9223372036854775808": math.MinInt64,
		"9223372036854775807":  math.MaxInt64,
		"9007199254740993":     9_007_199_254_740_993,
		"+8":                   8,
		"8.0":                  8,
		"8e0":                  8,
	} {
		t.Run(input, func(t *testing.T) {
			params := []NamedParam{}
			condition, err := BuildOperatorCondition(
				NativeInteger("severity_number"),
				&Query{FieldOperator: "=", Value: input},
				&params,
			)
			require.NoError(t, err)
			assert.Equal(t, "severity_number = $1", condition)
			assert.Equal(t, []NamedParam{{"value_0", expected}}, params)
		})
	}

	for _, input := range []string{"8.5", "9223372036854775808", "-9223372036854775809"} {
		t.Run(input, func(t *testing.T) {
			_, err := BuildOperatorCondition(
				NativeInteger("severity_number"),
				&Query{FieldOperator: "=", Value: input},
				new([]NamedParam),
			)
			assert.ErrorIs(t, err, ErrInvalidQuery)
		})
	}

	db, err := sql.Open("duckdb", "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	params := []NamedParam{}
	condition, err := BuildOperatorCondition(
		NativeInteger("severity_number"),
		&Query{FieldOperator: "=", Value: "8.0"},
		&params,
	)
	require.NoError(t, err)
	query := "select severity_number from (values (8::integer), (9::integer)) records(severity_number) where " + condition
	var matched int
	require.NoError(t, db.QueryRow(query, params[0].Value).Scan(&matched))
	assert.Equal(t, 8, matched)
}

func TestBuildOperatorCondition_TimestampScalar(t *testing.T) {
	for input, expected := range map[string]uint64{
		"0":                    0,
		"9007199254740993":     9_007_199_254_740_993,
		"18446744073709551615": math.MaxUint64,
		"8.0":                  8,
	} {
		t.Run(input, func(t *testing.T) {
			params := []NamedParam{}
			condition, err := BuildOperatorCondition(
				Timestamp("timestamp"),
				&Query{FieldOperator: "=", Value: input},
				&params,
			)
			require.NoError(t, err)
			assert.Equal(t, "timestamp = $1::ubigint", condition)
			require.Len(t, params, 1)
			assert.Equal(t, unsignedScalar(expected), params[0].Value)
		})
	}

	for _, input := range []string{"-1", "8.5", "18446744073709551616"} {
		t.Run(input, func(t *testing.T) {
			_, err := BuildOperatorCondition(
				Timestamp("timestamp"),
				&Query{FieldOperator: "=", Value: input},
				new([]NamedParam),
			)
			assert.ErrorIs(t, err, ErrInvalidQuery)
		})
	}
}

func TestNormalizeDuration(t *testing.T) {
	for input, expected := range map[string]string{
		"1.5h":                   "5400000000000",
		"0.5ns":                  "1",
		"0.499999999999999999ns": "0",
		"0s":                     "0",
		"9223372036854775807ns":  "9223372036854775807",
	} {
		t.Run(input, func(t *testing.T) {
			actual, err := NormalizeDuration(input)
			require.NoError(t, err)
			assert.Equal(t, expected, actual)
		})
	}

	for _, input := range []string{"-1ms", "invalid", "9223372036854775808ns"} {
		t.Run(input, func(t *testing.T) {
			_, err := NormalizeDuration(input)
			assert.ErrorIs(t, err, ErrInvalidQuery)
		})
	}
}

func TestBuildOperatorCondition_Duration(t *testing.T) {
	params := []NamedParam{}
	query := &Query{FieldOperator: ">=", Value: "1.5h"}
	sql, err := BuildOperatorCondition(Duration("s.duration"), query, &params)
	require.NoError(t, err)
	assert.Equal(t, "s.duration >= $1", sql)
	assert.Equal(t, []NamedParam{{"value_0", int64(5_400_000_000_000)}}, params)

	params = nil
	query = &Query{FieldOperator: "IN", Value: `["1s","0.5ns"]`}
	sql, err = BuildOperatorCondition(Duration("s.duration"), query, &params)
	require.NoError(t, err)
	assert.Equal(t, "s.duration IN CAST($1 AS BIGINT[])", sql)
	assert.Equal(t, []NamedParam{{"value_0", []any{int64(1000000000), int64(1)}}}, params)

	for _, value := range []string{"-1ms", "9223372036854775808ns", `["1s","bad"]`, "[]", "[null]"} {
		t.Run(value, func(t *testing.T) {
			operator := ">="
			if value[0] == '[' {
				operator = "IN"
			}
			_, err := BuildOperatorCondition(Duration("s.duration"), &Query{FieldOperator: operator, Value: value}, &params)
			assert.ErrorIs(t, err, ErrInvalidQuery)
		})
	}
}

func TestBuildOperatorCondition_WireIDList(t *testing.T) {
	params := []NamedParam{}
	query := &Query{FieldOperator: "IN", Value: `["ABC-123","not-an-id"]`}
	sql, err := BuildOperatorCondition(WireID("span_id_wire(s.span_id)"), query, &params)
	require.NoError(t, err)
	assert.Equal(t, "span_id_wire(s.span_id) IN $1", sql)
	assert.Equal(t, []NamedParam{{"value_0", []any{"abc123", "notanid"}}}, params)
}

func TestBuildOperatorCondition_MembershipRequiresNonemptyFlatStringList(t *testing.T) {
	modes := map[string]ResolvedExpression{
		"text":           Text("s.text"),
		"native integer": NativeInteger("s.integer"),
		"duration":       Duration("s.duration"),
		"wire ID":        WireID("s.wire_id"),
	}
	invalidValues := map[string]string{
		"scalar":          "value",
		"empty":           "[]",
		"nested":          `[["value"]]`,
		"null-containing": `["value",null]`,
	}

	for mode, resolved := range modes {
		for shape, value := range invalidValues {
			for _, operator := range []string{"IN", "NOT IN"} {
				t.Run(mode+"/"+shape+"/"+operator, func(t *testing.T) {
					_, err := BuildOperatorCondition(resolved, &Query{FieldOperator: operator, Value: value}, new([]NamedParam))
					assert.ErrorIs(t, err, ErrInvalidQuery)
				})
			}
		}
	}
}

func TestBuildOperatorCondition_MembershipBindsHostileValues(t *testing.T) {
	const hostile = `x') OR TRUE --`
	params := []NamedParam{}

	sql, err := BuildOperatorCondition(Text("s.name"), &Query{
		FieldOperator: "NOT IN",
		Value:         `["x') OR TRUE --","safe"]`,
	}, &params)

	require.NoError(t, err)
	assert.Equal(t, "s.name NOT IN $1", sql)
	assert.NotContains(t, sql, hostile)
	assert.Equal(t, []NamedParam{{"value_0", []any{hostile, "safe"}}}, params)
}

func TestBuildSearchSQL_NilQuery(t *testing.T) {
	mapper := func(field *FieldDefinition, _ *Query, _ *[]NamedParam) ([]ResolvedExpression, error) {
		return []ResolvedExpression{Text(field.Name)}, nil
	}

	start, end := uint64(1000), uint64(2000)
	timeCondition, timeParams := TimePredicate("StartTime", &start, &end)
	cte, where, args, err := BuildSearchSQL(nil, mapper, timeCondition, timeParams)
	require.NoError(t, err)
	assert.Empty(t, cte)
	assert.Equal(t, "StartTime >= $1::ubigint AND StartTime <= $2::ubigint", where)
	assert.Equal(t, []any{"1000", "2000"}, args)
}

func TestBuildSearchSQL_SimpleCondition(t *testing.T) {
	mapper := func(field *FieldDefinition, _ *Query, _ *[]NamedParam) ([]ResolvedExpression, error) {
		return []ResolvedExpression{Text(field.Name)}, nil
	}

	query := &QueryNode{
		ID:   "q1",
		Type: "condition",
		Query: &Query{
			Field:         &FieldDefinition{Name: "Name", SearchScope: "field"},
			FieldOperator: "=",
			Value:         "test-span",
		},
	}

	start, end := uint64(1000), uint64(2000)
	timeCondition, timeParams := TimePredicate("StartTime", &start, &end)
	cte, where, args, err := BuildSearchSQL(query, mapper, timeCondition, timeParams)
	require.NoError(t, err)
	assert.Empty(t, cte)
	assert.Equal(t, "(Name = $3) AND StartTime >= $1::ubigint AND StartTime <= $2::ubigint", where)
	assert.Equal(t, []any{"1000", "2000", "test-span"}, args)
}

func TestBuildSearchSQL_GroupAND(t *testing.T) {
	mapper := func(field *FieldDefinition, _ *Query, _ *[]NamedParam) ([]ResolvedExpression, error) {
		return []ResolvedExpression{Text(field.Name)}, nil
	}

	query := &QueryNode{
		ID:   "g1",
		Type: "group",
		Group: &QueryGroup{
			LogicalOperator: "AND",
			Children: []QueryNode{
				{
					ID:   "c1",
					Type: "condition",
					Query: &Query{
						Field:         &FieldDefinition{Name: "Name", SearchScope: "field"},
						FieldOperator: "=",
						Value:         "a",
					},
				},
				{
					ID:   "c2",
					Type: "condition",
					Query: &Query{
						Field:         &FieldDefinition{Name: "Kind", SearchScope: "field"},
						FieldOperator: "=",
						Value:         "b",
					},
				},
			},
		},
	}

	start, end := uint64(1000), uint64(2000)
	timeCondition, timeParams := TimePredicate("StartTime", &start, &end)
	cte, where, args, err := BuildSearchSQL(query, mapper, timeCondition, timeParams)
	require.NoError(t, err)
	assert.Empty(t, cte)
	assert.Contains(t, where, "Name = $3 AND Kind = $4")
	assert.Len(t, args, 4)
	assert.Equal(t, "1000", args[0])
	assert.Equal(t, "2000", args[1])
}

func TestBuildSearchSQL_GlobalORs(t *testing.T) {
	mapper := func(field *FieldDefinition, _ *Query, _ *[]NamedParam) ([]ResolvedExpression, error) {
		if field.SearchScope == "global" {
			return TextExpressions([]string{"SearchText {COND}", "Name {COND}"}), nil
		}
		return []ResolvedExpression{Text(field.Name)}, nil
	}

	query := &QueryNode{
		ID:   "q1",
		Type: "condition",
		Query: &Query{
			Field:         &FieldDefinition{SearchScope: "global"},
			FieldOperator: "CONTAINS",
			Value:         "test",
		},
	}

	start, end := uint64(1000), uint64(2000)
	timeCondition, timeParams := TimePredicate("StartTime", &start, &end)
	_, where, args, err := BuildSearchSQL(query, mapper, timeCondition, timeParams)
	require.NoError(t, err)
	assert.Contains(t, where, "SearchText LIKE $3 OR Name LIKE $4")
	assert.Contains(t, args, "%test%")
}

func TestBuildConditions_MissingField(t *testing.T) {
	mapper := func(field *FieldDefinition, _ *Query, _ *[]NamedParam) ([]ResolvedExpression, error) {
		return []ResolvedExpression{Text(field.Name)}, nil
	}

	node := &QueryNode{
		ID:   "q1",
		Type: "condition",
		Query: &Query{
			FieldOperator: "=",
			Value:         "test",
		},
	}

	var params []NamedParam
	var conditions []string
	err := BuildConditions(node, &conditions, &params, mapper)
	assert.Error(t, err)
}

func TestInlineArrayPredicateKeepsAbsentArraysOutOfNegativeMatches(t *testing.T) {
	query := &Query{
		Field:         &FieldDefinition{Name: "retries", Type: "array"},
		FieldOperator: "NOT CONTAINS",
		Value:         "42",
	}
	var params []NamedParam

	resolved, err := ResolveValue("s.attributes", []any{"retries"}, "array", &params)
	require.NoError(t, err)
	predicate, err := BuildOperatorCondition(resolved, query, &params)

	require.NoError(t, err)
	assert.Contains(t, predicate, "variant_exists(s.attributes, $1)")
	assert.Contains(t, predicate, "otel_value_kind(s.attributes[$1]) = $2")
	assert.Contains(t, predicate, "and not exists")
	assert.Equal(t, []NamedParam{
		{Name: "attr_key_0", Value: "retries"},
		{Name: "attr_kind_1", Value: "array"},
		{Name: "value_2", Value: "42"},
	}, params)
}

// TestMultiExpressionNamedFieldJoinsWithAND verifies that a named field mapped
// to several expressions produces one valid conjunction.
func TestMultiExpressionNamedFieldJoinsWithAND(t *testing.T) {
	node := &QueryNode{
		Type: "condition",
		Query: &Query{
			Field:         &FieldDefinition{Name: "twoPlaces", SearchScope: "field", Type: "string"},
			FieldOperator: "=",
			Value:         "x",
		},
	}
	mapper := func(field *FieldDefinition, query *Query, params *[]NamedParam) ([]ResolvedExpression, error) {
		return TextExpressions([]string{"a.col", "b.col"}), nil
	}
	start := uint64(0)
	timeCondition, timeParams := TimePredicate("t", &start, nil)
	cte, where, args, err := BuildSearchSQL(node, mapper, timeCondition, timeParams)
	require.NoError(t, err)
	assert.Equal(t, "((a.col = $2 AND b.col = $3)) AND t >= $1::ubigint", where)
	assert.Empty(t, cte)
	assert.Len(t, args, 3)
}

func TestTimePredicateShapes(t *testing.T) {
	start, end := uint64(10), uint64(20)
	for _, tc := range []struct {
		name       string
		start, end *uint64
		wantSQL    string
		wantArgs   []NamedParam
	}{
		{"unbounded", nil, nil, "", nil},
		{"end only", nil, &end, "ts <= $1::ubigint", []NamedParam{{Name: "time_end", Value: unsignedScalar(end)}}},
		{"start only", &start, nil, "ts >= $1::ubigint", []NamedParam{{Name: "time_start", Value: unsignedScalar(start)}}},
		{"bounded", &start, &end, "ts >= $1::ubigint AND ts <= $2::ubigint", []NamedParam{{Name: "time_start", Value: unsignedScalar(start)}, {Name: "time_end", Value: unsignedScalar(end)}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotSQL, gotArgs := TimePredicate("ts", tc.start, tc.end)
			assert.Equal(t, tc.wantSQL, gotSQL)
			assert.Equal(t, tc.wantArgs, gotArgs)
		})
	}
}
