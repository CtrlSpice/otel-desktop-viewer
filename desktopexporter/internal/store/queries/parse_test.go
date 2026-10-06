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
// without executing the query or requiring fixtures.
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
