package searchindex

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
)

// These boundaries preserve the previous complete-token plus punctuation-
// component contract. No token table or inverted text index is persisted.
var splitText = regexp.MustCompile(`[^\p{L}\p{N}_.:/?=&%#@+~-]+`)
var plainWord = regexp.MustCompile(`^[\p{L}\p{N}]+$`)

func BuildSearchSQL(node *search.QueryNode, mapper search.FieldMapper, timeCondition string, timeParams []search.NamedParam, signal, scope string) (string, string, []any, error) {
	if !search.HasUntargetedText(node) && !search.HasUnscopedField(node) && !search.HasChildField(node) {
		return search.BuildSearchSQL(node, mapper, timeCondition, timeParams)
	}
	switch signal {
	case "span", "log", "metric", "experiment":
	default:
		return "", "", nil, fmt.Errorf("invalid search signal: %w", search.ErrInvalidQuery)
	}
	var hits []string
	textMapper := func(q *search.Query, params *[]search.NamedParam) (string, error) {
		if q.FieldOperator != "CONTAINS" && q.FieldOperator != "NOT CONTAINS" {
			return "", fmt.Errorf("untargeted text supports CONTAINS and NOT CONTAINS: %w", search.ErrInvalidQuery)
		}
		var conditions []string
		for _, word := range splitText.Split(q.Value, -1) {
			if word == "" {
				continue
			}
			*params = append(*params, search.NamedParam{Name: fmt.Sprintf("text_%d", len(*params)), Value: word})
			parameter := fmt.Sprintf("$%d", len(*params))
			boundary := `[^\p{L}\p{N}_.:/?=&%#@+~-]`
			if plainWord.MatchString(word) {
				boundary = `[^\p{L}\p{N}]`
			}
			pattern := `'(^|` + boundary + `)'||regexp_escape(lower(` + parameter + `))||'(` + boundary + `|$)'`
			hit := fmt.Sprintf("search_hits_%d", len(hits))
			hits = append(hits, hit+` as materialized (
			 select distinct s.candidate_id from search_scope s join search_postings p on p.owner_id=s.owner_id
			 where p.value_id in(select id from search_values where regexp_matches(lower(value),`+pattern+`))
			 or p.path_id in(select id from search_paths where is_attribute and regexp_matches(lower(key_text),`+pattern+`)))`)
			conditions = append(conditions, "search_record.id in(select candidate_id from "+hit+")")
		}
		predicate := "false"
		if len(conditions) > 0 {
			predicate = "(" + strings.Join(conditions, " and ") + ")"
		}
		if q.FieldOperator == "NOT CONTAINS" {
			predicate = "not (" + predicate + ")"
		}
		return predicate, nil
	}
	// A selected native child must belong to this candidate's context chain.
	// Otherwise a text hit in one event/datapoint could combine with a typed
	// predicate satisfied only by a different sibling.
	nativeMapper := func(field *search.FieldDefinition, q *search.Query, params *[]search.NamedParam) ([]search.ResolvedExpression, error) {
		expressions, err := mapper(field, q, params)
		if err != nil {
			return nil, err
		}
		return nativeOwnerHits(expressions, q, params, &hits)
	}
	cte, where, args, err := search.BuildSearchSQLWithText(node, nativeMapper, textMapper, timeCondition, timeParams)
	if err != nil {
		return "", "", nil, err
	}
	graph := `search_scope_refs as materialized (
	 select c.id candidate_id,unnest([c.ref,c.parent_ref,p.parent_ref,c.resource_ref,c.scope_ref,p.resource_ref,p.scope_ref,g.resource_ref,g.scope_ref]) ref
	 from search_owners c left join search_owners p on p.ref=c.parent_ref left join search_owners g on g.ref=p.parent_ref where c.signal='` + signal + `'),
	 search_scope as materialized(select distinct r.candidate_id,o.id owner_id from search_scope_refs r join search_owners o using(ref))`
	cte = "with " + graph
	if len(hits) > 0 {
		cte += ", " + strings.Join(hits, ", ")
	}
	return cte, "exists(select 1 from search_owners search_record where search_record.signal='" + signal + "' and (" + scope + ") and (" + where + "))", args, nil
}
