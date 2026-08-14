package queries_test

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
	"github.com/duckdb/duckdb-go/v2"
	"github.com/stretchr/testify/require"
)

// TestQueriesParse prepares every non-templated query against a database
// carrying the full schema.
//
// Preparing validates syntax and every table, column and macro reference
// without running anything or needing a fixture, so it takes milliseconds and
// fails at the point of editing.
//
// It exists because nothing else checked this. A query is only exercised when
// some store test happens to execute it, so a malformed one is caught late, by
// a test whose name has nothing to do with the mistake -- or not at all, if no
// test covers that path. get_metric.sql is ~700 lines across 30 CTEs, where a
// CTE list is comma-separated with a special last element and DuckDB rejects a
// trailing comma, so inserting a stage means editing its neighbours. Three
// syntax errors went in that way while the cross-series aggregate was being
// written, each reported against the line *after* the mistake.
//
// Templated queries are skipped: rendering them needs caller-shaped data, and
// the golden tests in the signal packages already pin their output byte for
// byte.
func TestQueriesParse(t *testing.T) {
	connector, err := duckdb.NewConnector("", nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = connector.Close() })

	db := sql.OpenDB(connector)
	t.Cleanup(func() { _ = db.Close() })

	// Order matters: types, then tables, then indexes, then macros -- the same
	// sequence NewStore uses, for the same reasons.
	for _, group := range [][]queries.Statement{
		queries.Types(), queries.Tables(), queries.Indexes(), queries.Macros(),
	} {
		for _, stmt := range group {
			_, err := db.Exec(stmt.SQL)
			require.NoErrorf(t, err, "creating %s", stmt.Name)
		}
	}

	for _, name := range queries.Names() {
		t.Run(string(name), func(t *testing.T) {
			sqlText, err := queries.Render(name, nil)
			if err != nil {
				t.Skipf("templated, covered by the golden tests: %v", err)
			}
			if strings.Contains(sqlText, "{{") {
				t.Skip("templated, covered by the golden tests")
			}
			stmt, err := db.Prepare(sqlText)
			require.NoErrorf(t, err, "%s does not parse", name)
			require.NoError(t, stmt.Close())
		})
	}
}
