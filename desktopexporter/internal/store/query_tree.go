package store

import (
	"encoding/json"
	"fmt"
	"sort"
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
	operator := query.FieldOperator
	value := query.Value

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

		// For regular field expressions, use the normal operator logic
		sqlCondition, err = buildOperatorCondition(dbExpression, operator, value, namedArgs)
		if err != nil {
			return fmt.Errorf("failed to build operator condition: %w", err)
		}

		// Process attribute queries to add type casting for proper Union type access
		if field.SearchScope == "attribute" {
			sqlCondition = wrapWithTypeCasting(dbExpression, sqlCondition)
			// Post-process event and link attributes to wrap in EXISTS to search attributes inside arrays
			if field.AttributeScope == "event" || field.AttributeScope == "link" {
				sqlCondition = wrapWithExists(sqlCondition, field.AttributeScope)
			}
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
func buildOperatorCondition(expression, operator, value string, namedArgs *map[string]any) (string, error) {
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
		// Event attributes: event.Attributes['attribute_name']
		return []string{fmt.Sprintf("event.Attributes['%s']", field.Name)}, nil
	case "link":
		// Link attributes: link.Attributes['attribute_name']
		return []string{fmt.Sprintf("link.Attributes['%s']", field.Name)}, nil
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

// wrapWithTypeCasting wraps attribute expressions with type casting
func wrapWithTypeCasting(expression string, condition string) string {
	// Extract the operator part from the condition
	operatorPart, found := strings.CutPrefix(condition, expression)
	if !found {
		// Fallback if prefix doesn't match
		operatorPart = " = ?"
	}

	return fmt.Sprintf(`CASE 
		WHEN union_tag(%s) = 'string' THEN %s::VARCHAR%s
		WHEN union_tag(%s) = 'int64' THEN %s::BIGINT%s
		WHEN union_tag(%s) = 'float64' THEN %s::DOUBLE%s
		WHEN union_tag(%s) = 'boolean' THEN %s::BOOLEAN%s
		WHEN union_tag(%s) = 'string_list' THEN %s::VARCHAR[]%s
		WHEN union_tag(%s) = 'int64_list' THEN %s::BIGINT[]%s
		WHEN union_tag(%s) = 'float64_list' THEN %s::DOUBLE[]%s
		WHEN union_tag(%s) = 'boolean_list' THEN %s::BOOLEAN[]%s
		ELSE FALSE
	END`, expression, expression, operatorPart, expression, expression, operatorPart,
		expression, expression, operatorPart, expression, expression, operatorPart,
		expression, expression, operatorPart, expression, expression, operatorPart,
		expression, expression, operatorPart, expression, expression, operatorPart)
}

// wrapWithExists wraps event and link attribute expressions in EXISTS clauses
func wrapWithExists(sqlCondition, attributeScope string) string {
	switch attributeScope {
	case "event":
		return fmt.Sprintf("EXISTS(SELECT 1 FROM UNNEST(Events) AS event WHERE %s)", sqlCondition)
	case "link":
		return fmt.Sprintf("EXISTS(SELECT 1 FROM UNNEST(Links) AS link WHERE %s)", sqlCondition)
	default:
		return sqlCondition
	}
}
