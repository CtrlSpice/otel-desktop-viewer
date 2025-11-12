package store

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

func TestBuildSQL(t *testing.T) {
	tests := []struct {
		name         string
		queryNode    *QueryNode
		signalType   string
		startTime    int64
		endTime      int64
		expectedCTE  string
		expectedSQL  string
		expectedArgs []any
		wantErr      bool
	}{
		{
			name: "simple attribute condition",
			queryNode: &QueryNode{
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
			signalType:  "traces",
			startTime:   1000,
			endTime:     2000,
			expectedCTE: "WITH search_params AS (SELECT ? as time_start, ? as time_end, ? as value_0)",
			expectedSQL: `(CASE 
		WHEN union_tag(ResourceAttributes['service.name']) = 'string' THEN ResourceAttributes['service.name']::VARCHAR LIKE value_0
		WHEN union_tag(ResourceAttributes['service.name']) = 'int64' THEN ResourceAttributes['service.name']::BIGINT LIKE value_0
		WHEN union_tag(ResourceAttributes['service.name']) = 'float64' THEN ResourceAttributes['service.name']::DOUBLE LIKE value_0
		WHEN union_tag(ResourceAttributes['service.name']) = 'boolean' THEN ResourceAttributes['service.name']::BOOLEAN LIKE value_0
		WHEN union_tag(ResourceAttributes['service.name']) = 'string_list' THEN ResourceAttributes['service.name']::VARCHAR[] LIKE value_0
		WHEN union_tag(ResourceAttributes['service.name']) = 'int64_list' THEN ResourceAttributes['service.name']::BIGINT[] LIKE value_0
		WHEN union_tag(ResourceAttributes['service.name']) = 'float64_list' THEN ResourceAttributes['service.name']::DOUBLE[] LIKE value_0
		WHEN union_tag(ResourceAttributes['service.name']) = 'boolean_list' THEN ResourceAttributes['service.name']::BOOLEAN[] LIKE value_0
		ELSE FALSE
	END) AND StartTime >= time_start AND StartTime <= time_end`,
			expectedArgs: []any{int64(1000), int64(2000), "%sample%"},
		},
		{
			name: "span attribute equality",
			queryNode: &QueryNode{
				ID:   "query-2",
				Type: "condition",
				Query: &Query{
					Field: &FieldDefinition{
						Name:           "currency.conversion.from",
						SearchScope:    "attribute",
						AttributeScope: "span",
					},
					FieldOperator: "=",
					Value:         "USD",
				},
			},
			signalType:  "traces",
			startTime:   1000,
			endTime:     2000,
			expectedCTE: "WITH search_params AS (SELECT ? as time_start, ? as time_end, ? as value_0)",
			expectedSQL: `(CASE 
		WHEN union_tag(Attributes['currency.conversion.from']) = 'string' THEN Attributes['currency.conversion.from']::VARCHAR = value_0
		WHEN union_tag(Attributes['currency.conversion.from']) = 'int64' THEN Attributes['currency.conversion.from']::BIGINT = value_0
		WHEN union_tag(Attributes['currency.conversion.from']) = 'float64' THEN Attributes['currency.conversion.from']::DOUBLE = value_0
		WHEN union_tag(Attributes['currency.conversion.from']) = 'boolean' THEN Attributes['currency.conversion.from']::BOOLEAN = value_0
		WHEN union_tag(Attributes['currency.conversion.from']) = 'string_list' THEN Attributes['currency.conversion.from']::VARCHAR[] = value_0
		WHEN union_tag(Attributes['currency.conversion.from']) = 'int64_list' THEN Attributes['currency.conversion.from']::BIGINT[] = value_0
		WHEN union_tag(Attributes['currency.conversion.from']) = 'float64_list' THEN Attributes['currency.conversion.from']::DOUBLE[] = value_0
		WHEN union_tag(Attributes['currency.conversion.from']) = 'boolean_list' THEN Attributes['currency.conversion.from']::BOOLEAN[] = value_0
		ELSE FALSE
	END) AND StartTime >= time_start AND StartTime <= time_end`,
			expectedArgs: []any{int64(1000), int64(2000), "USD"},
		},
		{
			name: "group with AND operator",
			queryNode: &QueryNode{
				ID:   "query-3",
				Type: "group",
				Group: &QueryGroup{
					LogicalOperator: "AND",
					Children: []QueryNode{
						{
							ID:   "query-4",
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
						{
							ID:   "query-5",
							Type: "condition",
							Query: &Query{
								Field: &FieldDefinition{
									Name:           "currency.conversion.from",
									SearchScope:    "attribute",
									AttributeScope: "span",
								},
								FieldOperator: "=",
								Value:         "USD",
							},
						},
					},
				},
			},
			signalType:  "traces",
			startTime:   1000,
			endTime:     2000,
			expectedCTE: "WITH search_params AS (SELECT ? as time_start, ? as time_end, ? as value_0, ? as value_1)",
			expectedSQL: `((CASE 
		WHEN union_tag(ResourceAttributes['service.name']) = 'string' THEN ResourceAttributes['service.name']::VARCHAR LIKE value_0
		WHEN union_tag(ResourceAttributes['service.name']) = 'int64' THEN ResourceAttributes['service.name']::BIGINT LIKE value_0
		WHEN union_tag(ResourceAttributes['service.name']) = 'float64' THEN ResourceAttributes['service.name']::DOUBLE LIKE value_0
		WHEN union_tag(ResourceAttributes['service.name']) = 'boolean' THEN ResourceAttributes['service.name']::BOOLEAN LIKE value_0
		WHEN union_tag(ResourceAttributes['service.name']) = 'string_list' THEN ResourceAttributes['service.name']::VARCHAR[] LIKE value_0
		WHEN union_tag(ResourceAttributes['service.name']) = 'int64_list' THEN ResourceAttributes['service.name']::BIGINT[] LIKE value_0
		WHEN union_tag(ResourceAttributes['service.name']) = 'float64_list' THEN ResourceAttributes['service.name']::DOUBLE[] LIKE value_0
		WHEN union_tag(ResourceAttributes['service.name']) = 'boolean_list' THEN ResourceAttributes['service.name']::BOOLEAN[] LIKE value_0
		ELSE FALSE
	END AND CASE 
		WHEN union_tag(Attributes['currency.conversion.from']) = 'string' THEN Attributes['currency.conversion.from']::VARCHAR = value_1
		WHEN union_tag(Attributes['currency.conversion.from']) = 'int64' THEN Attributes['currency.conversion.from']::BIGINT = value_1
		WHEN union_tag(Attributes['currency.conversion.from']) = 'float64' THEN Attributes['currency.conversion.from']::DOUBLE = value_1
		WHEN union_tag(Attributes['currency.conversion.from']) = 'boolean' THEN Attributes['currency.conversion.from']::BOOLEAN = value_1
		WHEN union_tag(Attributes['currency.conversion.from']) = 'string_list' THEN Attributes['currency.conversion.from']::VARCHAR[] = value_1
		WHEN union_tag(Attributes['currency.conversion.from']) = 'int64_list' THEN Attributes['currency.conversion.from']::BIGINT[] = value_1
		WHEN union_tag(Attributes['currency.conversion.from']) = 'float64_list' THEN Attributes['currency.conversion.from']::DOUBLE[] = value_1
		WHEN union_tag(Attributes['currency.conversion.from']) = 'boolean_list' THEN Attributes['currency.conversion.from']::BOOLEAN[] = value_1
		ELSE FALSE
	END)) AND StartTime >= time_start AND StartTime <= time_end`,
			expectedArgs: []any{int64(1000), int64(2000), "%sample%", "USD"},
		},
		{
			name: "non-attribute field",
			queryNode: &QueryNode{
				ID:   "query-6",
				Type: "condition",
				Query: &Query{
					Field: &FieldDefinition{
						Name:        "Name",
						SearchScope: "field",
					},
					FieldOperator: "=",
					Value:         "test-span",
				},
			},
			signalType:   "traces",
			startTime:    1000,
			endTime:      2000,
			expectedCTE:  "WITH search_params AS (SELECT ? as time_start, ? as time_end, ? as value_0)",
			expectedSQL:  "(Name = value_0) AND StartTime >= time_start AND StartTime <= time_end",
			expectedArgs: []any{int64(1000), int64(2000), "test-span"},
		},
		{
			name: "event attribute with EXISTS",
			queryNode: &QueryNode{
				ID:   "query-7",
				Type: "condition",
				Query: &Query{
					Field: &FieldDefinition{
						Name:           "event.name",
						SearchScope:    "attribute",
						AttributeScope: "event",
					},
					FieldOperator: "=",
					Value:         "click",
				},
			},
			signalType:  "traces",
			startTime:   1000,
			endTime:     2000,
			expectedCTE: "WITH search_params AS (SELECT ? as time_start, ? as time_end, ? as value_0)",
			expectedSQL: `(EXISTS(SELECT 1 FROM UNNEST(Events) AS event WHERE CASE 
		WHEN union_tag(event.Attributes['event.name']) = 'string' THEN event.Attributes['event.name']::VARCHAR = value_0
		WHEN union_tag(event.Attributes['event.name']) = 'int64' THEN event.Attributes['event.name']::BIGINT = value_0
		WHEN union_tag(event.Attributes['event.name']) = 'float64' THEN event.Attributes['event.name']::DOUBLE = value_0
		WHEN union_tag(event.Attributes['event.name']) = 'boolean' THEN event.Attributes['event.name']::BOOLEAN = value_0
		WHEN union_tag(event.Attributes['event.name']) = 'string_list' THEN event.Attributes['event.name']::VARCHAR[] = value_0
		WHEN union_tag(event.Attributes['event.name']) = 'int64_list' THEN event.Attributes['event.name']::BIGINT[] = value_0
		WHEN union_tag(event.Attributes['event.name']) = 'float64_list' THEN event.Attributes['event.name']::DOUBLE[] = value_0
		WHEN union_tag(event.Attributes['event.name']) = 'boolean_list' THEN event.Attributes['event.name']::BOOLEAN[] = value_0
		ELSE FALSE
	END)) AND StartTime >= time_start AND StartTime <= time_end`,
			expectedArgs: []any{int64(1000), int64(2000), "click"},
		},
		{
			name: "null attribute condition",
			queryNode: &QueryNode{
				ID:   "query-null",
				Type: "condition",
				Query: &Query{
					Field: &FieldDefinition{
						Name:           "service.name",
						SearchScope:    "attribute",
						AttributeScope: "resource",
					},
					FieldOperator: "=",
					Value:         "NULL",
				},
			},
			signalType:  "traces",
			startTime:   1000,
			endTime:     2000,
			expectedCTE: "WITH search_params AS (SELECT ? as time_start, ? as time_end)",
			expectedSQL: `(CASE 
		WHEN union_tag(ResourceAttributes['service.name']) = 'string' THEN ResourceAttributes['service.name']::VARCHAR IS NULL
		WHEN union_tag(ResourceAttributes['service.name']) = 'int64' THEN ResourceAttributes['service.name']::BIGINT IS NULL
		WHEN union_tag(ResourceAttributes['service.name']) = 'float64' THEN ResourceAttributes['service.name']::DOUBLE IS NULL
		WHEN union_tag(ResourceAttributes['service.name']) = 'boolean' THEN ResourceAttributes['service.name']::BOOLEAN IS NULL
		WHEN union_tag(ResourceAttributes['service.name']) = 'string_list' THEN ResourceAttributes['service.name']::VARCHAR[] IS NULL
		WHEN union_tag(ResourceAttributes['service.name']) = 'int64_list' THEN ResourceAttributes['service.name']::BIGINT[] IS NULL
		WHEN union_tag(ResourceAttributes['service.name']) = 'float64_list' THEN ResourceAttributes['service.name']::DOUBLE[] IS NULL
		WHEN union_tag(ResourceAttributes['service.name']) = 'boolean_list' THEN ResourceAttributes['service.name']::BOOLEAN[] IS NULL
		ELSE FALSE
	END) AND StartTime >= time_start AND StartTime <= time_end`,
			expectedArgs: []any{int64(1000), int64(2000)},
		},
		{
			name: "not null attribute condition",
			queryNode: &QueryNode{
				ID:   "query-not-null",
				Type: "condition",
				Query: &Query{
					Field: &FieldDefinition{
						Name:           "service.name",
						SearchScope:    "attribute",
						AttributeScope: "resource",
					},
					FieldOperator: "!=",
					Value:         "NULL",
				},
			},
			signalType:  "traces",
			startTime:   1000,
			endTime:     2000,
			expectedCTE: "WITH search_params AS (SELECT ? as time_start, ? as time_end)",
			expectedSQL: `(CASE 
		WHEN union_tag(ResourceAttributes['service.name']) = 'string' THEN ResourceAttributes['service.name']::VARCHAR IS NOT NULL
		WHEN union_tag(ResourceAttributes['service.name']) = 'int64' THEN ResourceAttributes['service.name']::BIGINT IS NOT NULL
		WHEN union_tag(ResourceAttributes['service.name']) = 'float64' THEN ResourceAttributes['service.name']::DOUBLE IS NOT NULL
		WHEN union_tag(ResourceAttributes['service.name']) = 'boolean' THEN ResourceAttributes['service.name']::BOOLEAN IS NOT NULL
		WHEN union_tag(ResourceAttributes['service.name']) = 'string_list' THEN ResourceAttributes['service.name']::VARCHAR[] IS NOT NULL
		WHEN union_tag(ResourceAttributes['service.name']) = 'int64_list' THEN ResourceAttributes['service.name']::BIGINT[] IS NOT NULL
		WHEN union_tag(ResourceAttributes['service.name']) = 'float64_list' THEN ResourceAttributes['service.name']::DOUBLE[] IS NOT NULL
		WHEN union_tag(ResourceAttributes['service.name']) = 'boolean_list' THEN ResourceAttributes['service.name']::BOOLEAN[] IS NOT NULL
		ELSE FALSE
	END) AND StartTime >= time_start AND StartTime <= time_end`,
			expectedArgs: []any{int64(1000), int64(2000)},
		},
		{
			name: "array IN attribute condition",
			queryNode: &QueryNode{
				ID:   "query-array-in",
				Type: "condition",
				Query: &Query{
					Field: &FieldDefinition{
						Name:           "array.example",
						SearchScope:    "attribute",
						AttributeScope: "resource",
					},
					FieldOperator: "IN",
					Value:         "[example1,example2,example3]",
				},
			},
			signalType:  "traces",
			startTime:   1000,
			endTime:     2000,
			expectedCTE: "WITH search_params AS (SELECT ? as time_start, ? as time_end, ? as value_0)",
			expectedSQL: `(CASE 
		WHEN union_tag(ResourceAttributes['array.example']) = 'string' THEN ResourceAttributes['array.example']::VARCHAR IN value_0
		WHEN union_tag(ResourceAttributes['array.example']) = 'int64' THEN ResourceAttributes['array.example']::BIGINT IN value_0
		WHEN union_tag(ResourceAttributes['array.example']) = 'float64' THEN ResourceAttributes['array.example']::DOUBLE IN value_0
		WHEN union_tag(ResourceAttributes['array.example']) = 'boolean' THEN ResourceAttributes['array.example']::BOOLEAN IN value_0
		WHEN union_tag(ResourceAttributes['array.example']) = 'string_list' THEN ResourceAttributes['array.example']::VARCHAR[] IN value_0
		WHEN union_tag(ResourceAttributes['array.example']) = 'int64_list' THEN ResourceAttributes['array.example']::BIGINT[] IN value_0
		WHEN union_tag(ResourceAttributes['array.example']) = 'float64_list' THEN ResourceAttributes['array.example']::DOUBLE[] IN value_0
		WHEN union_tag(ResourceAttributes['array.example']) = 'boolean_list' THEN ResourceAttributes['array.example']::BOOLEAN[] IN value_0
		ELSE FALSE
	END) AND StartTime >= time_start AND StartTime <= time_end`,
			expectedArgs: []any{int64(1000), int64(2000), []any{"example1", "example2", "example3"}},
		},
		{
			name: "array NOT IN attribute condition",
			queryNode: &QueryNode{
				ID:   "query-array-not-in",
				Type: "condition",
				Query: &Query{
					Field: &FieldDefinition{
						Name:           "array.example",
						SearchScope:    "attribute",
						AttributeScope: "resource",
					},
					FieldOperator: "NOT IN",
					Value:         "[bad1,bad2]",
				},
			},
			signalType:  "traces",
			startTime:   1000,
			endTime:     2000,
			expectedCTE: "WITH search_params AS (SELECT ? as time_start, ? as time_end, ? as value_0)",
			expectedSQL: `(CASE 
		WHEN union_tag(ResourceAttributes['array.example']) = 'string' THEN ResourceAttributes['array.example']::VARCHAR NOT IN value_0
		WHEN union_tag(ResourceAttributes['array.example']) = 'int64' THEN ResourceAttributes['array.example']::BIGINT NOT IN value_0
		WHEN union_tag(ResourceAttributes['array.example']) = 'float64' THEN ResourceAttributes['array.example']::DOUBLE NOT IN value_0
		WHEN union_tag(ResourceAttributes['array.example']) = 'boolean' THEN ResourceAttributes['array.example']::BOOLEAN NOT IN value_0
		WHEN union_tag(ResourceAttributes['array.example']) = 'string_list' THEN ResourceAttributes['array.example']::VARCHAR[] NOT IN value_0
		WHEN union_tag(ResourceAttributes['array.example']) = 'int64_list' THEN ResourceAttributes['array.example']::BIGINT[] NOT IN value_0
		WHEN union_tag(ResourceAttributes['array.example']) = 'float64_list' THEN ResourceAttributes['array.example']::DOUBLE[] NOT IN value_0
		WHEN union_tag(ResourceAttributes['array.example']) = 'boolean_list' THEN ResourceAttributes['array.example']::BOOLEAN[] NOT IN value_0
		ELSE FALSE
	END) AND StartTime >= time_start AND StartTime <= time_end`,
			expectedArgs: []any{int64(1000), int64(2000), []any{"bad1", "bad2"}},
		},
		{
			name:         "nil query node",
			queryNode:    nil,
			signalType:   "traces",
			startTime:    1000,
			endTime:      2000,
			expectedCTE:  "WITH search_params AS (SELECT ? as time_start, ? as time_end)",
			expectedSQL:  "StartTime >= time_start AND StartTime <= time_end",
			expectedArgs: []any{int64(1000), int64(2000)},
		},
		{
			name: "invalid condition - missing field",
			queryNode: &QueryNode{
				ID:   "query-8",
				Type: "condition",
				Query: &Query{
					FieldOperator: "=",
					Value:         "test",
				},
			},
			signalType: "traces",
			startTime:  1000,
			endTime:    2000,
			wantErr:    true,
		},
		{
			name: "event field equality",
			queryNode: &QueryNode{
				ID:   "query-event-field",
				Type: "condition",
				Query: &Query{
					Field: &FieldDefinition{
						Name:        "event.name",
						SearchScope: "field",
					},
					FieldOperator: "=",
					Value:         "click",
				},
			},
			signalType:   "traces",
			startTime:    1000,
			endTime:      2000,
			expectedCTE:  "WITH search_params AS (SELECT ? as time_start, ? as time_end, ? as value_0)",
			expectedSQL:  "(EXISTS(SELECT 1 FROM UNNEST(Events) AS item WHERE item.'Name' = value_0)) AND StartTime >= time_start AND StartTime <= time_end",
			expectedArgs: []any{int64(1000), int64(2000), "click"},
		},
		{
			name: "event field contains",
			queryNode: &QueryNode{
				ID:   "query-event-contains",
				Type: "condition",
				Query: &Query{
					Field: &FieldDefinition{
						Name:        "event.name",
						SearchScope: "field",
					},
					FieldOperator: "CONTAINS",
					Value:         "click",
				},
			},
			signalType:   "traces",
			startTime:    1000,
			endTime:      2000,
			expectedCTE:  "WITH search_params AS (SELECT ? as time_start, ? as time_end, ? as value_0)",
			expectedSQL:  "(EXISTS(SELECT 1 FROM UNNEST(Events) AS item WHERE item.'Name' LIKE value_0)) AND StartTime >= time_start AND StartTime <= time_end",
			expectedArgs: []any{int64(1000), int64(2000), "%click%"},
		},
		{
			name: "link field equality",
			queryNode: &QueryNode{
				ID:   "query-link-field",
				Type: "condition",
				Query: &Query{
					Field: &FieldDefinition{
						Name:        "link.traceID",
						SearchScope: "field",
					},
					FieldOperator: "=",
					Value:         "abc123",
				},
			},
			signalType:   "traces",
			startTime:    1000,
			endTime:      2000,
			expectedCTE:  "WITH search_params AS (SELECT ? as time_start, ? as time_end, ? as value_0)",
			expectedSQL:  "(EXISTS(SELECT 1 FROM UNNEST(Links) AS item WHERE item.'TraceID' = value_0)) AND StartTime >= time_start AND StartTime <= time_end",
			expectedArgs: []any{int64(1000), int64(2000), "abc123"},
		},
		{
			name: "link field contains",
			queryNode: &QueryNode{
				ID:   "query-link-contains",
				Type: "condition",
				Query: &Query{
					Field: &FieldDefinition{
						Name:        "link.spanID",
						SearchScope: "field",
					},
					FieldOperator: "CONTAINS",
					Value:         "span",
				},
			},
			signalType:   "traces",
			startTime:    1000,
			endTime:      2000,
			expectedCTE:  "WITH search_params AS (SELECT ? as time_start, ? as time_end, ? as value_0)",
			expectedSQL:  "(EXISTS(SELECT 1 FROM UNNEST(Links) AS item WHERE item.'SpanID' LIKE value_0)) AND StartTime >= time_start AND StartTime <= time_end",
			expectedArgs: []any{int64(1000), int64(2000), "%span%"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cteSQL, whereSQL, args, err := BuildSQL(tt.queryNode, tt.signalType, tt.startTime, tt.endTime)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expectedCTE, cteSQL)
			assert.Equal(t, tt.expectedSQL, whereSQL)
			assert.ElementsMatch(t, tt.expectedArgs, args)
		})
	}
}

func TestBuildOperatorCondition(t *testing.T) {
	tests := []struct {
		name         string
		expression   string
		operator     string
		value        string
		expectedSQL  string
		expectedArgs map[string]any
		wantErr      bool
	}{
		{
			name:         "equality operator",
			expression:   "Name",
			operator:     "=",
			value:        "test-span",
			expectedSQL:  "Name = value_0",
			expectedArgs: map[string]any{"value_0": "test-span"},
		},
		{
			name:         "contains operator",
			expression:   "Name",
			operator:     "CONTAINS",
			value:        "test",
			expectedSQL:  "Name LIKE value_0",
			expectedArgs: map[string]any{"value_0": "%test%"},
		},
		{
			name:         "starts with operator",
			expression:   "Name",
			operator:     "^",
			value:        "test",
			expectedSQL:  "Name LIKE value_0",
			expectedArgs: map[string]any{"value_0": "test%"},
		},
		{
			name:         "ends with operator",
			expression:   "Name",
			operator:     "$",
			value:        "span",
			expectedSQL:  "Name LIKE value_0",
			expectedArgs: map[string]any{"value_0": "%span"},
		},
		{
			name:         "not contains operator",
			expression:   "Name",
			operator:     "NOT CONTAINS",
			value:        "test",
			expectedSQL:  "Name NOT LIKE value_0",
			expectedArgs: map[string]any{"value_0": "%test%"},
		},
		{
			name:         "IN operator",
			expression:   "Name",
			operator:     "IN",
			value:        "[test1,test2,test3]",
			expectedSQL:  "Name IN value_0",
			expectedArgs: map[string]any{"value_0": []any{"test1", "test2", "test3"}},
		},
		{
			name:         "NULL value with equals",
			expression:   "Name",
			operator:     "=",
			value:        "NULL",
			expectedSQL:  "Name IS NULL",
			expectedArgs: map[string]any{},
		},
		{
			name:         "NULL value with not equals",
			expression:   "Name",
			operator:     "!=",
			value:        "NULL",
			expectedSQL:  "Name IS NOT NULL",
			expectedArgs: map[string]any{},
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			namedArgs := make(map[string]any)
			// Initialize with time parameters as BuildSQL does
			namedArgs["time_start"] = int64(1000)
			namedArgs["time_end"] = int64(2000)

			sql, err := buildOperatorCondition(tt.expression, tt.operator, tt.value, &namedArgs)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expectedSQL, sql)

			// Build expected map with time parameters included
			expectedMap := make(map[string]any)
			expectedMap["time_start"] = int64(1000)
			expectedMap["time_end"] = int64(2000)
			for k, v := range tt.expectedArgs {
				expectedMap[k] = v
			}
			assert.Equal(t, expectedMap, namedArgs)
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
			result := parseArrayValue(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildSQL_GlobalSearch(t *testing.T) {
	tests := []struct {
		name            string
		queryNode       *QueryNode
		signalType      string
		startTime       int64
		endTime         int64
		expectedSQLPart string // Part of SQL to verify
		expectedValue   string // Expected search value (with operator formatting)
		wantErr         bool
	}{
		{
			name: "global search with CONTAINS",
			queryNode: &QueryNode{
				ID:   "query-global-1",
				Type: "condition",
				Query: &Query{
					Field: &FieldDefinition{
						SearchScope: "global",
					},
					FieldOperator: "CONTAINS",
					Value:         "test",
				},
			},
			signalType:      "traces",
			startTime:       1000,
			endTime:         2000,
			expectedSQLPart: "TraceID LIKE value_",
			expectedValue:   "%test%",
		},
		{
			name: "global search with equality",
			queryNode: &QueryNode{
				ID:   "query-global-2",
				Type: "condition",
				Query: &Query{
					Field: &FieldDefinition{
						SearchScope: "global",
					},
					FieldOperator: "=",
					Value:         "exact-match",
				},
			},
			signalType:      "traces",
			startTime:       1000,
			endTime:         2000,
			expectedSQLPart: "TraceID = value_",
			expectedValue:   "exact-match",
		},
		{
			name: "global search with starts with",
			queryNode: &QueryNode{
				ID:   "query-global-3",
				Type: "condition",
				Query: &Query{
					Field: &FieldDefinition{
						SearchScope: "global",
					},
					FieldOperator: "^",
					Value:         "prefix",
				},
			},
			signalType:      "traces",
			startTime:       1000,
			endTime:         2000,
			expectedSQLPart: "TraceID LIKE value_",
			expectedValue:   "prefix%",
		},
		{
			name: "global search combined with field condition (AND)",
			queryNode: &QueryNode{
				ID:   "query-global-4",
				Type: "group",
				Group: &QueryGroup{
					LogicalOperator: "AND",
					Children: []QueryNode{
						{
							ID:   "query-global-4a",
							Type: "condition",
							Query: &Query{
								Field: &FieldDefinition{
									SearchScope: "global",
								},
								FieldOperator: "CONTAINS",
								Value:         "test",
							},
						},
						{
							ID:   "query-global-4b",
							Type: "condition",
							Query: &Query{
								Field: &FieldDefinition{
									Name:        "Name",
									SearchScope: "field",
								},
								FieldOperator: "=",
								Value:         "specific-span",
							},
						},
					},
				},
			},
			signalType:      "traces",
			startTime:       1000,
			endTime:         2000,
			expectedSQLPart: "Name = value_",
			expectedValue:   "%test%",
		},
		{
			name: "global search includes resource attributes",
			queryNode: &QueryNode{
				ID:   "query-global-5",
				Type: "condition",
				Query: &Query{
					Field: &FieldDefinition{
						SearchScope: "global",
					},
					FieldOperator: "CONTAINS",
					Value:         "service",
				},
			},
			signalType:      "traces",
			startTime:       1000,
			endTime:         2000,
			expectedSQLPart: "map_entries(s.ResourceAttributes)",
			expectedValue:   "%service%",
		},
		{
			name: "global search includes scope attributes",
			queryNode: &QueryNode{
				ID:   "query-global-6",
				Type: "condition",
				Query: &Query{
					Field: &FieldDefinition{
						SearchScope: "global",
					},
					FieldOperator: "CONTAINS",
					Value:         "scope-value",
				},
			},
			signalType:      "traces",
			startTime:       1000,
			endTime:         2000,
			expectedSQLPart: "map_entries(s.ScopeAttributes)",
			expectedValue:   "%scope-value%",
		},
		{
			name: "global search includes span attributes",
			queryNode: &QueryNode{
				ID:   "query-global-7",
				Type: "condition",
				Query: &Query{
					Field: &FieldDefinition{
						SearchScope: "global",
					},
					FieldOperator: "CONTAINS",
					Value:         "span-attr",
				},
			},
			signalType:      "traces",
			startTime:       1000,
			endTime:         2000,
			expectedSQLPart: "map_entries(s.Attributes)",
			expectedValue:   "%span-attr%",
		},
		{
			name: "global search includes event fields (array fields)",
			queryNode: &QueryNode{
				ID:   "query-global-8",
				Type: "condition",
				Query: &Query{
					Field: &FieldDefinition{
						SearchScope: "global",
					},
					FieldOperator: "CONTAINS",
					Value:         "click",
				},
			},
			signalType:      "traces",
			startTime:       1000,
			endTime:         2000,
			expectedSQLPart: "UNNEST(s.Events) WHERE unnest.Name",
			expectedValue:   "%click%",
		},
		{
			name: "global search includes link fields (array fields)",
			queryNode: &QueryNode{
				ID:   "query-global-9",
				Type: "condition",
				Query: &Query{
					Field: &FieldDefinition{
						SearchScope: "global",
					},
					FieldOperator: "CONTAINS",
					Value:         "trace-id-value",
				},
			},
			signalType:      "traces",
			startTime:       1000,
			endTime:         2000,
			expectedSQLPart: "UNNEST(s.Links) WHERE unnest.TraceID",
			expectedValue:   "%trace-id-value%",
		},
		{
			name: "global search includes event attributes (attributes in array fields)",
			queryNode: &QueryNode{
				ID:   "query-global-10",
				Type: "condition",
				Query: &Query{
					Field: &FieldDefinition{
						SearchScope: "global",
					},
					FieldOperator: "CONTAINS",
					Value:         "event-attr",
				},
			},
			signalType:      "traces",
			startTime:       1000,
			endTime:         2000,
			expectedSQLPart: "(SELECT UNNEST(s.Events) AS event_data) WHERE EXISTS(SELECT 1 FROM UNNEST(map_entries(event_data.Attributes))",
			expectedValue:   "%event-attr%",
		},
		{
			name: "global search includes link attributes (attributes in array fields)",
			queryNode: &QueryNode{
				ID:   "query-global-11",
				Type: "condition",
				Query: &Query{
					Field: &FieldDefinition{
						SearchScope: "global",
					},
					FieldOperator: "CONTAINS",
					Value:         "link-attr",
				},
			},
			signalType:      "traces",
			startTime:       1000,
			endTime:         2000,
			expectedSQLPart: "(SELECT UNNEST(s.Links) AS link_data) WHERE EXISTS(SELECT 1 FROM UNNEST(map_entries(link_data.Attributes))",
			expectedValue:   "%link-attr%",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cte, sql, args, err := BuildSQL(tt.queryNode, tt.signalType, tt.startTime, tt.endTime)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			// Verify CTE contains time parameters
			assert.Contains(t, cte, "time_start")
			assert.Contains(t, cte, "time_end")
			// Verify SQL contains expected parts
			assert.Contains(t, sql, tt.expectedSQLPart)
			assert.Contains(t, sql, "StartTime >= time_start")
			assert.Contains(t, sql, "StartTime <= time_end")
			// Verify time args
			assert.Equal(t, int64(1000), args[0])
			assert.Equal(t, int64(2000), args[1])
			// Verify global search expressions are ORed together
			if tt.queryNode.Query != nil && tt.queryNode.Query.Field != nil && tt.queryNode.Query.Field.SearchScope == "global" {
				assert.Contains(t, sql, " OR ", "Global search should have OR conditions")
				// Verify all user args have the expected value (for global search)
				for i := 2; i < len(args); i++ {
					assert.Equal(t, tt.expectedValue, args[i], "All global search args should have the same value")
				}
			} else if tt.queryNode.Group != nil {
				// For group with global search, verify it's present
				assert.Contains(t, sql, " OR ", "Group with global search should have OR conditions")
			}
		})
	}
}
