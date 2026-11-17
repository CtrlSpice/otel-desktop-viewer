package search

import (
	"fmt"
)

// ResolveValue extracts a native value with bind-time constant path components.
// Keys and indices are parameters, never SQL identifiers or dynamic CTE columns.
func ResolveValue(document string, path []any, requestedType string, params *[]NamedParam) (ResolvedExpression, error) {
	kind, mode, err := AttributeKind(requestedType)
	if err != nil {
		return ResolvedExpression{}, err
	}
	value, present := document, "true"
	for _, segment := range path {
		key, err := pathParameter(segment)
		if err != nil {
			return ResolvedExpression{}, err
		}
		name := fmt.Sprintf("attr_key_%d", len(*params))
		*params = append(*params, NamedParam{Name: name, Value: key})
		parameter := fmt.Sprintf("$%d", len(*params))
		if _, index := key.(int64); index {
			// DuckDB's native array overload takes UINTEGER. Keep the cast on
			// the bound constant, not on a search_params column.
			parameter += "::uinteger"
		}
		present = "variant_exists(" + value + ", " + parameter + ")"
		if _, index := key.(int64); index {
			// Extraction returns VARIANT_NULL for both absent indices and
			// received empty elements. Native length preserves that distinction.
			present = "variant_array_length(" + value + ") >= " + parameter
		}
		value += "[" + parameter + "]"
	}
	if kind != "" {
		name := fmt.Sprintf("attr_kind_%d", len(*params))
		*params = append(*params, NamedParam{Name: name, Value: kind})
		present += " and otel_value_kind(" + value + ") = " + Parameter(len(*params)-1, kind)
	}
	return ResolvedExpression{SQL: AttributeValueExpression(kind, value), ValueSQL: value, PresentSQL: present, AttributeKind: kind, OperandMode: mode}, nil
}

func buildValueComparison(resolved ResolvedExpression, query *Query, params *[]NamedParam) (string, error) {
	var predicate string
	if resolved.OperandMode == OTelArrayOperand {
		if query.FieldOperator != "CONTAINS" && query.FieldOperator != "NOT CONTAINS" {
			return "", fmt.Errorf("unsupported array attribute query: %w", ErrInvalidQuery)
		}
		kind := map[string]string{"string[]": "string", "int64[]": "int64", "float64[]": "double", "boolean[]": "bool"}[query.Field.Type]
		_, mode, err := AttributeKind(kind)
		if err != nil {
			return "", err
		}
		comparison := *query
		comparison.FieldOperator = "="
		child, err := BuildOperatorCondition(AttributeExpression(AttributeValueExpression(kind, "child.value"), kind, mode), &comparison, params)
		if err != nil {
			return "", err
		}
		if kind != "" {
			child = "otel_value_kind(child.value) = '" + kind + "' and " + child
		}
		predicate = "exists(select 1 from unnest(case when variant_type(" + resolved.ValueSQL + ")='ARRAY' then " + resolved.ValueSQL + "::variant[] else []::variant[] end) child(value) where " + child + ")"
		if query.FieldOperator == "NOT CONTAINS" {
			predicate = "not " + predicate
		}
	} else {
		expression := resolved.SQL
		if query.FieldOperator == "IS NULL" && resolved.MissingIsNull {
			predicate = "not coalesce((" + resolved.PresentSQL + " and " + expression + " is not null), false)"
			return predicate, nil
		}
		var err error
		predicate, err = BuildOperatorCondition(AttributeExpression(expression, resolved.AttributeKind, resolved.OperandMode), query, params)
		if err != nil {
			return "", err
		}
	}
	return "(" + resolved.PresentSQL + " and " + predicate + ")", nil
}

func correlateValue(correlation, predicate string) string {
	if correlation == "" {
		return predicate
	}
	return "exists(select 1 from " + correlation + " and " + predicate + ")"
}
