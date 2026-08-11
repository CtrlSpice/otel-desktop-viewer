package queries_test

import (
	"database/sql"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/stretchr/testify/require"
)

// Every DDL statement must actually execute. Compiling proves only that the
// strings are valid Go.
func TestAllDDLExecutes(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	require.NoError(t, err)
	defer db.Close()

	for _, stmt := range queries.Types() {
		db.Exec(stmt.SQL) // "already exists" is fine
	}
	// Naming the failing file beats naming its index: the whole point of the
	// move was that "table query 4" made you count entries to find out what
	// broke.
	for _, group := range [][]queries.Statement{
		queries.Tables(), queries.Indexes(), queries.Macros(),
	} {
		for _, stmt := range group {
			_, err := db.Exec(stmt.SQL)
			require.NoErrorf(t, err, "%s:\n%s", stmt.Name, stmt.SQL)
		}
	}
}

// The wire format keys resources and scopes by seq, so the sequence defaults
// have to actually fire on insert.
func TestSequencesAssignShortKeys(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	require.NoError(t, err)
	defer db.Close()

	for _, group := range [][]queries.Statement{queries.Types(), queries.Tables()} {
		for _, stmt := range group {
			db.Exec(stmt.SQL)
		}
	}

	_, err = db.Exec(`insert into attributes values
		('11111111-1111-1111-1111-111111111111','service.name','checkout','string','resource')`)
	require.NoError(t, err)

	for _, id := range []string{
		"22222222-2222-2222-2222-222222222222",
		"33333333-3333-3333-3333-333333333333",
	} {
		_, err = db.Exec(`insert into resources (id, attribute_ids) values (?::uuid, ['11111111-1111-1111-1111-111111111111']::uuid[])`, id)
		require.NoError(t, err)
	}

	var a, b int
	require.NoError(t, db.QueryRow(`select min(seq), max(seq) from resources`).Scan(&a, &b))
	require.NotEqual(t, a, b, "each resource must get its own wire key")
}

// DuckDB cannot index a LIST, so an attribute_ids index would fail at startup.
// Pin that none is attempted -- it is the constraint behind several design
// choices and an easy one to forget.
func TestNoIndexOnArrayColumns(t *testing.T) {
	for _, stmt := range queries.Indexes() {
		require.NotContains(t, stmt.SQL, "attribute_ids",
			"DuckDB cannot index a LIST column; this would fail at store open")
	}
}
