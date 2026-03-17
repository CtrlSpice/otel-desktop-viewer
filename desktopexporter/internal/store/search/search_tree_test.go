package search

import (
	"testing"

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
			expectedSQL:    "Name = value_0",
			expectedParams: []NamedParam{{"value_0", "test-span"}},
		},
		{
			name:           "contains operator",
			expression:     "Name",
			operator:       "CONTAINS",
			value:          "test",
			expectedSQL:    "Name LIKE value_0",
			expectedParams: []NamedParam{{"value_0", "%test%"}},
		},
		{
			name:           "starts with operator",
			expression:     "Name",
			operator:       "^",
			value:          "test",
			expectedSQL:    "Name LIKE value_0",
			expectedParams: []NamedParam{{"value_0", "test%"}},
		},
		{
			name:           "ends with operator",
			expression:     "Name",
			operator:       "$",
			value:          "span",
			expectedSQL:    "Name LIKE value_0",
			expectedParams: []NamedParam{{"value_0", "%span"}},
		},
		{
			name:           "not contains operator",
			expression:     "Name",
			operator:       "NOT CONTAINS",
			value:          "test",
			expectedSQL:    "Name NOT LIKE value_0",
			expectedParams: []NamedParam{{"value_0", "%test%"}},
		},
		{
			name:           "IN operator",
			expression:     "Name",
			operator:       "IN",
			value:          "[test1,test2,test3]",
			expectedSQL:    "Name IN value_0",
			expectedParams: []NamedParam{{"value_0", []any{"test1", "test2", "test3"}}},
		},
		{
			name:           "NULL value with equals",
			expression:     "Name",
			operator:       "=",
			value:          "NULL",
			expectedSQL:    "Name IS NULL",
			expectedParams: nil,
		},
		{
			name:           "NULL value with not equals",
			expression:     "Name",
			operator:       "!=",
			value:          "NULL",
			expectedSQL:    "Name IS NOT NULL",
			expectedParams: nil,
		},
		{
			name:       "unsupported operator with NULL",
			expression: "Name",
			operator:   "CONTAINS",
			value:      "NULL",
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
			expectedSQL:    "s.SearchText = value_0",
			expectedParams: []NamedParam{{"value_0", "test"}},
		},
		{
			name:           "placeholder expression CONTAINS",
			expression:     "s.SearchText {COND}",
			operator:       "CONTAINS",
			value:          "test",
			expectedSQL:    "s.SearchText LIKE value_0",
			expectedParams: []NamedParam{{"value_0", "%test%"}},
		},
		{
			name:           "placeholder expression NULL",
			expression:     "s.SearchText {COND}",
			operator:       "=",
			value:          "NULL",
			expectedSQL:    "s.SearchText IS NULL",
			expectedParams: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := []NamedParam{
				{"time_start", int64(1000)},
				{"time_end", int64(2000)},
			}

			query := &Query{
				Field:         &FieldDefinition{Type: ""},
				FieldOperator: tt.operator,
				Value:         tt.value,
			}

			sql, err := BuildOperatorCondition(tt.expression, query, &params)
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

func TestBuildOperatorCondition_ArrayTypes(t *testing.T) {
	tests := []struct {
		name           string
		expression     string
		fieldType      string
		operator       string
		value          string
		expectedSQL    string
		expectedParams []NamedParam
		wantErr        bool
	}{
		{
			name:           "string array CONTAINS",
			expression:     "a.Value",
			fieldType:      "string[]",
			operator:       "CONTAINS",
			value:          "hello",
			expectedSQL:    "list_contains(CAST(a.Value AS VARCHAR[]), value_0)",
			expectedParams: []NamedParam{{"value_0", "hello"}},
		},
		{
			name:           "int64 array CONTAINS",
			expression:     "a.Value",
			fieldType:      "int64[]",
			operator:       "CONTAINS",
			value:          "42",
			expectedSQL:    "list_contains(CAST(a.Value AS BIGINT[]), value_0)",
			expectedParams: []NamedParam{{"value_0", int64(42)}},
		},
		{
			name:           "float64 array CONTAINS",
			expression:     "a.Value",
			fieldType:      "float64[]",
			operator:       "CONTAINS",
			value:          "3.14",
			expectedSQL:    "list_contains(CAST(a.Value AS DOUBLE[]), value_0)",
			expectedParams: []NamedParam{{"value_0", 3.14}},
		},
		{
			name:           "boolean array CONTAINS",
			expression:     "a.Value",
			fieldType:      "boolean[]",
			operator:       "CONTAINS",
			value:          "true",
			expectedSQL:    "list_contains(CAST(a.Value AS BOOLEAN[]), value_0)",
			expectedParams: []NamedParam{{"value_0", true}},
		},
		{
			name:           "string array IN",
			expression:     "a.Value",
			fieldType:      "string[]",
			operator:       "IN",
			value:          "[one,two,three]",
			expectedSQL:    "list_has_all(CAST(a.Value AS VARCHAR[]), value_0)",
			expectedParams: []NamedParam{{"value_0", []any{"one", "two", "three"}}},
		},
		{
			name:           "string array NOT IN",
			expression:     "a.Value",
			fieldType:      "string[]",
			operator:       "NOT IN",
			value:          "[bad1,bad2]",
			expectedSQL:    "NOT list_has_all(CAST(a.Value AS VARCHAR[]), value_0)",
			expectedParams: []NamedParam{{"value_0", []any{"bad1", "bad2"}}},
		},
		{
			name:           "string array NOT CONTAINS",
			expression:     "a.Value",
			fieldType:      "string[]",
			operator:       "NOT CONTAINS",
			value:          "gone",
			expectedSQL:    "NOT list_contains(CAST(a.Value AS VARCHAR[]), value_0)",
			expectedParams: []NamedParam{{"value_0", "gone"}},
		},
		{
			name:           "string array = with array value",
			expression:     "a.Value",
			fieldType:      "string[]",
			operator:       "=",
			value:          "[one,two,three]",
			expectedSQL:    "CAST(a.Value AS VARCHAR[]) = value_0",
			expectedParams: []NamedParam{{"value_0", []any{"one", "two", "three"}}},
		},
		{
			name:           "string array = with scalar value",
			expression:     "a.Value",
			fieldType:      "string[]",
			operator:       "=",
			value:          "single",
			expectedSQL:    "CAST(a.Value AS VARCHAR[]) = value_0",
			expectedParams: []NamedParam{{"value_0", "single"}},
		},
		{
			name:           "string array != with array value",
			expression:     "a.Value",
			fieldType:      "string[]",
			operator:       "!=",
			value:          "[x,y]",
			expectedSQL:    "CAST(a.Value AS VARCHAR[]) != value_0",
			expectedParams: []NamedParam{{"value_0", []any{"x", "y"}}},
		},
		{
			name:           "string array != with scalar value",
			expression:     "a.Value",
			fieldType:      "string[]",
			operator:       "!=",
			value:          "other",
			expectedSQL:    "CAST(a.Value AS VARCHAR[]) != value_0",
			expectedParams: []NamedParam{{"value_0", "other"}},
		},
		{
			name:       "unsupported array type",
			expression: "a.Value",
			fieldType:  "map[]",
			operator:   "CONTAINS",
			value:      "x",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := []NamedParam{
				{"time_start", int64(1000)},
				{"time_end", int64(2000)},
			}

			query := &Query{
				Field:         &FieldDefinition{Type: tt.fieldType},
				FieldOperator: tt.operator,
				Value:         tt.value,
			}

			sql, err := BuildOperatorCondition(tt.expression, query, &params)
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

func TestParseArrayValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []any
	}{
		{
			name:     "simple array",
			input:    "[value1,value2,value3]",
			expected: []any{"value1", "value2", "value3"},
		},
		{
			name:     "array with spaces",
			input:    "[ value1 , value2 , value3 ]",
			expected: []any{"value1", "value2", "value3"},
		},
		{
			name:     "empty array",
			input:    "[]",
			expected: nil,
		},
		{
			name:     "single value",
			input:    "[single]",
			expected: []any{"single"},
		},
		{
			name:     "array with empty values",
			input:    "[value1,,value3]",
			expected: []any{"value1", "value3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseArrayValue(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertValueForArrayType(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		arrayType string
		expected  any
	}{
		{name: "int64 valid", value: "42", arrayType: "int64[]", expected: int64(42)},
		{name: "int64 invalid", value: "abc", arrayType: "int64[]", expected: "abc"},
		{name: "float64 valid", value: "3.14", arrayType: "float64[]", expected: 3.14},
		{name: "float64 invalid", value: "abc", arrayType: "float64[]", expected: "abc"},
		{name: "boolean true", value: "true", arrayType: "boolean[]", expected: true},
		{name: "boolean false", value: "false", arrayType: "boolean[]", expected: false},
		{name: "boolean invalid", value: "abc", arrayType: "boolean[]", expected: "abc"},
		{name: "string passthrough", value: "hello", arrayType: "string[]", expected: "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertValueForArrayType(tt.value, tt.arrayType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildSearchSQL_NilQuery(t *testing.T) {
	mapper := func(field *FieldDefinition, _ *[]NamedParam) ([]string, error) {
		return []string{field.Name}, nil
	}

	cte, where, args, err := BuildSearchSQL(nil, 1000, 2000, mapper, "StartTime >= time_start AND StartTime <= time_end")
	require.NoError(t, err)
	assert.Equal(t, "with search_params as (select ? as time_start, ? as time_end)", cte)
	assert.Equal(t, "StartTime >= time_start AND StartTime <= time_end", where)
	assert.Equal(t, []any{int64(1000), int64(2000)}, args)
}

func TestBuildSearchSQL_SimpleCondition(t *testing.T) {
	mapper := func(field *FieldDefinition, _ *[]NamedParam) ([]string, error) {
		return []string{field.Name}, nil
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

	cte, where, args, err := BuildSearchSQL(query, 1000, 2000, mapper, "StartTime >= time_start AND StartTime <= time_end")
	require.NoError(t, err)
	assert.Equal(t, "with search_params as (select ? as time_start, ? as time_end, ? as value_0)", cte)
	assert.Equal(t, "(Name = value_0) AND StartTime >= time_start AND StartTime <= time_end", where)
	assert.Equal(t, []any{int64(1000), int64(2000), "test-span"}, args)
}

func TestBuildSearchSQL_GroupAND(t *testing.T) {
	mapper := func(field *FieldDefinition, _ *[]NamedParam) ([]string, error) {
		return []string{field.Name}, nil
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

	cte, where, args, err := BuildSearchSQL(query, 1000, 2000, mapper, "StartTime >= time_start AND StartTime <= time_end")
	require.NoError(t, err)
	assert.Contains(t, cte, "value_0")
	assert.Contains(t, cte, "value_1")
	assert.Contains(t, where, "Name = value_0 AND Kind = value_1")
	assert.Len(t, args, 4)
	assert.Equal(t, int64(1000), args[0])
	assert.Equal(t, int64(2000), args[1])
}

func TestBuildSearchSQL_GlobalORs(t *testing.T) {
	mapper := func(field *FieldDefinition, _ *[]NamedParam) ([]string, error) {
		if field.SearchScope == "global" {
			return []string{"SearchText {COND}", "Name {COND}"}, nil
		}
		return []string{field.Name}, nil
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

	_, where, args, err := BuildSearchSQL(query, 1000, 2000, mapper, "StartTime >= time_start AND StartTime <= time_end")
	require.NoError(t, err)
	assert.Contains(t, where, "SearchText LIKE value_0 OR Name LIKE value_1")
	assert.Contains(t, args, "%test%")
}

func TestBuildConditions_MissingField(t *testing.T) {
	mapper := func(field *FieldDefinition, _ *[]NamedParam) ([]string, error) {
		return []string{field.Name}, nil
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
