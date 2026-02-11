package store

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// QueryNode represents a parsed query tree from the frontend
type QueryNode struct {
	ID    string      `json:"id"`
	Type  string      `json:"type"` // "condition" or "group"
	Query *Query      `json:"query,omitempty"`
	Group *QueryGroup `json:"group,omitempty"`
}

type Query struct {
	Field         *FieldDefinition `json:"field"`
	FieldOperator string           `json:"fieldOperator"`
	Value         string           `json:"value"`
}

type FieldDefinition struct {
	Name           string `json:"name,omitempty"`
	SearchScope    string `json:"searchScope"`
	AttributeScope string `json:"attributeScope,omitempty"`
	Type           string `json:"type,omitempty"`
}

type QueryGroup struct {
	LogicalOperator string      `json:"logicalOperator"` // "AND" or "OR"
	Children        []QueryNode `json:"children"`
}

// FieldMapper maps a FieldDefinition to one or more SQL expressions.
// Signal-specific code provides this to the generic tree walker.
type FieldMapper func(field *FieldDefinition) ([]string, error)

// ParseQueryTree converts JSON from frontend to QueryNode struct
func ParseQueryTree(jsonData any) (*QueryNode, error) {
	jsonBytes, err := json.Marshal(jsonData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query data: %w", err)
	}

	var queryNode QueryNode
	if err := json.Unmarshal(jsonBytes, &queryNode); err != nil {
		return nil, fmt.Errorf("failed to parse query tree: %w", err)
	}

	return &queryNode, nil
}

// BuildConditions walks the query tree and produces SQL condition strings,
// populating namedArgs with parameter values. The caller provides a FieldMapper
// so the tree walker doesn't need to know about signal-specific schema.
func BuildConditions(node *QueryNode, conditions *[]string, namedArgs *map[string]any, mapper FieldMapper) error {
	switch node.Type {
	case "condition":
		return buildCondition(node.Query, conditions, namedArgs, mapper)
	case "group":
		return buildGroup(node.Group, conditions, namedArgs, mapper)
	default:
		return fmt.Errorf("unknown node type: %s", node.Type)
	}
}

// buildCondition builds SQL for a single condition
func buildCondition(query *Query, conditions *[]string, namedArgs *map[string]any, mapper FieldMapper) error {
	if query == nil || query.Field == nil || query.FieldOperator == "" {
		return fmt.Errorf("invalid condition: missing field or operator")
	}

	field := query.Field

	// Map field to database expressions using the signal-specific mapper
	dbExpressions, err := mapper(field)
	if err != nil {
		return fmt.Errorf("failed to map field %s: %w", field.Name, err)
	}

	// Build SQL condition for each expression
	var sqlConditions []string
	for _, dbExpression := range dbExpressions {
		expression := dbExpression
		if field.SearchScope == "attribute" {
			if field.Type == "" {
				return fmt.Errorf("attribute field %s missing type", field.Name)
			}

			// Map frontend type format to DuckDB union tag format
			duckDBType := field.Type
			if strings.HasSuffix(field.Type, "[]") {
				duckDBType = strings.TrimSuffix(field.Type, "[]") + "_list"
			}

			expression = fmt.Sprintf("union_extract(%s, '%s')", dbExpression, duckDBType)
		}

		sqlCondition, err := BuildOperatorCondition(expression, query, namedArgs)
		if err != nil {
			return fmt.Errorf("failed to build operator condition: %w", err)
		}

		sqlConditions = append(sqlConditions, sqlCondition)
	}

	// For global search, OR all expressions together
	if field.SearchScope == "global" && len(sqlConditions) > 1 {
		joinedConditions := strings.Join(sqlConditions, " OR ")
		*conditions = append(*conditions, "("+joinedConditions+")")
	} else {
		*conditions = append(*conditions, sqlConditions...)
	}

	return nil
}

// buildGroup builds SQL for a logical group (AND/OR)
func buildGroup(group *QueryGroup, conditions *[]string, namedArgs *map[string]any, mapper FieldMapper) error {
	if group == nil {
		return fmt.Errorf("invalid group: missing group data")
	}

	var childConditions []string

	for _, child := range group.Children {
		var childCondition []string

		err := BuildConditions(&child, &childCondition, namedArgs, mapper)
		if err != nil {
			return err
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
		return fmt.Errorf("invalid logical operator: %s", group.LogicalOperator)
	}

	joinedConditions := strings.Join(childConditions, " "+operator+" ")
	*conditions = append(*conditions, "("+joinedConditions+")")

	return nil
}

// BuildOperatorCondition builds SQL condition for a specific operator.
func BuildOperatorCondition(expression string, query *Query, namedArgs *map[string]any) (string, error) {
	if query == nil {
		return "", fmt.Errorf("query cannot be nil")
	}

	operator := query.FieldOperator
	value := query.Value

	hasPlaceholder := strings.Contains(expression, "?")
	var operatorString string

	// Handle null values
	if value == "NULL" {
		switch operator {
		case "=":
			operatorString = "IS NULL"
		case "!=":
			operatorString = "IS NOT NULL"
		default:
			return "", fmt.Errorf("operator %s not supported with NULL value", operator)
		}

		if hasPlaceholder {
			return strings.ReplaceAll(expression, "= ?", operatorString), nil
		}
		return expression + " " + operatorString, nil
	}

	// Handle array types
	if query.Field != nil && strings.HasSuffix(query.Field.Type, "[]") {
		return handleArrayOperator(expression, query, namedArgs)
	}

	// Generate parameter name
	paramName := fmt.Sprintf("value_%d", len(*namedArgs)-2)

	switch operator {
	case "=", "!=", ">", ">=", "<", "<=", "REGEXP":
		(*namedArgs)[paramName] = value
		operatorString = operator + " " + paramName
	case "CONTAINS":
		(*namedArgs)[paramName] = "%" + value + "%"
		operatorString = "LIKE " + paramName
	case "NOT CONTAINS":
		(*namedArgs)[paramName] = "%" + value + "%"
		operatorString = "NOT LIKE " + paramName
	case "^":
		(*namedArgs)[paramName] = value + "%"
		operatorString = "LIKE " + paramName
	case "$":
		(*namedArgs)[paramName] = "%" + value
		operatorString = "LIKE " + paramName
	case "IN", "NOT IN":
		values := ParseArrayValue(value)
		if len(values) == 0 {
			return "", fmt.Errorf("IN/NOT IN requires at least one value")
		}
		(*namedArgs)[paramName] = values
		operatorString = operator + " " + paramName
	default:
		return "", fmt.Errorf("unsupported operator: %s", operator)
	}

	if hasPlaceholder {
		return strings.ReplaceAll(expression, "= ?", operatorString), nil
	}
	return expression + " " + operatorString, nil
}

// handleArrayOperator handles array operators within BuildOperatorCondition
func handleArrayOperator(expression string, query *Query, namedArgs *map[string]any) (string, error) {
	operator := query.FieldOperator
	value := query.Value
	paramName := fmt.Sprintf("value_%d", len(*namedArgs)-2)

	switch operator {
	case "=", "!=":
		if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
			(*namedArgs)[paramName] = ParseArrayValue(value)
		} else {
			(*namedArgs)[paramName] = value
		}
		return fmt.Sprintf("%s %s %s", expression, operator, paramName), nil

	case "CONTAINS":
		convertedValue := ConvertValueForArrayType(value, query.Field.Type)
		(*namedArgs)[paramName] = convertedValue
		return fmt.Sprintf("list_contains(%s, %s)", expression, paramName), nil

	case "NOT CONTAINS":
		convertedValue := ConvertValueForArrayType(value, query.Field.Type)
		(*namedArgs)[paramName] = convertedValue
		return fmt.Sprintf("NOT list_contains(%s, %s)", expression, paramName), nil

	case "IN":
		values := ParseArrayValue(value)
		if len(values) == 0 {
			return "", fmt.Errorf("IN requires at least one value")
		}
		convertedValues := make([]any, len(values))
		for i, val := range values {
			if strVal, ok := val.(string); ok {
				convertedValues[i] = ConvertValueForArrayType(strVal, query.Field.Type)
			} else {
				convertedValues[i] = val
			}
		}
		(*namedArgs)[paramName] = convertedValues
		return fmt.Sprintf("list_has_all(%s, %s)", expression, paramName), nil

	case "NOT IN":
		values := ParseArrayValue(value)
		if len(values) == 0 {
			return "", fmt.Errorf("NOT IN requires at least one value")
		}
		convertedValues := make([]any, len(values))
		for i, val := range values {
			if strVal, ok := val.(string); ok {
				convertedValues[i] = ConvertValueForArrayType(strVal, query.Field.Type)
			} else {
				convertedValues[i] = val
			}
		}
		(*namedArgs)[paramName] = convertedValues
		return fmt.Sprintf("NOT list_has_all(%s, %s)", expression, paramName), nil

	default:
		return "", fmt.Errorf("unsupported operator %s for array type", operator)
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
