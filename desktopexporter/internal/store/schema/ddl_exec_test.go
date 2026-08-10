package schema_test

import (
	"database/sql"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/schema"
	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/stretchr/testify/require"
)

// Every DDL statement must actually execute. Compiling proves only that the
// strings are valid Go.
func TestAllDDLExecutes(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	require.NoError(t, err)
	defer db.Close()

	for _, q := range schema.TypeCreationQueries {
		db.Exec(q) // "already exists" is fine
	}
	for i, q := range schema.TableCreationQueries {
		_, err := db.Exec(q)
		require.NoErrorf(t, err, "table query %d:\n%s", i, q)
	}
	for i, q := range schema.IndexCreationQueries {
		_, err := db.Exec(q)
		require.NoErrorf(t, err, "index query %d:\n%s", i, q)
	}
	for i, q := range schema.MacroCreationQueries {
		_, err := db.Exec(q)
		require.NoErrorf(t, err, "macro query %d:\n%s", i, q)
	}
}

// The wire format keys resources and scopes by seq, so the sequence defaults
// have to actually fire on insert.
func TestSequencesAssignShortKeys(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	require.NoError(t, err)
	defer db.Close()

	for _, qs := range [][]string{schema.TypeCreationQueries, schema.TableCreationQueries} {
		for _, q := range qs {
			db.Exec(q)
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
	for _, q := range schema.IndexCreationQueries {
		require.NotContains(t, q, "attribute_ids",
			"DuckDB cannot index a LIST column; this would fail at store open")
	}
}
