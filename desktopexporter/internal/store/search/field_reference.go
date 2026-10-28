package search

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

// FieldReference addresses a value relative to its owner. Strings are exact
// object keys; integers are zero-based array indices. Display names are not paths.
type FieldReference struct {
	Owner string `json:"owner"`
	Path  []any  `json:"path"`
	Type  string `json:"type,omitempty"`
}

// UnscopedPath is an exact dynamic-field address with no selected owner.
// Registered scalar fields retain their explicit signal-specific meaning.
func (field *FieldDefinition) UnscopedPath() []any {
	if field == nil || field.Owner != "" {
		return nil
	}
	if len(field.Path) > 0 {
		return field.Path
	}
	if field.SearchScope == "attribute" && field.AttributeScope == "" {
		return []any{"attributes", field.Name}
	}
	return nil
}

// Reference accepts the previous wire representation at the API boundary. The
// resolver itself only consumes owner-relative addresses.
func (field *FieldDefinition) Reference(defaultOwner string) (FieldReference, error) {
	if field.Owner != "" {
		if len(field.Path) == 0 {
			return FieldReference{}, fmt.Errorf("empty field path: %w", ErrInvalidQuery)
		}
		return FieldReference{Owner: field.Owner, Path: field.Path, Type: field.Type}, nil
	}
	if field.SearchScope == "attribute" {
		root := "attributes"
		owner := field.AttributeScope
		if owner == "metric" {
			owner = "resource"
		}
		if owner == "metadata" {
			owner, root = "metric", "metadata"
		}
		return FieldReference{Owner: owner, Path: []any{root, field.Name}, Type: field.Type}, nil
	}
	if field.SearchScope != "field" {
		return FieldReference{}, fmt.Errorf("invalid field reference: %w", ErrInvalidQuery)
	}
	owner, name := defaultOwner, field.Name
	if prefix, suffix, found := strings.Cut(name, "."); found {
		switch prefix {
		case "resource", "scope", "event", "link", "result":
			owner, name = prefix, suffix
		}
	}
	return FieldReference{Owner: owner, Path: []any{name}, Type: field.Type}, nil
}

// ValueOwner supplies native documents and any required child correlation.
// Every SQL fragment in this registry is server-owned.
type ValueOwner struct {
	Documents   map[string]string
	Correlation string
}

// Resolver shares address resolution across signals. Registered scalar and
// derived expressions use the same comparison compiler as native documents.
func Resolver(defaultOwner string, owners map[string]ValueOwner,
	registered func(*FieldDefinition) (ResolvedExpression, error),
	freeText func() ([]ResolvedExpression, error),
) FieldMapper {
	return func(field *FieldDefinition, query *Query, params *[]NamedParam) ([]ResolvedExpression, error) {
		if field == nil || (field.Owner == "" && field.SearchScope == "global") {
			return freeText()
		}
		if path := field.UnscopedPath(); len(path) > 0 {
			root, ok := path[0].(string)
			if !ok || len(path) < 2 {
				return nil, fmt.Errorf("unscoped field must address a document value: %w", ErrInvalidQuery)
			}
			var names []string
			for name, owner := range owners {
				if _, ok := owner.Documents[root]; ok {
					names = append(names, name)
				}
			}
			sort.Strings(names)
			var expressions []ResolvedExpression
			for _, name := range names {
				owner := owners[name]
				expression, err := ResolveValue(owner.Documents[root], path[1:], field.Type, params)
				if err != nil {
					return nil, err
				}
				expression.Correlation = owner.Correlation
				expressions = append(expressions, expression)
			}
			if len(expressions) == 0 {
				return []ResolvedExpression{{SQL: "NULL", PresentSQL: "false", ValueSQL: "NULL::variant"}}, nil
			}
			return expressions, nil
		}
		ref, err := field.Reference(defaultOwner)
		if err != nil {
			return nil, err
		}
		owner, ok := owners[ref.Owner]
		if !ok {
			return nil, fmt.Errorf("unknown owner %q: %w", ref.Owner, ErrInvalidQuery)
		}
		root, ok := ref.Path[0].(string)
		if !ok {
			return nil, fmt.Errorf("owner path must begin with a key: %w", ErrInvalidQuery)
		}
		if document, ok := owner.Documents[root]; ok && len(ref.Path) > 1 {
			expression, err := ResolveValue(document, ref.Path[1:], ref.Type, params)
			expression.Correlation = owner.Correlation
			expression.MissingIsNull = ref.Owner == "resource" || ref.Owner == "scope"
			return []ResolvedExpression{expression}, err
		}
		if len(ref.Path) != 1 {
			return nil, fmt.Errorf("unknown owner path: %w", ErrInvalidQuery)
		}
		name := root
		if ref.Owner != defaultOwner {
			name = ref.Owner + "." + root
		}
		expression, err := registered(&FieldDefinition{Name: name, Type: ref.Type})
		expression.Correlation = owner.Correlation
		return []ResolvedExpression{expression}, err
	}
}

// pathParameter preserves numeric keys and converts zero-based addresses to
// DuckDB's one-based UINTEGER array indices.
func pathParameter(segment any) (any, error) {
	switch value := segment.(type) {
	case string:
		return value, nil
	case int:
		if value >= 0 && uint64(value) < math.MaxUint32 {
			return int64(value) + 1, nil
		}
	case int64:
		if value >= 0 && value < math.MaxUint32 {
			return value + 1, nil
		}
	case float64:
		if value >= 0 && value < math.MaxUint32 && math.Trunc(value) == value {
			return int64(value) + 1, nil
		}
	case json.Number:
		if index, err := value.Int64(); err == nil && index >= 0 && index < math.MaxUint32 {
			return index + 1, nil
		}
	}
	return nil, fmt.Errorf("path segment must be a string or uint32 array index: %w", ErrInvalidQuery)
}
