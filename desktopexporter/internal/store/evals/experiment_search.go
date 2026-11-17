package evals

import (
	"fmt"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/searchindex"
)

// The complete predicate filters one result and its parent metadata. Only then
// do we select experiments; their summaries and facets still include all results.
func experimentSearchSQL(criteria *search.QueryNode) (string, []any, error) {
	if criteria == nil {
		return `with searched as materialized (` + visibleExperiments + `)`, nil, nil
	}
	needsPayload := false
	cte, where, args, err := searchindex.BuildSearchSQL(criteria, experimentFieldMapper(&needsPayload), "", nil, "experiment", "search_record.ref='experiment:result:'||x.dataset_id||':'||x.ordinal::varchar")
	if err != nil {
		return "", nil, err
	}
	from := `visible v join eval_experiment_search x on x.experiment_id=v.id`
	if needsPayload {
		from += ` join eval_datasets d on d.id=v.dataset_id
join eval_records r on r.import_id=d.import_id and r.envelope=d.envelope and r.ordinal=x.ordinal`
	}
	if cte == "" {
		cte = "with "
	} else {
		cte += ", "
	}
	return cte + `visible as materialized (` + visibleExperiments + `), matched as (
select distinct v.id from ` + from + ` where ` + where + `
), searched as materialized (select v.* from visible v join matched m on m.id=v.id)`, args, nil
}

func experimentFieldMapper(needsPayload *bool) search.FieldMapper {
	return func(field *search.FieldDefinition, query *search.Query, params *[]search.NamedParam) ([]search.ResolvedExpression, error) {
		return search.Resolver("experiment", map[string]search.ValueOwner{"experiment": {}, "result": {}},
			func(field *search.FieldDefinition) (search.ResolvedExpression, error) {
				return experimentExpression(field.Name, query, needsPayload)
			}, search.RejectUnindexedText)(field, query, params)
	}
}

func experimentExpression(name string, query *search.Query, needsPayload *bool) (search.ResolvedExpression, error) {
	var expression search.ResolvedExpression
	kind := "string"
	switch name {
	case "name":
		expression = search.Text("v.name")
	case "evaluation":
		expression = search.Text("v.evaluation")
	case "source":
		expression = search.Text("v.source")
	case "format":
		expression = search.Text("v.format")
	case "id":
		expression = search.Text("v.id")
	case "datasetID":
		expression = search.Text("v.dataset_id")
	case "result.text":
		expression = search.Text("x.text")
	case "result.ordinal":
		expression = search.NativeInteger("x.ordinal")
		kind = "number"
	case "result.id", "result.provider", "result.prompt", "result.output":
		paths := map[string]string{
			"result.id": "r.payload.id", "result.provider": "r.payload.provider.id",
			"result.prompt": "r.payload.promptId", "result.output": "r.payload.response.output",
		}
		path := paths[name]
		expression = search.Text("case when variant_typeof(" + path + ")='VARCHAR' then " + path + "::varchar end")
		*needsPayload = true
	case "result.success":
		expression = search.Text("case when starts_with(variant_typeof(r.payload.success),'BOOL') then r.payload.success::varchar end")
		kind = "boolean"
		*needsPayload = true
	case "result.score":
		expression = search.ResolvedExpression{SQL: "case when regexp_full_match(variant_typeof(r.payload.score), 'U?INT(8|16|32|64|128)|FLOAT|DOUBLE') then try_cast(r.payload.score as double) end", OperandMode: search.AttributeDoubleOperand}
		kind = "number"
		*needsPayload = true
	default:
		return search.ResolvedExpression{}, fmt.Errorf("unknown Evals field %q: %w", name, search.ErrInvalidQuery)
	}
	// Field types and permitted operators are server-owned, not supplied by
	// the browser. In particular, numeric values never fall back to text.
	op := query.FieldOperator
	allowed := op == "=" || op == "!=" || op == "IN" || op == "NOT IN" || op == "IS NULL" || op == "IS NOT NULL"
	if kind == "number" {
		allowed = allowed || op == ">" || op == ">=" || op == "<" || op == "<="
	} else if kind == "string" {
		allowed = allowed || op == "CONTAINS" || op == "NOT CONTAINS" || op == "LIKE" || op == "NOT LIKE" || op == "REGEXP" || op == "NOT REGEXP"
	}
	if !allowed {
		return search.ResolvedExpression{}, fmt.Errorf("operator %q is not supported for %s: %w", op, name, search.ErrInvalidQuery)
	}
	if kind == "boolean" && op != "IS NULL" && op != "IS NOT NULL" {
		values := []any{query.Value}
		if op == "IN" || op == "NOT IN" {
			var err error
			values, err = search.ParseArrayValue(query.Value)
			if err != nil {
				return search.ResolvedExpression{}, err
			}
		}
		for _, value := range values {
			if value != "true" && value != "false" {
				return search.ResolvedExpression{}, fmt.Errorf("expected true or false: %w", search.ErrInvalidQuery)
			}
		}
	}
	return expression, nil
}
