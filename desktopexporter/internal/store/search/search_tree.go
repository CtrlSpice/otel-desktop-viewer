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

// unsignedScalar binds through a one-element UBIGINT array because database/sql
// rejects scalar uint64 values above MaxInt64 before duckdb-go sees them.
type unsignedScalar uint64

// OperandMode defines the server-owned binding and normalisation contract.
type OperandMode uint8

const (
	TextOperand OperandMode = iota
	NativeSignedIntegerOperand
	TimestampOperand
	DurationOperand
	WireIDOperand
	OTelArrayOperand
	CompletePredicateOperand
	AttributeSignedIntegerOperand
	AttributeDoubleOperand
)

// ResolvedExpression is the mapper-owned SQL expression and operand contract.
type ResolvedExpression struct {
	SQL           string
	OperandMode   OperandMode
	AttributeKind string
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
func Complete(expr string) ResolvedExpression {
	return ResolvedExpression{SQL: expr, OperandMode: CompletePredicateOperand}
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

func AttributeValueExpression(kind string) string {
	switch kind {
	case "int64":
		return "attribute_int64(a.value)"
	case "double":
		return "attribute_double(a.value)"
	default:
		return "coalesce(json_extract_string(a.value, '$.value'), json_extract(a.value, '$.value')::varchar)"
	}
}

func AttributeExpression(sql, kind string, mode OperandMode) ResolvedExpression {
	return ResolvedExpression{SQL: sql, OperandMode: mode, AttributeKind: kind}
}

func AttributeKindPredicate(kindParam string) string {
	if kindParam == "" {
		return ""
	}
	return " and json_extract_string(a.value, '$.kind') = " + kindParam
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

// wireIDFields are field names whose values are trace/span IDs. Signal mappers
// convert their native UUID or UBIGINT columns to OTLP wire form for comparison,
// and values are normalized here to match, so dashed or uppercase input still
// works and malformed IDs match nothing instead of causing a cast error.
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
func BuildOperatorCondition(resolved ResolvedExpression, query *Query, params *[]NamedParam) (string, error) {
	if query == nil {
		return "", fmt.Errorf("query cannot be nil: %w", ErrInvalidQuery)
	}

	expression := resolved.SQL

	operator := query.FieldOperator
	value := query.Value

	// A mapper that already produced a complete boolean says so, and nothing
	// further is appended to it.
	if resolved.OperandMode == CompletePredicateOperand {
		return expression, nil
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

	if resolved.OperandMode == WireIDOperand {
		value = normalizeWireIDValue(value)
	}

	paramName := fmt.Sprintf("value_%d", len(*params))

	var bindValue any = value
	if resolved.OperandMode == NativeSignedIntegerOperand {
		if n, err := strconv.ParseInt(value, 10, 64); err == nil {
			bindValue = n
		}
	} else if resolved.OperandMode == AttributeSignedIntegerOperand && operator != "IN" && operator != "NOT IN" {
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
			operatorString = fmt.Sprintf("%s CAST(%s AS UBIGINT[])", operator, paramName)
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
		*params = append(*params, NamedParam{paramName, values})
		if resolved.OperandMode == TimestampOperand {
			operatorString = fmt.Sprintf("%s CAST(%s AS UBIGINT[])", operator, paramName)
		} else if resolved.OperandMode == NativeSignedIntegerOperand || resolved.OperandMode == AttributeSignedIntegerOperand || resolved.OperandMode == DurationOperand {
			operatorString = fmt.Sprintf("%s CAST(%s AS BIGINT[])", operator, paramName)
		} else if resolved.OperandMode == AttributeDoubleOperand {
			operatorString = fmt.Sprintf("%s CAST(%s AS DOUBLE[])", operator, paramName)
		} else {
			operatorString = operator + " " + paramName
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
			values, err := ParseArrayValue(value)
			if err != nil {
				return "", err
			}
			*params = append(*params, NamedParam{paramName, values})
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
		values, err := ParseArrayValue(value)
		if err != nil {
			return "", err
		}
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
		values, err := ParseArrayValue(value)
		if err != nil {
			return "", err
		}
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

// JSONValueArrayPredicate matches an element of a D06 tagged OTel array.
// Attribute mappers wrap it in their owner-specific resource/scope lookup.
func JSONValueArrayPredicate(attributeIDs, keyParam, kindParam string, query *Query, params *[]NamedParam) (string, error) {
	if query.FieldOperator != "CONTAINS" && query.FieldOperator != "NOT CONTAINS" {
		return "", fmt.Errorf("unsupported array attribute query: %w", ErrInvalidQuery)
	}
	valueParam := fmt.Sprintf("value_%d", len(*params))
	*params = append(*params, NamedParam{Name: valueParam, Value: ConvertValueForArrayType(query.Value, query.Field.Type)})
	kindPredicate := AttributeKindPredicate(kindParam)
	arrayExists := fmt.Sprintf(`exists(
		select 1 from unnest(%s) t(aid)
		join attributes a on a.id = t.aid
		where a.key = %s%s
	)`, attributeIDs, keyParam, kindPredicate)
	predicate := fmt.Sprintf(`exists(
		select 1 from unnest(%s) t(aid)
		join attributes a on a.id = t.aid, json_each(a.value, '$.value') j
		where a.key = %s%s
			and json_extract_string(j.value, '$.value') = %s
	)`, attributeIDs, keyParam, kindPredicate, valueParam)
	if query.FieldOperator == "NOT CONTAINS" {
		return arrayExists + " and not " + predicate, nil
	}
	return predicate, nil
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
func TimePredicate(column string, startTime, endTime *uint64) (string, []NamedParam) {
	var conditions []string
	var params []NamedParam
	if startTime != nil {
		conditions = append(conditions, column+" >= time_start")
		params = append(params, NamedParam{Name: "time_start", Value: unsignedScalar(*startTime)})
	}
	if endTime != nil {
		conditions = append(conditions, column+" <= time_end")
		params = append(params, NamedParam{Name: "time_end", Value: unsignedScalar(*endTime)})
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
		if value, ok := p.Value.(unsignedScalar); ok {
			args[i] = []uint64{uint64(value)}
			cteParams[i] = fmt.Sprintf("unnest(?::ubigint[]) as %s", p.Name)
		} else {
			args[i] = p.Value
			cteParams[i] = fmt.Sprintf("? as %s", p.Name)
		}
	}
	if len(cteParams) == 0 {
		cteSQL = "with search_params as (select true as unbounded)"
	} else {
		cteSQL = fmt.Sprintf("with search_params as (select %s)", strings.Join(cteParams, ", "))
	}
	return cteSQL, whereSQL, args, nil
}
