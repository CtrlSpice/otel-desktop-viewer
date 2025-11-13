package store

import (
	"encoding/json"
	"fmt"
	"sort"
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

// BuildSQL converts QueryNode to SQL WHERE clause with time filtering
func BuildSQL(queryNode *QueryNode, signalType string, startTime, endTime int64) (string, string, []any, error) {

	var conditions []string
	namedArgs := make(map[string]any)

	// Always add time parameters to CTE
	namedArgs["time_start"] = startTime
	namedArgs["time_end"] = endTime

	// Add user query conditions if present
	if queryNode != nil {
		err := buildConditions(queryNode, &conditions, &namedArgs, signalType)
		if err != nil {
			return "", "", nil, err
		}
	}

	// Build WHERE clause with time conditions appended
	var whereSQL string
	if len(conditions) > 0 {
		whereSQL = "(" + strings.Join(conditions, " ") + ") AND StartTime >= time_start AND StartTime <= time_end"
	} else {
		whereSQL = "StartTime >= time_start AND StartTime <= time_end"
	}

	// Convert namedArgs to args slice for return
	args := make([]any, len(namedArgs))
	paramNames := make([]string, len(namedArgs))

	// Fill in time parameters first
	if timeStart, ok := namedArgs["time_start"]; ok {
		args[0] = timeStart
		paramNames[0] = "time_start"
	}
	if timeEnd, ok := namedArgs["time_end"]; ok {
		args[1] = timeEnd
		paramNames[1] = "time_end"
	}

	// Fill in user parameters (sort for deterministic order)
	userParamIndex := 2
	var userParamNames []string
	for paramName := range namedArgs {
		if paramName != "time_start" && paramName != "time_end" {
			userParamNames = append(userParamNames, paramName)
		}
	}
	sort.Strings(userParamNames)
	for _, paramName := range userParamNames {
		args[userParamIndex] = namedArgs[paramName]
		paramNames[userParamIndex] = paramName
		userParamIndex++
	}

	// Build CTE using the ordered parameter names
	var cteParams []string
	for _, paramName := range paramNames {
		cteParams = append(cteParams, fmt.Sprintf("? as %s", paramName))
	}
	cteSQL := fmt.Sprintf("WITH search_params AS (SELECT %s)", strings.Join(cteParams, ", "))

	return cteSQL, whereSQL, args, nil
}

// buildConditions recursively builds SQL conditions from QueryNode
func buildConditions(node *QueryNode, conditions *[]string, namedArgs *map[string]any, signalType string) error {
	switch node.Type {
	case "condition":
		return buildCondition(node.Query, conditions, namedArgs, signalType)
	case "group":
		return buildGroup(node.Group, conditions, namedArgs, signalType)
	default:
		return fmt.Errorf("unknown node type: %s", node.Type)
	}
}

// buildCondition builds SQL for a single condition
func buildCondition(query *Query, conditions *[]string, namedArgs *map[string]any, signalType string) error {
	if query == nil || query.Field == nil || query.FieldOperator == "" {
		return fmt.Errorf("invalid condition: missing field or operator")
	}

	field := query.Field

	// Map field to database expressions
	dbExpressions, err := mapFieldToExpressions(field, signalType)
	if err != nil {
		return fmt.Errorf("failed to map field %s: %w", field.Name, err)
	}

	// Build SQL condition for each expression
	var sqlConditions []string
	for _, dbExpression := range dbExpressions {
		var sqlCondition string
		var err error

		// For attribute queries, build union_extract expression first, then apply operator
		// For regular field expressions, use the expression directly
		expression := dbExpression
		if field.SearchScope == "attribute" {
			// This should never happen, but just in case
			if field.Type == "" {
				return fmt.Errorf("attribute field %s missing type", field.Name)
			}

			// Map frontend type format to DuckDB union tag format
			duckDBType := field.Type
			if strings.HasSuffix(field.Type, "[]") {
				duckDBType = strings.TrimSuffix(field.Type, "[]") + "_list"
			}

			// Build union_extract expression - buildOperatorCondition will handle array operators
			expression = fmt.Sprintf("union_extract(%s, '%s')", dbExpression, duckDBType)
		}

		// Build operator condition - this will handle arrays if the type is an array
		sqlCondition, err = buildOperatorCondition(expression, query, namedArgs)
		if err != nil {
			return fmt.Errorf("failed to build operator condition: %w", err)
		}

		// Post-process event and link attributes to wrap in EXISTS to search attributes inside arrays
		if field.SearchScope == "attribute" && (field.AttributeScope == "event" || field.AttributeScope == "link") {
			sqlCondition = wrapWithExists(sqlCondition, field.AttributeScope)
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
func buildGroup(group *QueryGroup, conditions *[]string, namedArgs *map[string]any, signalType string) error {
	if group == nil {
		return fmt.Errorf("invalid group: missing group data")
	}

	var childConditions []string

	for _, child := range group.Children {
		var childCondition []string

		err := buildConditions(&child, &childCondition, namedArgs, signalType)
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

	// Wrap in parentheses and join with operator
	operator := strings.ToUpper(group.LogicalOperator)
	if operator != "AND" && operator != "OR" {
		return fmt.Errorf("invalid logical operator: %s", group.LogicalOperator)
	}

	joinedConditions := strings.Join(childConditions, " "+operator+" ")
	*conditions = append(*conditions, "("+joinedConditions+")")

	return nil
}

// buildCTE generates the Common Table Expression for parameter aliasing

// buildOperatorCondition builds SQL condition for specific operator
func buildOperatorCondition(expression string, query *Query, namedArgs *map[string]any) (string, error) {
	if query == nil {
		return "", fmt.Errorf("query cannot be nil")
	}

	operator := query.FieldOperator
	value := query.Value

	// Check if expression contains a placeholder that needs to be replaced
	hasPlaceholder := strings.Contains(expression, "?")
	var operatorString string

	// Handle null values with standard equality operators
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

	// Handle array types - check if field type is an array
	if query.Field != nil && strings.HasSuffix(query.Field.Type, "[]") {
		return handleArrayOperatorInBuildOperator(expression, query, namedArgs)
	}

	// Generate parameter name and populate namedArgs
	paramName := fmt.Sprintf("value_%d", len(*namedArgs)-2)

	// Build operator part based on operator type
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
		values := parseArrayValue(value)
		if len(values) == 0 {
			return "", fmt.Errorf("IN/NOT IN requires at least one value")
		}
		(*namedArgs)[paramName] = values
		operatorString = operator + " " + paramName
	default:
		return "", fmt.Errorf("unsupported operator: %s", operator)
	}

	// Apply operator part to expression
	if hasPlaceholder {
		// Replace all "= ?" with operator part (needed for OR conditions with multiple placeholders)
		return strings.ReplaceAll(expression, "= ?", operatorString), nil
	}
	// No placeholder, append operator part
	return expression + " " + operatorString, nil
}

// handleArrayOperatorInBuildOperator handles array operators within buildOperatorCondition
func handleArrayOperatorInBuildOperator(expression string, query *Query, namedArgs *map[string]any) (string, error) {
	operator := query.FieldOperator
	value := query.Value

	// Generate parameter name
	paramName := fmt.Sprintf("value_%d", len(*namedArgs)-2)

	switch operator {
	case "=", "!=":
		// Direct array equality comparison - DuckDB supports this natively
		// If value looks like an array, parse it
		if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
			(*namedArgs)[paramName] = parseArrayValue(value)
		} else {
			(*namedArgs)[paramName] = value
		}
		return fmt.Sprintf("%s %s %s", expression, operator, paramName), nil

	case "CONTAINS":
		// Check if array contains a single value
		// Convert value to appropriate type based on array element type
		convertedValue := convertValueForArrayType(value, query.Field.Type)
		(*namedArgs)[paramName] = convertedValue
		return fmt.Sprintf("list_contains(%s, %s)", expression, paramName), nil

	case "NOT CONTAINS":
		// Check if array does not contain a single value
		// Convert value to appropriate type based on array element type
		convertedValue := convertValueForArrayType(value, query.Field.Type)
		(*namedArgs)[paramName] = convertedValue
		return fmt.Sprintf("NOT list_contains(%s, %s)", expression, paramName), nil

	case "IN":
		// Check if array contains all of the values in the set using list_has_all
		values := parseArrayValue(value)
		if len(values) == 0 {
			return "", fmt.Errorf("IN requires at least one value")
		}
		// Convert all values to appropriate type based on array element type
		convertedValues := make([]any, len(values))
		for i, val := range values {
			if strVal, ok := val.(string); ok {
				convertedValues[i] = convertValueForArrayType(strVal, query.Field.Type)
			} else {
				convertedValues[i] = val
			}
		}
		(*namedArgs)[paramName] = convertedValues
		return fmt.Sprintf("list_has_all(%s, %s)", expression, paramName), nil

	case "NOT IN":
		// Check if array does not contain all of the values in the set using list_has_all
		values := parseArrayValue(value)
		if len(values) == 0 {
			return "", fmt.Errorf("NOT IN requires at least one value")
		}
		// Convert all values to appropriate type based on array element type
		convertedValues := make([]any, len(values))
		for i, val := range values {
			if strVal, ok := val.(string); ok {
				convertedValues[i] = convertValueForArrayType(strVal, query.Field.Type)
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
		return mapGlobalExpressions(signalType)
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
	switch signalType {
	case "traces":
		return mapTraceAttributeExpressions(field)
	case "logs":
		return nil, fmt.Errorf("logs attribute search not implemented yet")
	case "metrics":
		return nil, fmt.Errorf("metrics attribute search not implemented yet")
	default:
		return nil, fmt.Errorf("unknown signal type: %s", signalType)
	}
}

// mapTraceAttributeExpressions maps trace attributes to SQL expressions
func mapTraceAttributeExpressions(field *FieldDefinition) ([]string, error) {
	switch field.AttributeScope {
	case "resource":
		// Resource attributes: ResourceAttributes['attribute_name']
		return []string{fmt.Sprintf("ResourceAttributes['%s']", field.Name)}, nil
	case "scope":
		// Scope attributes: ScopeAttributes['attribute_name']
		return []string{fmt.Sprintf("ScopeAttributes['%s']", field.Name)}, nil
	case "span":
		// Span attributes: Attributes['attribute_name']
		return []string{fmt.Sprintf("Attributes['%s']", field.Name)}, nil
	case "event":
		// Event attributes: unnest.Attributes['attribute_name']
		return []string{fmt.Sprintf("unnest.Attributes['%s']", field.Name)}, nil
	case "link":
		// Link attributes: unnest.Attributes['attribute_name']
		return []string{fmt.Sprintf("unnest.Attributes['%s']", field.Name)}, nil
	default:
		return nil, fmt.Errorf("unknown attribute scope: %s", field.AttributeScope)
	}
}

// mapGlobalExpressions handles global scope mapping
func mapGlobalExpressions(signalType string) ([]string, error) {
	var searchFields []string

	// Resource and scope attributes (apply to all signal types)
	searchFields = append(searchFields,
		"EXISTS(SELECT 1 FROM UNNEST(map_entries(s.ResourceAttributes)) WHERE unnest.key = ? OR CAST(unnest.value AS VARCHAR) = ?)",
		"EXISTS(SELECT 1 FROM UNNEST(map_entries(s.ScopeAttributes)) WHERE unnest.key = ? OR CAST(unnest.value AS VARCHAR) = ?)",
	)

	switch signalType {
	case "traces":
		searchFields = append(searchFields,
			"s.TraceID",
			"s.TraceState",
			"s.SpanID",
			"s.ParentSpanID",
			"s.Name",
			"s.Kind",
			"CAST(s.StartTime AS VARCHAR)",
			"CAST(s.EndTime AS VARCHAR)",
			"CAST(s.ResourceDroppedAttributesCount AS VARCHAR)",
			"s.ScopeName",
			"s.ScopeVersion",
			"CAST(s.ScopeDroppedAttributesCount AS VARCHAR)",
			"CAST(s.DroppedAttributesCount AS VARCHAR)",
			"CAST(s.DroppedEventsCount AS VARCHAR)",
			"CAST(s.DroppedLinksCount AS VARCHAR)",
			"s.StatusCode",
			"s.StatusMessage",
			"EXISTS(SELECT 1 FROM UNNEST(s.Events) WHERE unnest.Name = ?)",
			"EXISTS(SELECT 1 FROM UNNEST(s.Events) WHERE CAST(unnest.Timestamp AS VARCHAR) = ?)",
			"EXISTS(SELECT 1 FROM UNNEST(s.Events) WHERE CAST(unnest.DroppedAttributesCount AS VARCHAR) = ?)",
			"EXISTS(SELECT 1 FROM UNNEST(s.Links) WHERE unnest.TraceID = ?)",
			"EXISTS(SELECT 1 FROM UNNEST(s.Links) WHERE unnest.SpanID = ?)",
			"EXISTS(SELECT 1 FROM UNNEST(s.Links) WHERE unnest.TraceState = ?)",
			"EXISTS(SELECT 1 FROM UNNEST(s.Links) WHERE CAST(unnest.DroppedAttributesCount AS VARCHAR) = ?)",
			// Span attributes (keys and values)
			"EXISTS(SELECT 1 FROM UNNEST(map_entries(s.Attributes)) WHERE unnest.key = ? OR CAST(unnest.value AS VARCHAR) = ?)",
			// Event and link attributes (keys and values) - using derived table to avoid double UNNEST
			"EXISTS(SELECT 1 FROM (SELECT UNNEST(s.Events) AS event_data) WHERE EXISTS(SELECT 1 FROM UNNEST(map_entries(event_data.Attributes)) WHERE unnest.key = ? OR CAST(unnest.value AS VARCHAR) = ?))",
			"EXISTS(SELECT 1 FROM (SELECT UNNEST(s.Links) AS link_data) WHERE EXISTS(SELECT 1 FROM UNNEST(map_entries(link_data.Attributes)) WHERE unnest.key = ? OR CAST(unnest.value AS VARCHAR) = ?))",
		)

	case "logs":
		searchFields = append(searchFields, "LogID", "Timestamp", "ObservedTimestamp", "TraceID", "SpanID", "SeverityText", "SeverityNumber", "Body")
	case "metrics":
		searchFields = append(searchFields, "MetricID", "Name", "Description", "Unit")
	default:
		return nil, fmt.Errorf("unknown signal type: %s", signalType)
	}

	return searchFields, nil
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

// convertValueForArrayType converts a string value to the appropriate type for array operations
func convertValueForArrayType(value, arrayType string) any {
	switch arrayType {
	case "int64[]":
		// Try to parse as int64
		if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intVal
		}
		// If parsing fails, return as string (will cause error but that's better than silent failure)
		return value
	case "float64[]":
		// Try to parse as float64
		if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
			return floatVal
		}
		// If parsing fails, return as string (will cause error but that's better than silent failure)
		return value
	case "boolean[]":
		// Try to parse as boolean
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
		// If parsing fails, return as string (will cause error but that's better than silent failure)
		return value
	default:
		// For string[] and unknown types, return as string
		return value
	}
}

// wrapWithExists wraps event and link attribute expressions in EXISTS clauses
func wrapWithExists(sqlCondition, attributeScope string) string {
	switch attributeScope {
	case "event":
		return fmt.Sprintf("EXISTS(SELECT 1 FROM UNNEST(Events) WHERE %s)", sqlCondition)
	case "link":
		return fmt.Sprintf("EXISTS(SELECT 1 FROM UNNEST(Links) WHERE %s)", sqlCondition)
	default:
		return sqlCondition
	}
}
