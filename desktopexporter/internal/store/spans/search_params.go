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

// searchTracesParams are the fragments searchTracesSQL assembles into
// queries/spans/search_traces.sql.
type searchTracesParams struct {
	// CTEs is the search_params CTE holding the time bounds.
	CTEs string
	// From is the shared FROM/JOIN chain for span search, so the summary
	// query and the matched_spans CTE in search_spans stay in step.
	From string
	// EligibilityWhere selects spans within the requested time range that also
	// match the optional query. Their trace IDs select complete stored traces.
	EligibilityWhere string
	// MatchCTEs, MatchJoin and MatchProjection add computed matching-span
	// identities only when a query predicate is present.
	MatchCTEs       string
	MatchJoin       string
	MatchProjection string
	// Order is assembled from an allowlisted summary expression and direction.
	Order string
	// Limit is empty for the existing unbounded search and "limit ?" when
	// the search request includes a result cap.
	Limit string
}
