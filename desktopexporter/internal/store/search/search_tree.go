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
type FieldMapper func(field *FieldDefinition, params *[]NamedParam) ([]string, error)

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

	dbExpressions, err := mapper(field, params)
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

	if field.SearchScope == "global" && len(sqlConditions) > 1 {
		joinedConditions := strings.Join(sqlConditions, " OR ")
		*conditions = append(*conditions, "("+joinedConditions+")")
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

// BuildOperatorCondition builds SQL condition for a specific operator.
func BuildOperatorCondition(expression string, query *Query, params *[]NamedParam) (string, error) {
	if query == nil {
		return "", fmt.Errorf("query cannot be nil: %w", ErrInvalidQuery)
	}

	operator := query.FieldOperator
	value := query.Value

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

	if value == "NULL" {
		switch operator {
		case "=":
			operatorString = "IS NULL"
		case "!=":
			operatorString = "IS NOT NULL"
		default:
			return "", fmt.Errorf("operator %s not supported with NULL value: %w", operator, ErrInvalidQuery)
		}

		result := expression
		if hasPlaceholder {
			return strings.ReplaceAll(result, condToken, operatorString), nil
		}
		return result + " " + operatorString, nil
	}

	if query.Field != nil && strings.HasSuffix(query.Field.Type, "[]") {
		return handleArrayOperator(expression, query, params)
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
	case "=", "!=", ">", ">=", "<", "<=", "REGEXP":
		*params = append(*params, NamedParam{paramName, bindValue})
		operatorString = operator + " " + paramName
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

// ParseArrayValue parses array values from frontend format "[value1,value2,value3]"
func ParseArrayValue(value string) []any {
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

// BuildSearchSQL builds the search_params CTE, WHERE clause, and args for any signal.
// timeCondition must reference time_start and time_end.
func BuildSearchSQL(queryNode *QueryNode, startTime, endTime int64, mapper FieldMapper, timeCondition string) (cteSQL, whereSQL string, args []any, err error) {
	params := []NamedParam{
		{Name: "time_start", Value: startTime},
		{Name: "time_end", Value: endTime},
	}

	var conditions []string
	if queryNode != nil {
		if err := BuildConditions(queryNode, &conditions, &params, mapper); err != nil {
			return "", "", nil, err
		}
	}

	if len(conditions) > 0 {
		whereSQL = "(" + strings.Join(conditions, " ") + ") AND " + timeCondition
	} else {
		whereSQL = timeCondition
	}

	args = make([]any, len(params))
	cteParams := make([]string, len(params))
	for i, p := range params {
		args[i] = p.Value
		cteParams[i] = fmt.Sprintf("? as %s", p.Name)
	}
	cteSQL = fmt.Sprintf("with search_params as (select %s)", strings.Join(cteParams, ", "))
	return cteSQL, whereSQL, args, nil
}
