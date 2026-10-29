package spans

// getTraceViewParams are the conditional fragments getTraceViewSQL assembles into
// queries/spans/get_trace_view.sql.
type getTraceViewParams struct {
	// SelectedIDs is the directly bound list of requested span IDs.
	SelectedIDs string
	// MatchedCTE, MatchedExpr and MatchedJoin are empty, "true" and empty
	// respectively when there is no search predicate.
	MatchedCTE  string
	MatchedExpr string
	MatchedJoin string
	// Selected limits payload enrichment to requested IDs after ancestry traversal.
	Selected bool
	Salvage  bool
}

// searchTraceSummariesParams are the fragments searchTraceSummariesSQL assembles into
// queries/spans/search_trace_summaries.sql.
type searchTraceSummariesParams struct {
	// CTEs contains optional search graph definitions.
	CTEs string
	// From is the shared FROM/JOIN chain for span search, so the summary
	// query and the matched_spans CTE in get_trace_view stay in step.
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
