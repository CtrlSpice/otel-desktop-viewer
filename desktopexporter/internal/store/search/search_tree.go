package search

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var (
	ErrInvalidQuery = errors.New("invalid search query")
)

// QueryNode represents a parsed query tree from the frontend
type QueryNode struct {
	ID    string      `json:"id"`
	Type  string      `json:"type"` // "condition" or "group"
	Query *Query      `json:"query,omitempty"`
	Group *QueryGroup `json:"group,omitempty"`
}

// Query holds a single condition.
type Query struct {
	Field         *FieldDefinition `json:"field"`
	FieldOperator string           `json:"fieldOperator"`
	Value         string           `json:"value"`
}

// FieldDefinition describes a field or attribute used in a condition.
type FieldDefinition struct {
	Name           string `json:"name,omitempty"`
	SearchScope    string `json:"searchScope"`
	AttributeScope string `json:"attributeScope,omitempty"`
	Type           string `json:"type,omitempty"`
}

// QueryGroup holds a logical group (AND/OR) of children.
type QueryGroup struct {
	LogicalOperator string      `json:"logicalOperator"` // "AND" or "OR"
	Children        []QueryNode `json:"children"`
}

// NamedParam is a positional parameter with a CTE column name and its value.
// Using a slice of these instead of a map guarantees insertion-order alignment
// between the CTE columns and the positional ? args.
type NamedParam struct {
	Name  string
	Value any
}

// FieldMapper maps a FieldDefinition to one or more SQL expressions.
// Signal-specific code provides this to the generic tree walker.
// The params slice is provided so mappers can add their own CTE parameters
// (e.g. for parameterized attribute scope/key lookups).
type FieldMapper func(field *FieldDefinition, query *Query, params *[]NamedParam) ([]string, error)

// PredicateToken marks an expression that is already a complete boolean, so
// BuildOperatorCondition returns it untouched instead of appending the operator
// and value.
//
// It exists for predicates a mapper can answer better than the generic
// machinery. The motivating case: an equality test on a string attribute is a
// membership test against a content-derived id, computable in Go, so the
// expression becomes list_contains(ids, <literal uuid>) with the operator and
// value already consumed -- 20x faster than resolving the value through the
// dictionary at query time.
const PredicateToken = "{PREDICATE}"

// Complete marks expr as a finished boolean, so no operator or value is
// appended to it.
func Complete(expr string) string { return PredicateToken + expr }

// ParseQueryTree converts JSON from frontend to QueryNode struct.
func ParseQueryTree(jsonData any) (*QueryNode, error) {
	jsonBytes, err := json.Marshal(jsonData)
	if err != nil {
		return nil, fmt.Errorf("ParseQueryTree: %w: %w", ErrInvalidQuery, err)
	}

	var queryNode QueryNode
	if err := json.Unmarshal(jsonBytes, &queryNode); err != nil {
		return nil, fmt.Errorf("ParseQueryTree: %w: %w", ErrInvalidQuery, err)
	}

	return &queryNode, nil
}

// BuildConditions walks the query tree and produces SQL condition strings,
// appending parameter values to params. The caller provides a FieldMapper
// so the tree walker doesn't need to know about signal-specific schema.
func BuildConditions(node *QueryNode, conditions *[]string, params *[]NamedParam, mapper FieldMapper) error {
	switch node.Type {
	case "condition":
		return buildCondition(node.Query, conditions, params, mapper)
	case "group":
		return buildGroup(node.Group, conditions, params, mapper)
	default:
		return fmt.Errorf("unknown node type %s: %w", node.Type, ErrInvalidQuery)
	}
}

func buildCondition(query *Query, conditions *[]string, params *[]NamedParam, mapper FieldMapper) error {
	if query == nil || query.Field == nil || query.FieldOperator == "" {
		return fmt.Errorf("invalid condition: missing field or operator: %w", ErrInvalidQuery)
	}

	field := query.Field

	dbExpressions, err := mapper(field, query, params)
	if err != nil {
		return fmt.Errorf("map field %s: %w", field.Name, err)
	}

	var sqlConditions []string
	for _, dbExpression := range dbExpressions {
		sqlCondition, err := BuildOperatorCondition(dbExpression, query, params)
		if err != nil {
			return fmt.Errorf("build operator condition: %w", err)
		}

		sqlConditions = append(sqlConditions, sqlCondition)
	}

	// A mapper may return several expressions for one condition. For a
	// global search they are alternatives -- the value may live in any of
	// those places -- so they join with OR. For a named field they are
	// requirements and join with AND. Appending them unjoined was the old
	// behaviour, and it was a trap: BuildSearchSQL later joined the top-level
	// list with a bare space, so the first named-field mapper to return two
	// expressions would have produced syntactically invalid SQL.
	if len(sqlConditions) > 1 {
		joiner := " AND "
		if field.SearchScope == "global" {
			joiner = " OR "
		}
		*conditions = append(*conditions, "("+strings.Join(sqlConditions, joiner)+")")
	} else {
		*conditions = append(*conditions, sqlConditions...)
	}

	return nil
}

func buildGroup(group *QueryGroup, conditions *[]string, params *[]NamedParam, mapper FieldMapper) error {
	if group == nil {
		return fmt.Errorf("invalid group: missing group data: %w", ErrInvalidQuery)
	}

	var childConditions []string

	for _, child := range group.Children {
		var childCondition []string

		err := BuildConditions(&child, &childCondition, params, mapper)
		if err != nil {
			return fmt.Errorf("BuildConditions: %w", err)
		}

		if len(childCondition) > 0 {
			childConditions = append(childConditions, childCondition...)
		}
	}

	if len(childConditions) == 0 {
		return nil
	}

	operator := strings.ToUpper(group.LogicalOperator)
	if operator != "AND" && operator != "OR" {
		return fmt.Errorf("invalid logical operator %s: %w", group.LogicalOperator, ErrInvalidQuery)
	}

	joinedConditions := strings.Join(childConditions, " "+operator+" ")
	*conditions = append(*conditions, "("+joinedConditions+")")

	return nil
}

// wireIDFields are field names whose values are trace/span IDs: served in
// OTLP wire form (dash-less lowercase hex) but stored in uuid columns.
// Signal mappers convert those columns to wire form for comparison, and
// values are normalized here to match, so dashed or uppercase input still
// works and malformed IDs match nothing instead of erroring on a uuid cast.
var wireIDFields = map[string]struct{}{
	"traceID":      {},
	"traceId":      {},
	"spanID":       {},
	"spanId":       {},
	"parentSpanID": {},
	"link.traceID": {},
	"link.spanID":  {},
}

func normalizeWireIDValue(value string) string {
	return strings.ToLower(strings.ReplaceAll(value, "-", ""))
}

// BuildOperatorCondition builds SQL condition for a specific operator.
func BuildOperatorCondition(expression string, query *Query, params *[]NamedParam) (string, error) {
	if query == nil {
		return "", fmt.Errorf("query cannot be nil: %w", ErrInvalidQuery)
	}

	operator := query.FieldOperator
	value := query.Value

	// A mapper that already produced a complete boolean says so, and nothing
	// further is appended to it.
	if rest, found := strings.CutPrefix(expression, PredicateToken); found {
		return rest, nil
	}

	const condToken = "{COND}"
	const rawToken = "{RAW}"
	hasPlaceholder := strings.Contains(expression, condToken)
	hasRaw := strings.Contains(expression, rawToken)
	var operatorString string

	if hasRaw {
		rawParamName := fmt.Sprintf("raw_%d", len(*params))
		*params = append(*params, NamedParam{rawParamName, value})
		expression = strings.ReplaceAll(expression, rawToken, rawParamName)
	}

	// IS NULL / IS NOT NULL arrive as explicit operators. They used to be
	// inferred from the sentinel value "NULL", which made the literal string
	// "NULL" unsearchable -- a quoted "NULL" in a query was indistinguishable
	// from the null check by the time it reached this function.
	if operator == "IS NULL" || operator == "IS NOT NULL" {
		if hasPlaceholder {
			return strings.ReplaceAll(expression, condToken, operator), nil
		}
		return expression + " " + operator, nil
	}

	if query.Field != nil && strings.HasSuffix(query.Field.Type, "[]") {
		return handleArrayOperator(expression, query, params)
	}

	// Normalized after the NULL check so `= NULL` keeps its IS NULL meaning.
	if query.Field != nil {
		if _, ok := wireIDFields[query.Field.Name]; ok {
			value = normalizeWireIDValue(value)
		}
	}

	paramName := fmt.Sprintf("value_%d", len(*params))

	// TODO: Query.Value is always a string because the frontend sends JSON and
	// the Go struct declares Value as string. For int64 fields (e.g. duration),
	// DuckDB needs an integer bind parameter — parse the string here as a
	// workaround until the wire format carries typed values.
	var bindValue any = value
	if query.Field != nil && query.Field.Type == "int64" {
		if n, err := strconv.ParseInt(value, 10, 64); err == nil {
			bindValue = n
		}
	}

	switch operator {
	case "=", "!=", ">", ">=", "<", "<=":
		*params = append(*params, NamedParam{paramName, bindValue})
		operatorString = operator + " " + paramName
	// DuckDB has no infix REGEXP -- `x REGEXP y` is a parser error, which
	// made every regex search fail from the day the operator shipped, and no
	// test executed one to notice. ~ and !~ are DuckDB's native full-match
	// regex operators; !~ on a NULL value yields NULL and excludes the row,
	// the same shape NOT LIKE gives NOT CONTAINS.
	case "REGEXP":
		*params = append(*params, NamedParam{paramName, bindValue})
		operatorString = "~ " + paramName
	case "NOT REGEXP":
		*params = append(*params, NamedParam{paramName, bindValue})
		operatorString = "!~ " + paramName
	case "CONTAINS":
		*params = append(*params, NamedParam{paramName, "%" + value + "%"})
		operatorString = "LIKE " + paramName
	case "NOT CONTAINS":
		*params = append(*params, NamedParam{paramName, "%" + value + "%"})
		operatorString = "NOT LIKE " + paramName
	case "^":
		*params = append(*params, NamedParam{paramName, value + "%"})
		operatorString = "LIKE " + paramName
	case "$":
		*params = append(*params, NamedParam{paramName, "%" + value})
		operatorString = "LIKE " + paramName
	case "IN", "NOT IN":
		values := ParseArrayValue(value)
		if len(values) == 0 {
			return "", fmt.Errorf("IN/NOT IN requires at least one value: %w", ErrInvalidQuery)
		}
		*params = append(*params, NamedParam{paramName, values})
		operatorString = operator + " " + paramName
	default:
		return "", fmt.Errorf("unsupported operator %s: %w", operator, ErrInvalidQuery)
	}

	if hasPlaceholder {
		return strings.ReplaceAll(expression, condToken, operatorString), nil
	}
	return expression + " " + operatorString, nil
}

func mapArrayTypeToDuckDB(frontendType string) (string, error) {
	switch frontendType {
	case "string[]":
		return "VARCHAR[]", nil
	case "int64[]":
		return "BIGINT[]", nil
	case "float64[]":
		return "DOUBLE[]", nil
	case "boolean[]":
		return "BOOLEAN[]", nil
	default:
		return "", fmt.Errorf("unsupported array type %s: %w", frontendType, ErrInvalidQuery)
	}
}

func handleArrayOperator(expression string, query *Query, params *[]NamedParam) (string, error) {
	operator := query.FieldOperator
	value := query.Value
	paramName := fmt.Sprintf("value_%d", len(*params))

	duckDBType, err := mapArrayTypeToDuckDB(query.Field.Type)
	if err != nil {
		return "", err
	}
	expression = fmt.Sprintf("CAST(%s AS %s)", expression, duckDBType)

	switch operator {
	case "=", "!=":
		if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
			*params = append(*params, NamedParam{paramName, ParseArrayValue(value)})
		} else {
			*params = append(*params, NamedParam{paramName, value})
		}
		return fmt.Sprintf("%s %s %s", expression, operator, paramName), nil

	case "CONTAINS":
		convertedValue := ConvertValueForArrayType(value, query.Field.Type)
		*params = append(*params, NamedParam{paramName, convertedValue})
		return fmt.Sprintf("list_contains(%s, %s)", expression, paramName), nil

	case "NOT CONTAINS":
		convertedValue := ConvertValueForArrayType(value, query.Field.Type)
		*params = append(*params, NamedParam{paramName, convertedValue})
		return fmt.Sprintf("NOT list_contains(%s, %s)", expression, paramName), nil

	case "IN":
		values := ParseArrayValue(value)
		if len(values) == 0 {
			return "", fmt.Errorf("IN requires at least one value: %w", ErrInvalidQuery)
		}
		convertedValues := make([]any, len(values))
		for i, val := range values {
			if strVal, ok := val.(string); ok {
				convertedValues[i] = ConvertValueForArrayType(strVal, query.Field.Type)
			} else {
				convertedValues[i] = val
			}
		}
		*params = append(*params, NamedParam{paramName, convertedValues})
		return fmt.Sprintf("list_has_all(%s, %s)", expression, paramName), nil

	case "NOT IN":
		values := ParseArrayValue(value)
		if len(values) == 0 {
			return "", fmt.Errorf("NOT IN requires at least one value: %w", ErrInvalidQuery)
		}
		convertedValues := make([]any, len(values))
		for i, val := range values {
			if strVal, ok := val.(string); ok {
				convertedValues[i] = ConvertValueForArrayType(strVal, query.Field.Type)
			} else {
				convertedValues[i] = val
			}
		}
		*params = append(*params, NamedParam{paramName, convertedValues})
		return fmt.Sprintf("NOT list_has_all(%s, %s)", expression, paramName), nil

	default:
		return "", fmt.Errorf("unsupported operator %s for array type: %w", operator, ErrInvalidQuery)
	}
}

// ParseArrayValue parses an array value off the wire. The parser sends JSON
// -- `["a,b","c"]` -- because a quoted element may contain commas, which the
// legacy "[a,b,c]" comma split corrupted into three wrong values. The split
// remains as the fallback for values that are not valid JSON arrays.
func ParseArrayValue(value string) []any {
	var decoded []string
	if err := json.Unmarshal([]byte(value), &decoded); err == nil {
		// nil for empty, matching the legacy path: callers branch on len.
		var result []any
		for _, v := range decoded {
			result = append(result, v)
		}
		return result
	}

	value = strings.Trim(value, "[]")
	parts := strings.Split(value, ",")
	var result []any

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}

// ConvertValueForArrayType converts a string value to the appropriate type for array operations
func ConvertValueForArrayType(value, arrayType string) any {
	switch arrayType {
	case "int64[]":
		if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intVal
		}
		return value
	case "float64[]":
		if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
			return floatVal
		}
		return value
	case "boolean[]":
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
		return value
	default:
		return value
	}
}

// TimePredicate builds the direct predicate for a nullable inclusive range.
// Only concrete endpoints become parameters and SQL conditions.
func TimePredicate(column string, startTime, endTime *int64) (string, []NamedParam) {
	var conditions []string
	var params []NamedParam
	if startTime != nil {
		conditions = append(conditions, column+" >= time_start")
		params = append(params, NamedParam{Name: "time_start", Value: *startTime})
	}
	if endTime != nil {
		conditions = append(conditions, column+" <= time_end")
		params = append(params, NamedParam{Name: "time_end", Value: *endTime})
	}
	return strings.Join(conditions, " AND "), params
}

// BuildSearchSQL builds the search_params CTE, WHERE clause, and args for any
// signal. timeCondition is empty when time is unbounded; timeParams contains
// only the concrete endpoints referenced by it.
func BuildSearchSQL(queryNode *QueryNode, mapper FieldMapper, timeCondition string, timeParams []NamedParam) (cteSQL, whereSQL string, args []any, err error) {
	params := append([]NamedParam(nil), timeParams...)

	var conditions []string
	if queryNode != nil {
		if err := BuildConditions(queryNode, &conditions, &params, mapper); err != nil {
			return "", "", nil, err
		}
	}

	if len(conditions) > 0 && timeCondition != "" {
		// One condition or one group reaches here as exactly one string --
		// buildCondition and buildGroup each append a single joined element --
		// so a multi-element list means a caller bug. Join defensively with
		// AND rather than the bare space this once was, which produced
		// syntactically invalid SQL the first time anything appended two.
		whereSQL = "(" + strings.Join(conditions, " AND ") + ") AND " + timeCondition
	} else if len(conditions) > 0 {
		whereSQL = "(" + strings.Join(conditions, " AND ") + ")"
	} else if timeCondition != "" {
		whereSQL = timeCondition
	} else {
		whereSQL = "true"
	}

	args = make([]any, len(params))
	cteParams := make([]string, len(params))
	for i, p := range params {
		args[i] = p.Value
		cteParams[i] = fmt.Sprintf("? as %s", p.Name)
	}
	if len(cteParams) == 0 {
		cteSQL = "with search_params as (select true as unbounded)"
	} else {
		cteSQL = fmt.Sprintf("with search_params as (select %s)", strings.Join(cteParams, ", "))
	}
	return cteSQL, whereSQL, args, nil
}
