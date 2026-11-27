package logs

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/stretchr/testify/require"
)

var updateGolden = flag.Bool("update-golden", false, "rewrite the golden SQL files")

func normalizeGoldenSQL(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}

// TestSearchLogPageSQLGolden pins the rendered searchLogPage SQL shapes byte for byte.
//
// Regenerate with: go test ./desktopexporter/internal/store/logs/ -run Golden -update-golden
func TestSearchLogPageSQLGolden(t *testing.T) {
	cases := []struct {
		name     string
		criteria any
		page     PageOptions
	}{
		{
			name: "no_search",
			page: PageOptions{Size: 50, Offset: 0},
		},
		{
			name: "with_search",
			criteria: map[string]any{
				"id":   "n1",
				"type": "condition",
				"query": map[string]any{
					"field": map[string]any{
						"name":           "http.method",
						"searchScope":    "attribute",
						"attributeScope": "log",
						"type":           "string",
					},
					"fieldOperator": "=",
					"value":         "GET",
				},
			},
			page: PageOptions{Size: 50, Offset: 0},
		},
		{
			name: "with_columns",
			page: PageOptions{
				Size:   50,
				Offset: 0,
				Columns: []Column{
					{Source: "log", Kind: "field", Key: "flags"},
					{Source: "log", Kind: "attribute", Key: "http.method"},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rendered, err := logResultsSQL(
				store.BoundedTimeRange(0, 1<<62),
				tc.criteria,
				search.ResultOptions{},
				&tc.page,
				nil,
				false,
			)
			require.NoError(t, err)

			got := normalizeGoldenSQL(rendered.sql)
			path := filepath.Join("testdata", "search_log_page_"+tc.name+".golden")
			if *updateGolden {
				require.NoError(t, os.MkdirAll("testdata", 0o755))
				require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
				return
			}

			want, err := os.ReadFile(path)
			require.NoError(t, err, "missing golden file; run with -update-golden")
			require.Equal(t, normalizeGoldenSQL(string(want)), got,
				"rendered SQL changed. If deliberate, re-run with -update-golden and read the diff carefully")
		})
	}
}
