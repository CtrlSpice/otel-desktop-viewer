package queries_test

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/stretchr/testify/require"
)

// Every DDL statement must actually execute. Compiling proves only that the
// strings are valid Go.
func TestAllDDLExecutes(t *testing.T) {
	db := freshDB(t)

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

func TestSpanIDColumnTypesAndNullability(t *testing.T) {
	db := freshDB(t)

	tests := []struct {
		table, column, dataType, nullable string
	}{
		{"spans", "trace_id", "UUID", "NO"},
		{"spans", "span_id", "UBIGINT", "NO"},
		{"spans", "parent_span_id", "UBIGINT", "YES"},
		{"events", "id", "UUID", "NO"},
		{"events", "trace_id", "UUID", "NO"},
		{"events", "span_id", "UBIGINT", "NO"},
		{"links", "id", "UUID", "NO"},
		{"links", "trace_id", "UUID", "NO"},
		{"links", "span_id", "UBIGINT", "NO"},
		{"links", "linked_trace_id", "UUID", "YES"},
		{"links", "linked_span_id", "UBIGINT", "YES"},
		{"logs", "id", "UUID", "NO"},
		{"logs", "trace_id", "UUID", "YES"},
		{"logs", "span_id", "UBIGINT", "YES"},
		{"exemplars", "id", "UUID", "NO"},
		{"exemplars", "datapoint_id", "UUID", "NO"},
		{"exemplars", "trace_id", "UUID", "YES"},
		{"exemplars", "span_id", "UBIGINT", "YES"},
	}
	for _, tc := range tests {
		t.Run(tc.table+"/"+tc.column, func(t *testing.T) {
			var dataType, nullable string
			err := db.QueryRow(`
				select data_type, is_nullable
				from information_schema.columns
				where table_name = ? and column_name = ?`, tc.table, tc.column).
				Scan(&dataType, &nullable)
			require.NoError(t, err)
			require.Equal(t, tc.dataType, dataType)
			require.Equal(t, tc.nullable, nullable)
		})
	}
}

func TestSpanIDWireMacro(t *testing.T) {
	db := macroDB(t)

	for _, tc := range []struct {
		name, literal, want string
	}{
		{"leading zeros", "1", "0000000000000001"},
		{"high bit", "9223372036854775808", "8000000000000000"},
		{"max uint64", "18446744073709551615", "ffffffffffffffff"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			require.NoError(t, db.QueryRow(
				"select span_id_wire("+tc.literal+"::ubigint)").Scan(&got))
			require.Equal(t, tc.want, got)
		})
	}

	var null sql.NullString
	require.NoError(t, db.QueryRow(`select span_id_wire(null::ubigint)`).Scan(&null))
	require.False(t, null.Valid)
}

// The wire format keys resources and scopes by seq, so the sequence defaults
// have to actually fire on insert.
func TestSequencesAssignShortKeys(t *testing.T) {
	db := freshDB(t)

	_, err := db.Exec(`insert into attributes values
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

// Cross-signal references must stay unconstrained.
//
// logs and exemplars carry trace_id / span_id, and links carries the
// linked_trace_id / linked_span_id pair it points at. None of them may become a
// foreign key: signals
// arrive independently and out of order, so the span a log names may arrive
// later, be dropped by sampling, or never be sent. An FK would reject that
// telemetry on arrival -- most often for partial or failed traces, which is
// precisely what someone opens this tool to look at.
//
// No ingest test would catch it either: they all write complete traces, where
// the referenced span happens to exist. So the guard has to be on the DDL.
func TestCrossSignalReferencesAreNotForeignKeys(t *testing.T) {
	for _, tc := range []struct{ file, column string }{
		{"logs.sql", "trace_id"},
		{"logs.sql", "span_id"},
		{"exemplars.sql", "trace_id"},
		{"exemplars.sql", "span_id"},
		{"links.sql", "linked_trace_id"},
		{"links.sql", "linked_span_id"},
	} {
		t.Run(tc.file+"/"+tc.column, func(t *testing.T) {
			var ddl string
			for _, stmt := range queries.Tables() {
				if strings.HasSuffix(stmt.Name, tc.file) {
					ddl = stmt.SQL
				}
			}
			require.NotEmpty(t, ddl, "table file not found: %s", tc.file)
			require.NotContains(t, ddl, "foreign key ("+tc.column+")",
				"%s.%s must stay unconstrained: signals arrive out of order, and an FK "+
					"would reject logs and exemplars whose span has not arrived yet",
				tc.file, tc.column)
		})
	}
}
