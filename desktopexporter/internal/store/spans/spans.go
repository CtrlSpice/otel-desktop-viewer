package spans

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/nested"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
)

// Sentinel errors for use with errors.Is.
var (
	ErrTraceIDNotFound    = errors.New("trace ID not found")
	ErrInvalidTraceQuery  = errors.New("invalid trace search query")
	ErrInvalidTraceLimit  = errors.New("invalid trace search limit")
	ErrInvalidSpanID      = errors.New("span ID is empty")
	ErrSpansStoreInternal = errors.New("spans store internal error")
)

type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// GetTraceView returns a trace with matching spans annotated when criteria is
// provided. Optional spanIDs restrict payloads to those exact stored IDs and
// compress display depth through available ancestry; received parent IDs remain intact.
//
// The ordinary tree walk reports unreachable spans. A non-zero count triggers
// the cycle-aware query, which chooses entry points and tracks ancestry.
func GetTraceView(ctx context.Context, db *sql.DB, traceID string, criteria any, spanIDs ...uint64) (json.RawMessage, error) {
	query, args, err := renderSpansQuery(queries.GetTraceView, traceID, criteria, spanIDs...)
	if err != nil {
		return nil, err
	}

	var raw []byte
	var unplaced int64
	if err := db.QueryRowContext(ctx, query, args...).Scan(&raw, &unplaced); err != nil {
		return nil, fmt.Errorf("GetTraceView: %w: %w", ErrSpansStoreInternal, err)
	}
	if raw == nil {
		return nil, fmt.Errorf("GetTraceView: %w", ErrTraceIDNotFound)
	}
	if unplaced == 0 {
		return json.RawMessage(raw), nil
	}

	// Re-run ancestry with the cycle-aware walk, retaining the same payload selection.
	salvaged, err := salvageSpans(ctx, db, traceID, criteria, spanIDs...)
	if err != nil {
		// The ordinary result is still correct as far as it goes, and short a
		// few spans is better than an error page.
		return json.RawMessage(raw), nil
	}
	return salvaged, nil
}

// GetTraceOverview returns the compact, untruncated overview for one trace. The query
// computes exact nanosecond strings and reads every span in one operation.
func GetTraceOverview(ctx context.Context, db queryRower, traceID string) (json.RawMessage, error) {
	query, err := queries.Render(queries.GetTraceOverview, nil)
	if err != nil {
		return nil, fmt.Errorf("GetTraceOverview: %w: %w", ErrSpansStoreInternal, err)
	}
	var raw []byte
	if err := db.QueryRowContext(ctx, query, traceID).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("GetTraceOverview: %w", ErrTraceIDNotFound)
		}
		return nil, fmt.Errorf("GetTraceOverview: %w: %w", ErrSpansStoreInternal, err)
	}
	return json.RawMessage(raw), nil
}

// GetSpanSummaries returns at most limit stable summary rows for one span ID,
// together with the exact number of matching composite identities.
func GetSpanSummaries(ctx context.Context, db queryRower, spanID uint64, limit int64) (json.RawMessage, int64, error) {
	query, err := queries.Render(queries.GetSpanSummaries, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("GetSpanSummaries: %w: %w", ErrSpansStoreInternal, err)
	}
	var raw []byte
	var matchCount int64
	if err := db.QueryRowContext(ctx, query, []uint64{spanID}, limit).Scan(&raw, &matchCount); err != nil {
		return nil, 0, fmt.Errorf("GetSpanSummaries: %w: %w", ErrSpansStoreInternal, err)
	}
	if raw == nil {
		raw = []byte("[]")
	}
	return json.RawMessage(raw), matchCount, nil
}

// GetSpan returns full stored detail for one composite trace and span identity.
// A missing exact pair returns a nil result without an error.
func GetSpan(ctx context.Context, db queryRower, traceID string, spanID uint64) (json.RawMessage, error) {
	query, err := queries.Render(queries.GetSpan, nil)
	if err != nil {
		return nil, fmt.Errorf("GetSpan: %w: %w", ErrSpansStoreInternal, err)
	}
	var raw []byte
	if err := db.QueryRowContext(ctx, query, traceID, []uint64{spanID}).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetSpan: %w: %w", ErrSpansStoreInternal, err)
	}
	if raw == nil {
		return nil, nil
	}
	return json.RawMessage(raw), nil
}

// GetTraceOTLP returns every stored span for traceID as one standard OTLP JSON
// document. The SQL owns reconstruction so callers receive the stored signal,
// not the search and display projection. Callers must transport the returned
// bytes unchanged: parsing and re-encoding in JavaScript loses negative zero.
func GetTraceOTLP(ctx context.Context, db *sql.DB, traceID string) (json.RawMessage, error) {
	return GetTraceSelectionOTLP(ctx, db, traceID, "")
}

// GetTraceSelectionOTLP keeps resource/scope wrappers and filters by exact span identity.
// spanID is decimal unsigned text, or empty for the complete retained trace.
func GetTraceSelectionOTLP(ctx context.Context, db *sql.DB, traceID, spanID string) (json.RawMessage, error) {
	query, err := queries.Render(queries.GetTraceOTLP, nil)
	if err != nil {
		return nil, fmt.Errorf("GetTraceOTLP: %w: %w", ErrSpansStoreInternal, err)
	}

	var raw []byte
	if err := db.QueryRowContext(ctx, query, traceID, spanID, spanID).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("GetTraceOTLP: %w", ErrTraceIDNotFound)
		}
		return nil, fmt.Errorf("GetTraceOTLP: %w: %w", ErrSpansStoreInternal, err)
	}
	return json.RawMessage(raw), nil
}

// salvageSpans runs the cycle-aware variant of the trace fetch.
func salvageSpans(ctx context.Context, db *sql.DB, traceID string, criteria any, spanIDs ...uint64) (json.RawMessage, error) {
	query, args, err := renderSpansQuery(queries.SalvageSpans, traceID, criteria, spanIDs...)
	if err != nil {
		return nil, err
	}
	var raw []byte
	var unplaced int64
	if err := db.QueryRowContext(ctx, query, args...).Scan(&raw, &unplaced); err != nil {
		return nil, fmt.Errorf("salvageSpans: %w: %w", ErrSpansStoreInternal, err)
	}
	if raw == nil {
		return nil, fmt.Errorf("salvageSpans: %w", ErrTraceIDNotFound)
	}
	return json.RawMessage(raw), nil
}

// getTraceViewSQL renders the trace-fetch query and its bound arguments for
// execution and golden tests.
func getTraceViewSQL(traceID string, criteria any) (string, []any, error) {
	return renderSpansQuery(queries.GetTraceView, traceID, criteria)
}

// renderSpansQuery renders either trace-fetch query. Both take identical
// parameters and differ only in whether they carry the salvage walk, so the
// search-predicate plumbing is shared rather than duplicated.
func renderSpansQuery(name queries.Name, traceID string, criteria any, spanIDs ...uint64) (string, []any, error) {
	return spansQuery(name, traceID, criteria, false, spanIDs...)
}

func spansQuery(name queries.Name, traceID string, criteria any, native bool, spanIDs ...uint64) (string, []any, error) {
	return traceQuery(name, traceID, criteria, native, false, spanIDs...)
}

func traceQuery(name queries.Name, traceID string, criteria any, native, lightweight bool, spanIDs ...uint64) (string, []any, error) {
	return traceQueryColumns(name, traceID, criteria, native, lightweight, nil, spanIDs...)
}

func traceQueryColumns(name queries.Name, traceID string, criteria any, native, lightweight bool, columns []WaterfallColumn, spanIDs ...uint64) (string, []any, error) {
	var searchTree *search.QueryNode
	if criteria != nil {
		var err error
		searchTree, err = search.ParseQueryTree(criteria)
		if err != nil {
			return "", nil, fmt.Errorf("GetTraceView: %w: %w", ErrInvalidTraceQuery, err)
		}
	}

	cteSQL, whereClause, args, err := buildSpanSQL(searchTree, traceID)
	if err != nil {
		return "", nil, fmt.Errorf("GetTraceView: %w: %w", ErrInvalidTraceQuery, err)
	}

	// The recursive CTE always walks the full trace tree (filtered by trace_id only)
	// to compute depth and sort_path. When search criteria are present, a matched_spans
	// CTE identifies matching span IDs via LEFT JOIN so the full tree is always returned
	// with a per-span 'matched' boolean annotation.
	matchedCTE := ""
	matchedJoin := ""
	matchedExpr := "true"
	if searchTree != nil {
		if cteSQL != "" {
			cteSQL += "\n\t\t\t"
		}
		matchedCTE = fmt.Sprintf(`,
		matched_spans as (
			%sselect s.span_id
			%s
			where %s
		)`, cteSQL, spanSearchFrom, whereClause)
		matchedJoin = "left join matched_spans ms on ts.span_id = ms.span_id"
		matchedExpr = "case when ms.span_id is not null then true else false end"
	}

	// The recursion carries only traversal columns. The tree CTE joins the
	// payload once and defines one span set for every downstream projection.
	selectedIDs := ""
	if len(spanIDs) > 0 {
		selectedIDs = fmt.Sprintf("$%d::ubigint[]", len(args)+1)
		args = append(args, spanIDs)
	}
	render := queries.Render
	if native {
		render = queries.RenderArrow
	}
	previews, args, err := waterfallColumnExpressions(columns, args, "ts")
	if err != nil {
		return "", nil, err
	}
	query, err := render(name, getTraceViewParams{
		SelectedIDs:    selectedIDs,
		MatchedCTE:     matchedCTE,
		MatchedExpr:    matchedExpr,
		MatchedJoin:    matchedJoin,
		Selected:       len(spanIDs) > 0,
		Salvage:        name == queries.SalvageSpans,
		Lightweight:    lightweight,
		ColumnPreviews: previews,
	})
	if err != nil {
		return "", nil, fmt.Errorf("GetTraceView: %w: %w", ErrSpansStoreInternal, err)
	}

	return query, args, nil
}

// Clear deletes owning span rows and their complete search catalogue.
func Clear(ctx context.Context, db *sql.DB) error {
	return nested.Mutate(ctx, db, `delete from spans`)
}

// DeleteSpansByTraceIDs deletes all spans for multiple traces.
func DeleteSpansByTraceIDs(ctx context.Context, db *sql.DB, traceIDs []any) error {
	return nested.Mutate(ctx, db, `delete from spans where trace_id in(select id from uuid_list(?))`, util.ToStringList(traceIDs))
}
