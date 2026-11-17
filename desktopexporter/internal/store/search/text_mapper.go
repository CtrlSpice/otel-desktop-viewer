package search

import (
	"fmt"
	"strings"
)

// TextMapper resolves an untargeted text leaf. The normal FieldMapper still
// resolves every selected owner/path using its native comparison contract.
type TextMapper func(*Query, *[]NamedParam) (string, error)

// RejectUnindexedText prevents a selected-field mapper from accidentally
// reinstating a per-query scan of every native attribute document.
func RejectUnindexedText() ([]ResolvedExpression, error) {
	return nil, fmt.Errorf("untargeted text requires indexed resolution: %w", ErrInvalidQuery)
}

// HasChildField selects one child/context candidate for the entire expression,
// including queries that contain only explicitly scoped field comparisons.
func HasChildField(node *QueryNode) bool {
	if node == nil {
		return false
	}
	if node.Query != nil && node.Query.Field != nil {
		f := node.Query.Field
		owner := f.Owner
		if owner == "" {
			owner = f.AttributeScope
		}
		if owner == "" && f.SearchScope == "field" {
			owner, _, _ = strings.Cut(f.Name, ".")
		}
		switch owner {
		case "event", "link", "datapoint", "exemplar", "result":
			return true
		}
	}
	if node.Group != nil {
		for i := range node.Group.Children {
			if HasChildField(&node.Group.Children[i]) {
				return true
			}
		}
	}
	return false
}

// HasUntargetedText includes the old global wire reference as well as the
// current text AST node. It does not change validation or introduce syntax.
func HasUntargetedText(node *QueryNode) bool {
	if node == nil {
		return false
	}
	if node.Type == "text" {
		return true
	}
	if node.Type == "condition" && node.Query != nil && node.Query.Field != nil {
		f := node.Query.Field
		return f.Owner == "" && f.SearchScope == "global"
	}
	if node.Type == "group" && node.Group != nil {
		for i := range node.Group.Children {
			if HasUntargetedText(&node.Group.Children[i]) {
				return true
			}
		}
	}
	return false
}

// HasUnscopedField requires candidate-bound evaluation even without free text,
// so sibling owners cannot satisfy separate clauses of the same expression.
func HasUnscopedField(node *QueryNode) bool {
	if node == nil {
		return false
	}
	if node.Query != nil && len(node.Query.Field.UnscopedPath()) > 0 {
		return true
	}
	if node.Group != nil {
		for i := range node.Group.Children {
			if HasUnscopedField(&node.Group.Children[i]) {
				return true
			}
		}
	}
	return false
}

// BuildSearchSQLWithText preserves the native tree walker for selected fields
// and permits a server-owned text resolver for indexed eligibility. It uses the
// same positional binding and time-range serializer as BuildSearchSQL.
func BuildSearchSQLWithText(node *QueryNode, mapper FieldMapper, textMapper TextMapper, timeCondition string, timeParams []NamedParam) (string, string, []any, error) {
	params := append([]NamedParam(nil), timeParams...)
	var conditions []string
	if node != nil {
		if err := buildConditions(node, &conditions, &params, mapper, textMapper); err != nil {
			return "", "", nil, err
		}
	}
	return finishSearchSQL(conditions, params, timeCondition)
}

func buildTextCondition(query *Query, conditions *[]string, params *[]NamedParam, mapper TextMapper) error {
	predicate, err := mapper(query, params)
	if err != nil {
		return err
	}
	*conditions = append(*conditions, predicate)
	return nil
}
