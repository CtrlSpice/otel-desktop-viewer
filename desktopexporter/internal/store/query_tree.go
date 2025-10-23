package store

import (
	"encoding/json"
	"fmt"
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
	Name           string `json:"name"`
	SearchScope    string `json:"searchScope"`
	AttributeScope string `json:"attributeScope,omitempty"`
}

type QueryGroup struct {
	LogicalOperator string      `json:"logicalOperator"` // "AND" or "OR"
	Children        []QueryNode `json:"children"`
}

// ParseQueryTree converts JSON from frontend to QueryNode struct
func ParseQueryTree(jsonData any) (*QueryNode, error) {
	// Convert to JSON bytes first
	jsonBytes, err := json.Marshal(jsonData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query data: %w", err)
	}

	// Parse into QueryNode
	var queryNode QueryNode
	if err := json.Unmarshal(jsonBytes, &queryNode); err != nil {
		return nil, fmt.Errorf("failed to parse query tree: %w", err)
	}

	return &queryNode, nil
}

// BuildSQLWhereClause converts QueryNode to SQL WHERE clause
func BuildSQLWhereClause(queryNode *QueryNode, signalType string) (string, []any, error) {
	if queryNode == nil {
		return "", nil, nil
	}

	var conditions []string
	var args []any

	err := buildConditions(queryNode, &conditions, &args, signalType)
	if err != nil {
		return "", nil, err
	}

	if len(conditions) == 0 {
		return "", nil, nil
	}

	return strings.Join(conditions, " "), args, nil
}

// buildConditions recursively builds SQL conditions from QueryNode
func buildConditions(node *QueryNode, conditions *[]string, args *[]any, signalType string) error {
	switch node.Type {
	case "condition":
		return buildCondition(node.Query, conditions, args, signalType)
	case "group":
		return buildGroup(node.Group, conditions, args, signalType)
	default:
		return fmt.Errorf("unknown node type: %s", node.Type)
	}
}

// buildCondition builds SQL for a single condition
func buildCondition(query *Query, conditions *[]string, args *[]any, signalType string) error {
	if query == nil || query.Field == nil || query.FieldOperator == "" {
		return fmt.Errorf("invalid condition: missing field or operator")
	}

	field := query.Field
	operator := query.FieldOperator
	value := query.Value

	// Map field to database expressions
	dbExpressions, err := mapFieldToExpressions(field, signalType)
	if err != nil {
		return fmt.Errorf("failed to map field %s: %w", field.Name, err)
	}

	// Build SQL condition for each expression
	for _, dbExpression := range dbExpressions {
		var sqlCondition string
		var sqlArgs []any
		var err error

		// Check if this is an EXISTS expression (array field access)
		if strings.HasPrefix(dbExpression, "EXISTS(") {
			// For EXISTS expressions, we don't need additional operators
			// The EXISTS already returns a boolean
			sqlCondition = dbExpression
			sqlArgs = []any{value}
		} else {
			// For regular field expressions, use the normal operator logic
			sqlCondition, sqlArgs, err = buildOperatorCondition(dbExpression, operator, value)
			if err != nil {
				return fmt.Errorf("failed to build operator condition: %w", err)
			}
		}

		*conditions = append(*conditions, sqlCondition)
		*args = append(*args, sqlArgs...)
	}

	return nil
}

// buildGroup builds SQL for a logical group (AND/OR)
func buildGroup(group *QueryGroup, conditions *[]string, args *[]any, signalType string) error {
	if group == nil {
		return fmt.Errorf("invalid group: missing group data")
	}

	var childConditions []string
	var childArgs []any

	for _, child := range group.Children {
		var childCondition []string
		var childArg []any

		err := buildConditions(&child, &childCondition, &childArg, signalType)
		if err != nil {
			return err
		}

		if len(childCondition) > 0 {
			childConditions = append(childConditions, childCondition...)
			childArgs = append(childArgs, childArg...)
		}
	}

	if len(childConditions) == 0 {
		return nil
	}

	// Wrap in parentheses and join with operator
	operator := strings.ToUpper(group.LogicalOperator)
	if operator != "AND" && operator != "OR" {
		return fmt.Errorf("invalid logical operator: %s", group.LogicalOperator)
	}

	joinedConditions := strings.Join(childConditions, " "+operator+" ")
	*conditions = append(*conditions, "("+joinedConditions+")")
	*args = append(*args, childArgs...)

	return nil
}

// buildOperatorCondition builds SQL condition for specific operator
func buildOperatorCondition(expression, operator, value string) (string, []any, error) {
	// Handle null values with standard equality operators
	if value == "NULL" {
		switch operator {
		case "=":
			return expression + " IS NULL", []any{}, nil
		case "!=":
			return expression + " IS NOT NULL", []any{}, nil
		default:
			return "", nil, fmt.Errorf("operator %s not supported with NULL value", operator)
		}
	}

	switch operator {
	case "=", "!=", ">", ">=", "<", "<=", "REGEXP":
		return expression + " " + operator + " ?", []any{value}, nil
	case "CONTAINS", "NOT CONTAINS", "^", "$":
		// Map user-friendly operators to SQL LIKE patterns
		var sqlOperator, likeValue string
		switch operator {
		case "CONTAINS":
			sqlOperator = "LIKE"
			likeValue = "%" + value + "%"
		case "NOT CONTAINS":
			sqlOperator = "NOT LIKE"
			likeValue = "%" + value + "%"
		case "^":
			sqlOperator = "LIKE"
			likeValue = value + "%"
		case "$":
			sqlOperator = "LIKE"
			likeValue = "%" + value
		}

		return expression + " " + sqlOperator + " ?", []any{likeValue}, nil
	case "IN", "NOT IN":
		values := parseArrayValue(value)
		placeholders := strings.Repeat("?,", len(values))
		placeholders = placeholders[:len(placeholders)-1] // Remove trailing comma
		return expression + " " + operator + " (" + placeholders + ")", values, nil
	default:
		return "", nil, fmt.Errorf("unsupported operator: %s", operator)
	}
}

// mapFieldToExpressions maps frontend field to database expressions
func mapFieldToExpressions(field *FieldDefinition, signalType string) ([]string, error) {
	switch field.SearchScope {
	case "field":
		expression, err := mapFieldExpressions(field, signalType)
		if err != nil {
			return nil, err
		}
		return []string{expression}, nil
	case "attribute":
		return mapAttributeExpressions(field, signalType)
	case "global":
		return mapGlobalExpressions(field, signalType)
	default:
		return nil, fmt.Errorf("unknown search scope: %s", field.SearchScope)
	}
}

// mapFieldExpressions handles field scope mapping
func mapFieldExpressions(field *FieldDefinition, signalType string) (string, error) {
	// Handle common fields first (resource.* and scope.*) - applies to all signal types
	if expression, isCommon := mapCommonFields(field.Name); isCommon {
		return expression, nil
	}

	switch signalType {
	case "traces":
		// Check for event fields: event.name -> Events[].Name
		if expression, isArray := mapArrayField(field.Name, "event", "Events"); isArray {
			return expression, nil
		}

		// Check for link fields: link.traceID -> Links[].TraceID
		if expression, isArray := mapArrayField(field.Name, "link", "Links"); isArray {
			return expression, nil
		}

		// Default: capitalize first letter
		if len(field.Name) > 0 {
			capitalized := strings.ToUpper(field.Name[:1]) + field.Name[1:]
			return capitalized, nil
		}
		return field.Name, nil

	case "logs":
		// TODO: Implement logs field mapping
		return "", fmt.Errorf("logs search not implemented yet")
	case "metrics":
		// TODO: Implement metrics field mapping
		return "", fmt.Errorf("metrics search not implemented yet")
	default:
		return "", fmt.Errorf("unknown signal type: %s", signalType)
	}
}

// mapAttributeExpressions handles attribute scope mapping
func mapAttributeExpressions(field *FieldDefinition, signalType string) ([]string, error) {
	// TODO: Implement attribute mapping
	return nil, fmt.Errorf("attribute search not implemented yet")
}

// mapGlobalExpressions handles global scope mapping
func mapGlobalExpressions(field *FieldDefinition, signalType string) ([]string, error) {
	// TODO: Implement global search
	return nil, fmt.Errorf("global search not implemented yet")
}

// mapCommonFields handles resource and scope field mapping
func mapCommonFields(fieldName string) (string, bool) {
	// Handle resource fields: resource.droppedAttributesCount -> ResourceDroppedAttributesCount
	if resourceField, found := strings.CutPrefix(fieldName, "resource."); found {
		capitalized := strings.ToUpper(resourceField[:1]) + resourceField[1:]
		return "Resource" + capitalized, true
	}

	// Handle scope fields: scope.name -> ScopeName, scope.version -> ScopeVersion, etc.
	if scopeField, found := strings.CutPrefix(fieldName, "scope."); found {
		capitalized := strings.ToUpper(scopeField[:1]) + scopeField[1:]
		return "Scope" + capitalized, true
	}

	return "", false
}

// mapArrayField handles nested field mapping with a given prefix and array name
func mapArrayField(fieldName, prefix, arrayName string) (string, bool) {
	if nestedField, found := strings.CutPrefix(fieldName, prefix+"."); found {
		// Extract the nested field name after the prefix
		capitalized := strings.ToUpper(nestedField[:1]) + nestedField[1:]
		// Use UNNEST to access array elements in DuckDB
		return fmt.Sprintf("EXISTS(SELECT 1 FROM UNNEST(%s) AS item WHERE item.'%s' = ?)", arrayName, capitalized), true
	}

	return "", false
}

// parseArrayValue parses array values from frontend format "[value1,value2,value3]"
func parseArrayValue(value string) []any {
	// Remove brackets
	value = strings.Trim(value, "[]")

	// Split by comma and trim whitespace
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
