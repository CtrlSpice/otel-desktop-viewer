package queries_test

import (
	"database/sql"
	"fmt"
	"os"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
	_ "github.com/duckdb/duckdb-go/v2"
)

// One DuckDB for the tests in this package that only read.
//
// Most tests here evaluate a macro against literal arguments -- downscaling a
// bucket array, slicing a list, folding counts. They never write a row, so the
// database they run against is a constant: types and macros installed, tables
// empty. Building that per test was thirteen copies of the same preamble, and
// the copies had already drifted -- some loaded Types before Macros, some
// discarded the error from Types, and setupMacroDB installed macros but not
// types, which is why the files needing both hand-rolled it instead of calling
// it.
//
// The split is by what a test does to the database, not by what it costs.
// Sharing is safe for the readers because DuckDB evaluating a scalar macro
// touches nothing another test can observe. It is not safe for the two tests
// that write -- see freshDB.
var sharedDB *sql.DB

func TestMain(m *testing.M) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		fmt.Fprintln(os.Stderr, "opening shared duckdb:", err)
		os.Exit(1)
	}
	if err := install(db); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sharedDB = db
	code := m.Run()
	_ = db.Close()
	os.Exit(code)
}

// install puts the schema on a connection: types first, since macros and
// tables both refer to them, then macros, then tables.
func install(db *sql.DB) error {
	// Types are created with plain CREATE TYPE and there is no IF NOT EXISTS
	// for it, so a redundant one is an "already exists" error rather than a
	// real failure. Every other statement here is checked.
	for _, stmt := range queries.Types() {
		db.Exec(stmt.SQL)
	}
	for _, stmt := range queries.Macros() {
		if _, err := db.Exec(stmt.SQL); err != nil {
			return fmt.Errorf("creating macro %s: %w", stmt.Name, err)
		}
	}
	for _, stmt := range queries.Tables() {
		if _, err := db.Exec(stmt.SQL); err != nil {
			return fmt.Errorf("creating table %s: %w", stmt.Name, err)
		}
	}
	return nil
}

// macroDB is the shared database, for tests that only evaluate expressions.
//
// Do not write to it. If a test needs to insert a row or create a table, it
// needs freshDB instead: a stray row here is visible to every other test in
// the package and to none of their assertions.
func macroDB(t *testing.T) *sql.DB {
	t.Helper()
	return sharedDB
}

// freshDB is a private database for a test that writes.
//
// Two tests in this package do: one inserts into attributes and resources to
// exercise the orphan sweep, the other creates a table called `t`. Both would
// be visible to unrelated tests on the shared handle, and `t` would collide
// outright with a second test doing the same.
func freshDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatalf("opening duckdb: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := install(db); err != nil {
		t.Fatal(err)
	}
	return db
}
