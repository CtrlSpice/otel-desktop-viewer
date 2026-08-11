package spans

// searchSpansParams are the conditional fragments searchSpansSQL assembles into
// queries/spans/search_spans.sql.
//
// Named fields rather than positional arguments: the previous form threaded
// four %s through ~150 lines of SQL, so getting the order wrong swapped a join
// for an expression and produced SQL that still parsed.
//
// Deliberately here rather than in package queries. The query file is shared
// infrastructure; which fragments it needs, and when they are empty, is
// knowledge belonging to the code that builds them.
type searchSpansParams struct {
	// CTEs is the search_params CTE, always present.
	CTEs string
	// MatchedCTE, MatchedExpr and MatchedJoin are empty, "true" and empty
	// respectively when there is no search predicate.
	MatchedCTE  string
	MatchedExpr string
	MatchedJoin string
}
