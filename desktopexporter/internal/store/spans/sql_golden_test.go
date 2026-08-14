package spans

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

var updateGolden = flag.Bool("update-golden", false, "rewrite the golden SQL files")

// The two shapes searchSpansSQL renders. A search predicate adds the
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

// TestSearchSpansSQLGolden pins the rendered SQL byte for byte.
//
// It exists for one job: the query bodies are moving out of Go string literals
// into embedded .sql files rendered through text/template, and that move is
// only worth making if it provably changes nothing. A diff here after the move
// means the relocation was not a relocation.
//
// It is deliberately a text comparison rather than an execution test. The store
// tests already prove the queries return the right rows; what they cannot tell
// you is whether a refactor quietly changed the SQL in a way that happens to
// produce the same answer on the fixtures.
//
// Regenerate with: go test ./internal/store/spans/ -run Golden -update-golden
func TestSearchSpansSQLGolden(t *testing.T) {
	for _, tc := range goldenCases {
		t.Run(tc.name, func(t *testing.T) {
			query, _, err := searchSpansSQL(tc.traceID, tc.criteria)
			require.NoError(t, err)

			path := filepath.Join("testdata", "search_spans_"+tc.name+".sql")
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

// TestSearchSpansSQLBindsTraceID guards the half the golden files cannot: that
// the trace id travels as a bound argument rather than being interpolated into
// the text. A golden file would look identical either way, since the id is not
// in it -- which is the point.
func TestSearchSpansSQLBindsTraceID(t *testing.T) {
	query, args, err := searchSpansSQL("00000000000000000000000000000099", nil)
	require.NoError(t, err)
	require.NotEmpty(t, args)
	require.Equal(t, "00000000000000000000000000000099", args[0])
	require.NotContains(t, query, "00000000000000000000000000000099",
		"trace id must be bound, not interpolated")
}

// Same contract as the searchSpans golden, for the trace-summary query. Its two
// shapes are the same two: a search predicate is either present or it is not.
func TestSearchTracesSQLGolden(t *testing.T) {
	for _, tc := range goldenCases {
		t.Run(tc.name, func(t *testing.T) {
			query, _, err := searchTracesSQL(0, 1<<62, tc.criteria)
			require.NoError(t, err)

			path := filepath.Join("testdata", "search_traces_"+tc.name+".sql")
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
