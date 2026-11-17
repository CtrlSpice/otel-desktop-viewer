package searchindex

import (
	"fmt"
	"strings"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
)

// Compile selected child comparisons once over their native rows. Joining the
// resulting owner IDs to search_scope preserves the whole-expression boundary
// without a correlated nested-list expansion for every outer record/candidate.
func nativeOwnerHits(expressions []search.ResolvedExpression, q *search.Query, params *[]search.NamedParam, hits *[]string) ([]search.ResolvedExpression, error) {
	for i, e := range expressions {
		owner, source, identity := "", "", ""
		switch {
		case strings.HasPrefix(e.Correlation, "events e where "):
			owner, source, identity = "event", "events e", "e.id::varchar"
		case strings.HasPrefix(e.Correlation, "links l where "):
			owner, source, identity = "link", "links l", "l.id::varchar"
		case strings.HasPrefix(e.Correlation, "metric_datapoints d where "):
			owner, source, identity = "datapoint", "metric_datapoints d", "d.id::varchar"
		case strings.HasPrefix(e.Correlation, "exemplars e join "):
			owner, source, identity = "exemplar", "exemplars e", "e.id::varchar"
		case strings.HasPrefix(e.Correlation, "metric_records mr where "):
			owner, source, identity = "metric", "metric_records mr", "mr.record_id::varchar"
		default:
			continue
		}
		e.Correlation = ""
		predicate, err := search.BuildOperatorCondition(e, q, params)
		if err != nil {
			return nil, err
		}
		name := fmt.Sprintf("search_native_hits_%d", len(*hits))
		*hits = append(*hits, name+` as materialized(
   select distinct sc.candidate_id from search_scope sc join search_owners matched on matched.id=sc.owner_id
   join `+source+` on matched.record_key=`+identity+`
   where matched.owner='`+owner+`' and (`+predicate+`))`)
		expressions[i] = search.ResolvedExpression{ConditionSQL: "search_record.id in(select candidate_id from " + name + ")"}
	}
	return expressions, nil
}
