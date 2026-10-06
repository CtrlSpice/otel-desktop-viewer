package spans

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/stretchr/testify/require"
)

var updateGolden = flag.Bool("update-golden", false, "rewrite the golden SQL files")

// The two shapes getTraceViewSQL renders. A search predicate adds the
// matched_spans CTE, its join and the per-span matched expression; without one
// they collapse to empty strings and a literal `true`. Every other input feeds
// bound parameters rather than the text, so these two cover the rendered SQL.
var goldenCases = []struct {
	name     string
	traceID  string
	criteria any
}{
	{"no_search", "00000000000000000000000000000099", nil},
	{"with_search", "00000000000000000000000000000099", map[string]any{
		"id":   "n1",
		"type": "condition",
		"query": map[string]any{
			"field": map[string]any{
				"name":           "http.method",
				"searchScope":    "attribute",
				"attributeScope": "span",
				"type":           "string",
			},
			"fieldOperator": "=",
			"value":         "GET",
		},
	}},
}

// TestGetTraceViewSQLGolden pins the rendered SQL byte for byte.
//
// This text comparison detects rendered SQL changes that fixture-based
// execution tests may not expose.
//
// Regenerate with: go test ./internal/store/spans/ -run Golden -update-golden
func TestGetTraceViewSQLGolden(t *testing.T) {
	// Cover both the hot path and its cycle-aware fallback templates.
	for _, q := range []struct {
		prefix string
		name   queries.Name
	}{
		{"get_trace_view", queries.GetTraceView},
		{"salvage_spans", queries.SalvageSpans},
	} {
		for _, tc := range goldenCases {
			t.Run(q.prefix+"_"+tc.name, func(t *testing.T) {
				query, _, err := renderSpansQuery(q.name, tc.traceID, tc.criteria)
				require.NoError(t, err)

				path := filepath.Join("testdata", q.prefix+"_"+tc.name+".sql")
				if *updateGolden {
					require.NoError(t, os.MkdirAll("testdata", 0o755))
					require.NoError(t, os.WriteFile(path, []byte(query), 0o644))
					return
				}

				want, err := os.ReadFile(path)
				require.NoError(t, err, "missing golden file; run with -update-golden")
				require.Equal(t, string(want), query,
					"rendered SQL changed. If deliberate, re-run with -update-golden and read the diff carefully")
			})
		}
	}
}

// TestGetTraceViewSQLBindsTraceID verifies the bound argument that golden SQL
// files cannot observe.
func TestGetTraceViewSQLBindsTraceID(t *testing.T) {
	t.Parallel()
	query, args, err := getTraceViewSQL("00000000000000000000000000000099", nil)
	require.NoError(t, err)
	require.NotEmpty(t, args)
	require.Equal(t, "00000000000000000000000000000099", args[0])
	require.NotContains(t, query, "00000000000000000000000000000099",
		"trace id must be bound, not interpolated")
}

// Same contract as the getTraceView golden, for the trace-summary query. Its two
// shapes are the same two: a search predicate is either present or it is not.
func TestSearchTraceSummariesSQLGolden(t *testing.T) {
	for _, tc := range goldenCases {
		t.Run(tc.name, func(t *testing.T) {
			query, _, err := searchTraceSummariesSQL(store.BoundedTimeRange(0, 1<<62), tc.criteria, search.ResultOptions{})
			require.NoError(t, err)

			path := filepath.Join("testdata", "search_trace_summaries_"+tc.name+".sql")
			if *updateGolden {
				require.NoError(t, os.MkdirAll("testdata", 0o755))
				require.NoError(t, os.WriteFile(path, []byte(query), 0o644))
				return
			}
			want, err := os.ReadFile(path)
			require.NoError(t, err, "missing golden file; run with -update-golden")
			require.Equal(t, string(want), query,
				"rendered SQL changed. If deliberate, re-run with -update-golden and read the diff carefully")
		})
	}
}

func TestSearchTraceSummariesSQLBindsLimit(t *testing.T) {
	t.Parallel()
	limit := int64(3)
	query, args, err := searchTraceSummariesSQL(store.BoundedTimeRange(0, 1<<62), nil, search.ResultOptions{Limit: &limit})
	require.NoError(t, err)
	require.Equal(t, limit, args[len(args)-1])
	require.Contains(t, query, "limit ?")
	require.NotContains(t, query, "limit 3")
}
