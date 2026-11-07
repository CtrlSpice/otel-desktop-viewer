package search

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

var (
	ErrInvalidQuery = errors.New("invalid search query")
	numericInteger  = regexp.MustCompile(`^([+-]?)(?:([0-9]+)(?:\.([0-9]*))?|\.([0-9]+))(?:[eE]([+-]?[0-9]+))?$`)
	rawDuration     = regexp.MustCompile(`^\d+$`)
	durationValue   = regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*([a-zµ]+)$`)
)

// QueryNode represents a parsed query tree from the frontend
type QueryNode struct {
	ID    string      `json:"id"`
	Type  string      `json:"type"` // "condition", "text", or "group"
	Query *Query      `json:"query,omitempty"`
	Group *QueryGroup `json:"group,omitempty"`
}

// Query holds a single condition.
type Query struct {
	Field         *FieldDefinition `json:"field"`
	FieldOperator string           `json:"fieldOperator"`
	Value         string           `json:"value"`
}

// FieldDefinition accepts owner/path/type addresses and legacy wire references.
type FieldDefinition struct {
	Owner          string `json:"owner,omitempty"`
	Path           []any  `json:"path,omitempty"`
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

// NamedParam records a parameter's diagnostic name and value in binding order.
// SQL references its position directly; the name is not a query column.
type NamedParam struct {
	Name  string
	Value any
}

// unsignedScalar binds as exact decimal text because database/sql rejects
// scalar uint64 values above MaxInt64 before duckdb-go sees them.
type unsignedScalar uint64

// Parameter reuses one binding wherever its value is needed in a query.
func Parameter(index int, value any) string {
	parameter := fmt.Sprintf("$%d", index+1)
	if _, ok := value.(unsignedScalar); ok {
		return parameter + "::ubigint"
	}
	return parameter
}

// Arguments preserves the complete uint64 domain through the driver boundary.
func Arguments(params []NamedParam) []any {
	args := make([]any, len(params))
	for i, p := range params {
		if value, ok := p.Value.(unsignedScalar); ok {
			args[i] = strconv.FormatUint(uint64(value), 10)
		} else {
			args[i] = p.Value
		}
	}
	return args
}

// OperandMode defines the server-owned binding and normalisation contract.
type OperandMode uint8

const (
	TextOperand OperandMode = iota
	NativeSignedIntegerOperand
	TimestampOperand
	DurationOperand
	WireIDOperand
	OTelArrayOperand
	AttributeSignedIntegerOperand
	AttributeDoubleOperand
)

// ResolvedExpression is the mapper-owned SQL expression and operand contract.
type ResolvedExpression struct {
	// ConditionSQL is a complete server-compiled predicate, including its bound
	// operands. Catalogue mappers use it to reference precomputed owner hits.
	ConditionSQL  string
	SQL           string
	OperandMode   OperandMode
	AttributeKind string
	ValueSQL      string
	PresentSQL    string
	Correlation   string
	MissingIsNull bool
}

func Text(expr string) ResolvedExpression {
	return ResolvedExpression{SQL: expr, OperandMode: TextOperand}
}
func NativeInteger(expr string) ResolvedExpression {
	return ResolvedExpression{SQL: expr, OperandMode: NativeSignedIntegerOperand}
}
func Timestamp(expr string) ResolvedExpression {
	return ResolvedExpression{SQL: expr, OperandMode: TimestampOperand}
}
func Duration(expr string) ResolvedExpression {
	return ResolvedExpression{SQL: expr, OperandMode: DurationOperand}
}
func WireID(expr string) ResolvedExpression {
	return ResolvedExpression{SQL: expr, OperandMode: WireIDOperand}
}
func TextExpressions(expressions []string) []ResolvedExpression {
	resolved := make([]ResolvedExpression, len(expressions))
	for i, expression := range expressions {
		resolved[i] = Text(expression)
	}
	return resolved
}

// AttributeKind validates a requested dynamic attribute kind and chooses its
// server-owned operand mode.
func AttributeKind(requested string) (string, OperandMode, error) {
	switch requested {
	case "":
		return "", TextOperand, nil
	case "boolean":
		return "bool", TextOperand, nil
	case "float64":
		return "double", AttributeDoubleOperand, nil
	case "array", "string[]", "int64[]", "float64[]", "boolean[]":
		return "array", OTelArrayOperand, nil
	case "int64":
		return requested, AttributeSignedIntegerOperand, nil
	case "double":
		return requested, AttributeDoubleOperand, nil
	case "string", "bool", "bytes", "map", "empty":
		return requested, TextOperand, nil
	default:
		return "", TextOperand, fmt.Errorf("unsupported attribute kind %q: %w", requested, ErrInvalidQuery)
	}
}

func AttributeValueExpression(kind, value string) string {
	switch kind {
	case "int64":
		return "try_cast(" + value + " as bigint)"
	case "double":
		return "try_cast(" + value + " as double)"
	case "string":
		return "try_cast(" + value + " as varchar)"
	case "bool":
		return "try_cast(" + value + " as varchar)"
	default:
		return "otel_search_text(" + value + ")"
	}
}

func AttributeExpression(sql, kind string, mode OperandMode) ResolvedExpression {
	return ResolvedExpression{SQL: sql, OperandMode: mode, AttributeKind: kind}
}

// FieldMapper maps a FieldDefinition to one or more resolved SQL expressions.
// Signal-specific code provides this to the generic tree walker.
// The params slice is provided so mappers can add their own CTE parameters
// (e.g. for parameterized attribute scope/key lookups).
type FieldMapper func(field *FieldDefinition, query *Query, params *[]NamedParam) ([]ResolvedExpression, error)

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
	return buildConditions(node, conditions, params, mapper, nil)
}

func buildConditions(node *QueryNode, conditions *[]string, params *[]NamedParam, mapper FieldMapper, textMapper TextMapper) error {
	switch node.Type {
	case "text":
		if node.Query == nil || node.Query.Field != nil {
			return fmt.Errorf("invalid text query: %w", ErrInvalidQuery)
		}
		if textMapper != nil {
			return buildTextCondition(node.Query, conditions, params, textMapper)
		}
		return buildCondition(node.Query, conditions, params, mapper)
	case "condition":
		if node.Query == nil || node.Query.Field == nil {
			return fmt.Errorf("invalid condition: missing field: %w", ErrInvalidQuery)
		}
		if f := node.Query.Field; textMapper != nil && f.Owner == "" && f.SearchScope == "global" {
			return buildTextCondition(node.Query, conditions, params, textMapper)
		}
		return buildCondition(node.Query, conditions, params, mapper)
	case "group":
		return buildGroup(node.Group, conditions, params, mapper, textMapper)
	default:
		return fmt.Errorf("unknown node type %s: %w", node.Type, ErrInvalidQuery)
	}
}

func buildCondition(query *Query, conditions *[]string, params *[]NamedParam, mapper FieldMapper) error {
	if query == nil || query.FieldOperator == "" {
		return fmt.Errorf("invalid condition: missing field or operator: %w", ErrInvalidQuery)
	}

	field := query.Field

	dbExpressions, err := mapper(field, query, params)
	if err != nil {
		return fmt.Errorf("resolve search value: %w", err)
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
	// requirements and join with AND.
	if len(sqlConditions) > 1 {
		joiner := " AND "
		if field == nil || (field.Owner == "" && field.SearchScope == "global") || len(field.UnscopedPath()) > 0 {
			joiner = " OR "
		}
		*conditions = append(*conditions, "("+strings.Join(sqlConditions, joiner)+")")
	} else {
		*conditions = append(*conditions, sqlConditions...)
	}

	return nil
}

func buildGroup(group *QueryGroup, conditions *[]string, params *[]NamedParam, mapper FieldMapper, textMapper TextMapper) error {
	if group == nil {
		return fmt.Errorf("invalid group: missing group data: %w", ErrInvalidQuery)
	}

	var childConditions []string

	for _, child := range group.Children {
		var childCondition []string

		err := buildConditions(&child, &childCondition, params, mapper, textMapper)
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

func normalizeWireIDValue(value string) string {
	return strings.ToLower(strings.ReplaceAll(value, "-", ""))
}

// BuildOperatorCondition builds SQL condition for a specific operator.
func BuildOperatorCondition(resolved ResolvedExpression, query *Query, params *[]NamedParam) (string, error) {
	if query == nil {
		return "", fmt.Errorf("query cannot be nil: %w", ErrInvalidQuery)
	}
	if resolved.ConditionSQL != "" {
		return resolved.ConditionSQL, nil
	}
	if resolved.Correlation != "" {
		correlation := resolved.Correlation
		resolved.Correlation = ""
		predicate, err := BuildOperatorCondition(resolved, query, params)
		if err != nil {
			return "", err
		}
		return correlateValue(correlation, predicate), nil
	}
	if resolved.ValueSQL != "" {
		return buildValueComparison(resolved, query, params)
	}

	expression := resolved.SQL

	operator := query.FieldOperator
	value := query.Value

	const condToken = "{COND}"
	hasPlaceholder := strings.Contains(expression, condToken)
	var operatorString string

	// Null checks are explicit operators; the literal string "NULL" remains a
	// searchable value.
	if operator == "IS NULL" || operator == "IS NOT NULL" {
		if hasPlaceholder {
			return strings.ReplaceAll(expression, condToken, operator), nil
		}
		return expression + " " + operator, nil
	}

	if resolved.OperandMode == WireIDOperand {
		value = normalizeWireIDValue(value)
	}

	paramName := fmt.Sprintf("value_%d", len(*params))

	var bindValue any = value
	if (resolved.OperandMode == NativeSignedIntegerOperand || resolved.OperandMode == AttributeSignedIntegerOperand) &&
		operator != "IN" && operator != "NOT IN" {
		normalized, err := normalizeExactSignedInteger(value)
		if err != nil {
			return "", err
		}
		bindValue, _ = strconv.ParseInt(normalized, 10, 64)
	} else if resolved.OperandMode == AttributeDoubleOperand && operator != "IN" && operator != "NOT IN" {
		parsed, err := normalizeAttributeDouble(value)
		if err != nil {
			return "", err
		}
		bindValue = parsed
	} else if resolved.OperandMode == TimestampOperand && operator != "IN" && operator != "NOT IN" {
		normalized, err := normalizeNativeUnsignedInteger(value)
		if err != nil {
			return "", err
		}
		parsed, _ := strconv.ParseUint(normalized, 10, 64)
		bindValue = unsignedScalar(parsed)
	} else if resolved.OperandMode == DurationOperand && operator != "IN" && operator != "NOT IN" {
		normalized, err := NormalizeDuration(value)
		if err != nil {
			return "", err
		}
		bindValue, _ = strconv.ParseInt(normalized, 10, 64)
	}

	parameter := Parameter(len(*params), bindValue)
	switch operator {
	case "=", "!=", ">", ">=", "<", "<=":
		*params = append(*params, NamedParam{paramName, bindValue})
		operatorString = operator + " " + parameter
	// DuckDB uses ~ and !~ as full-match regex operators. !~ on NULL yields
	// NULL and excludes the row, matching NOT LIKE behavior.
	case "REGEXP":
		*params = append(*params, NamedParam{paramName, bindValue})
		operatorString = "~ " + parameter
	case "NOT REGEXP":
		*params = append(*params, NamedParam{paramName, bindValue})
		operatorString = "!~ " + parameter
	case "LIKE", "NOT LIKE":
		*params = append(*params, NamedParam{paramName, bindValue})
		operatorString = operator + " " + parameter
	case "CONTAINS":
		*params = append(*params, NamedParam{paramName, "%" + value + "%"})
		operatorString = "LIKE " + parameter
	case "NOT CONTAINS":
		*params = append(*params, NamedParam{paramName, "%" + value + "%"})
		operatorString = "NOT LIKE " + parameter
	case "^":
		*params = append(*params, NamedParam{paramName, value + "%"})
		operatorString = "LIKE " + parameter
	case "$":
		*params = append(*params, NamedParam{paramName, "%" + value})
		operatorString = "LIKE " + parameter
	case "IN", "NOT IN":
		values, err := ParseArrayValue(value)
		if err != nil {
			return "", err
		}
		if len(values) == 0 {
			return "", fmt.Errorf("IN/NOT IN requires at least one value: %w", ErrInvalidQuery)
		}
		if resolved.OperandMode == NativeSignedIntegerOperand || resolved.OperandMode == AttributeSignedIntegerOperand {
			values, err = NormalizeNativeIntegerList(values)
			if err != nil {
				return "", err
			}
		} else if resolved.OperandMode == AttributeDoubleOperand {
			values, err = normalizeAttributeDoubleList(values)
			if err != nil {
				return "", err
			}
		} else if resolved.OperandMode == TimestampOperand {
			timestampValues, timestampErr := NormalizeTimestampList(values)
			err = timestampErr
			if err != nil {
				return "", err
			}
			*params = append(*params, NamedParam{paramName, timestampValues})
			operatorString = fmt.Sprintf("%s CAST(%s AS UBIGINT[])", operator, parameter)
			break
		} else if resolved.OperandMode == DurationOperand {
			values, err = NormalizeDurationList(values)
			if err != nil {
				return "", err
			}
		} else if resolved.OperandMode == WireIDOperand {
			for i, value := range values {
				values[i] = normalizeWireIDValue(value.(string))
			}
		}
		if resolved.OperandMode == NativeSignedIntegerOperand || resolved.OperandMode == AttributeSignedIntegerOperand || resolved.OperandMode == DurationOperand {
			for i, value := range values {
				integer, err := strconv.ParseInt(value.(string), 10, 64)
				if err != nil {
					return "", fmt.Errorf("integer list element %d: %w: %w", i+1, err, ErrInvalidQuery)
				}
				values[i] = integer
			}
		}
		*params = append(*params, NamedParam{paramName, values})
		if resolved.OperandMode == TimestampOperand {
			operatorString = fmt.Sprintf("%s CAST(%s AS UBIGINT[])", operator, parameter)
		} else if resolved.OperandMode == NativeSignedIntegerOperand || resolved.OperandMode == AttributeSignedIntegerOperand || resolved.OperandMode == DurationOperand {
			operatorString = fmt.Sprintf("%s CAST(%s AS BIGINT[])", operator, parameter)
		} else if resolved.OperandMode == AttributeDoubleOperand {
			operatorString = fmt.Sprintf("%s CAST(%s AS DOUBLE[])", operator, parameter)
		} else {
			operatorString = operator + " " + parameter
		}
	default:
		return "", fmt.Errorf("unsupported operator %s: %w", operator, ErrInvalidQuery)
	}

	if hasPlaceholder {
		return strings.ReplaceAll(expression, condToken, operatorString), nil
	}
	return expression + " " + operatorString, nil
}

func normalizeAttributeDouble(value string) (float64, error) {
	if len(value) == 18 && strings.HasPrefix(value, "0x") {
		bits, err := strconv.ParseUint(value[2:], 16, 64)
		if err == nil {
			return math.Float64frombits(bits), nil
		}
	}
	if value == "" || len(value) > 256 || !numericInteger.MatchString(value) {
		return 0, fmt.Errorf("double %q is invalid: %w", value, ErrInvalidQuery)
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("double %q is invalid: %w", value, ErrInvalidQuery)
	}
	return parsed, nil
}

func normalizeAttributeDoubleList(values []any) ([]any, error) {
	normalized := make([]any, len(values))
	for i, value := range values {
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("double list element %d is invalid: %w", i+1, ErrInvalidQuery)
		}
		parsed, err := normalizeAttributeDouble(text)
		if err != nil {
			return nil, fmt.Errorf("double list element %d: %w", i+1, err)
		}
		normalized[i] = parsed
	}
	return normalized, nil
}

var durationUnits = map[string]*big.Int{
	"ns":  big.NewInt(1),
	"us":  big.NewInt(1_000),
	"µs":  big.NewInt(1_000),
	"ms":  big.NewInt(1_000_000),
	"s":   big.NewInt(1_000_000_000),
	"m":   big.NewInt(60_000_000_000),
	"min": big.NewInt(60_000_000_000),
	"h":   big.NewInt(3_600_000_000_000),
}

// NormalizeDuration accepts the browser's non-negative duration grammar and
// returns its half-up-rounded signed-int64 nanosecond representation.
func NormalizeDuration(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("duration is invalid: %w", ErrInvalidQuery)
	}
	if rawDuration.MatchString(value) {
		integer, ok := new(big.Int).SetString(value, 10)
		if ok && integer.IsInt64() {
			return integer.String(), nil
		}
		return "", fmt.Errorf("duration %q exceeds signed int64: %w", value, ErrInvalidQuery)
	}

	matches := durationValue.FindStringSubmatch(strings.ToLower(value))
	if matches == nil {
		return "", fmt.Errorf("duration %q is invalid: %w", value, ErrInvalidQuery)
	}
	multiplier := durationUnits[matches[2]]
	if multiplier == nil {
		return "", fmt.Errorf("duration %q is invalid: %w", value, ErrInvalidQuery)
	}
	whole, fraction, _ := strings.Cut(matches[1], ".")
	numerator, ok := new(big.Int).SetString(whole+fraction, 10)
	if !ok {
		return "", fmt.Errorf("duration %q is invalid: %w", value, ErrInvalidQuery)
	}
	numerator.Mul(numerator, multiplier)
	denominator := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(len(fraction))), nil)
	nanoseconds, remainder := new(big.Int), new(big.Int)
	nanoseconds.QuoRem(numerator, denominator, remainder)
	if remainder.Lsh(remainder, 1).Cmp(denominator) >= 0 {
		nanoseconds.Add(nanoseconds, big.NewInt(1))
	}
	if !nanoseconds.IsInt64() {
		return "", fmt.Errorf("duration %q exceeds signed int64: %w", value, ErrInvalidQuery)
	}
	return nanoseconds.String(), nil
}

func NormalizeDurationList(values []any) ([]any, error) {
	normalized := make([]any, len(values))
	for i, value := range values {
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("duration list element %d is invalid: %w", i+1, ErrInvalidQuery)
		}
		value, err := NormalizeDuration(text)
		if err != nil {
			return nil, fmt.Errorf("duration list element %d: %w", i+1, err)
		}
		normalized[i] = value
	}
	return normalized, nil
}

// NormalizeNativeIntegerList accepts decimal numeric spellings whose value is
// an exactly representable signed int64 without expanding exponent notation.
func NormalizeNativeIntegerList(values []any) ([]any, error) {
	normalized := make([]any, len(values))
	for i, value := range values {
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("integer list element %d is invalid: %w", i+1, ErrInvalidQuery)
		}
		value, err := normalizeExactSignedInteger(text)
		if err != nil {
			return nil, fmt.Errorf("integer list element %d: %w", i+1, err)
		}
		normalized[i] = value
	}
	return normalized, nil
}

func normalizeExactSignedInteger(text string) (string, error) {
	if text == "" || len(text) > 256 {
		return "", fmt.Errorf("integer %q is invalid: %w", text, ErrInvalidQuery)
	}
	matches := numericInteger.FindStringSubmatch(text)
	if matches == nil {
		return "", fmt.Errorf("integer %q is not a decimal number: %w", text, ErrInvalidQuery)
	}
	digits := strings.TrimLeft(matches[2]+matches[3]+matches[4], "0")
	if digits == "" {
		return "0", nil
	}
	exponentText := matches[5]
	if exponentText == "" {
		exponentText = "0"
	}
	exponent, ok := new(big.Int).SetString(exponentText, 10)
	if !ok {
		return "", fmt.Errorf("integer %q is not a decimal number: %w", text, ErrInvalidQuery)
	}
	scale := exponent.Sub(exponent, big.NewInt(int64(len(matches[3]+matches[4]))))
	var integerText string
	if scale.Sign() >= 0 {
		if new(big.Int).Add(big.NewInt(int64(len(digits))), scale).Cmp(big.NewInt(19)) > 0 {
			return "", fmt.Errorf("integer %q is not an exact signed integer: %w", text, ErrInvalidQuery)
		}
		integerText = digits + strings.Repeat("0", int(scale.Int64()))
	} else {
		shift := new(big.Int).Neg(scale)
		trailingZeros := len(digits) - len(strings.TrimRight(digits, "0"))
		if shift.Cmp(big.NewInt(int64(trailingZeros))) > 0 {
			return "", fmt.Errorf("integer %q is not an exact signed integer: %w", text, ErrInvalidQuery)
		}
		integerText = digits[:len(digits)-int(shift.Int64())]
	}
	integer, ok := new(big.Int).SetString(integerText, 10)
	if !ok {
		return "", fmt.Errorf("integer %q is not an exact signed integer: %w", text, ErrInvalidQuery)
	}
	if matches[1] == "-" {
		integer.Neg(integer)
	}
	if !integer.IsInt64() {
		return "", fmt.Errorf("integer %q is not an exact signed integer: %w", text, ErrInvalidQuery)
	}
	return integer.String(), nil
}

func normalizeNativeUnsignedInteger(value string) (string, error) {
	if value == "" || len(value) > 256 {
		return "", fmt.Errorf("timestamp %q is invalid: %w", value, ErrInvalidQuery)
	}
	matches := numericInteger.FindStringSubmatch(value)
	if matches == nil || matches[1] == "-" {
		return "", fmt.Errorf("timestamp %q is not an unsigned decimal number: %w", value, ErrInvalidQuery)
	}
	digits := strings.TrimLeft(matches[2]+matches[3]+matches[4], "0")
	if digits == "" {
		return "0", nil
	}
	exponentText := matches[5]
	if exponentText == "" {
		exponentText = "0"
	}
	exponent, ok := new(big.Int).SetString(exponentText, 10)
	if !ok {
		return "", fmt.Errorf("timestamp %q is not an unsigned decimal number: %w", value, ErrInvalidQuery)
	}
	scale := exponent.Sub(exponent, big.NewInt(int64(len(matches[3]+matches[4]))))
	var integerText string
	if scale.Sign() >= 0 {
		if new(big.Int).Add(big.NewInt(int64(len(digits))), scale).Cmp(big.NewInt(20)) > 0 {
			return "", fmt.Errorf("timestamp %q exceeds uint64: %w", value, ErrInvalidQuery)
		}
		integerText = digits + strings.Repeat("0", int(scale.Int64()))
	} else {
		shift := new(big.Int).Neg(scale)
		trailingZeros := len(digits) - len(strings.TrimRight(digits, "0"))
		if shift.Cmp(big.NewInt(int64(trailingZeros))) > 0 {
			return "", fmt.Errorf("timestamp %q is not an exact unsigned integer: %w", value, ErrInvalidQuery)
		}
		integerText = digits[:len(digits)-int(shift.Int64())]
	}
	integer, ok := new(big.Int).SetString(integerText, 10)
	if !ok || !integer.IsUint64() {
		return "", fmt.Errorf("timestamp %q exceeds uint64: %w", value, ErrInvalidQuery)
	}
	return integer.String(), nil
}

func NormalizeTimestampList(values []any) ([]uint64, error) {
	normalized := make([]uint64, len(values))
	for i, value := range values {
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("timestamp list element %d is invalid: %w", i+1, ErrInvalidQuery)
		}
		text, err := normalizeNativeUnsignedInteger(text)
		if err != nil {
			return nil, fmt.Errorf("timestamp list element %d: %w", i+1, err)
		}
		normalized[i], _ = strconv.ParseUint(text, 10, 64)
	}
	return normalized, nil
}

// ParseArrayValue parses the JSON string array sent over the query wire format.
func ParseArrayValue(value string) ([]any, error) {
	var decoded []*string
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return nil, fmt.Errorf("array value must be a JSON string array: %w", ErrInvalidQuery)
	}
	if decoded == nil {
		return nil, fmt.Errorf("array value must be a JSON string array: %w", ErrInvalidQuery)
	}

	result := make([]any, len(decoded))
	for i, v := range decoded {
		if v == nil {
			return nil, fmt.Errorf("array value must be a JSON string array: %w", ErrInvalidQuery)
		}
		result[i] = *v
	}
	return result, nil
}

// TimePredicate builds the direct predicate for a nullable inclusive range.
// Only concrete endpoints become parameters and SQL conditions.
func TimePredicate(column string, startTime, endTime *uint64) (string, []NamedParam) {
	var conditions []string
	var params []NamedParam
	if startTime != nil {
		conditions = append(conditions, column+" >= "+Parameter(len(params), unsignedScalar(*startTime)))
		params = append(params, NamedParam{Name: "time_start", Value: unsignedScalar(*startTime)})
	}
	if endTime != nil {
		conditions = append(conditions, column+" <= "+Parameter(len(params), unsignedScalar(*endTime)))
		params = append(params, NamedParam{Name: "time_end", Value: unsignedScalar(*endTime)})
	}
	return strings.Join(conditions, " AND "), params
}

// BuildSearchSQL builds the WHERE clause and directly bound args for any
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
	return finishSearchSQL(conditions, params, timeCondition)
}

func finishSearchSQL(conditions []string, params []NamedParam, timeCondition string) (cteSQL, whereSQL string, args []any, err error) {
	if len(conditions) > 0 && timeCondition != "" {
		// buildCondition and buildGroup normally append one joined element.
		// Joining defensively keeps a multi-element list valid.
		whereSQL = "(" + strings.Join(conditions, " AND ") + ") AND " + timeCondition
	} else if len(conditions) > 0 {
		whereSQL = "(" + strings.Join(conditions, " AND ") + ")"
	} else if timeCondition != "" {
		whereSQL = timeCondition
	} else {
		whereSQL = "true"
	}

	return "", whereSQL, Arguments(params), nil
}
