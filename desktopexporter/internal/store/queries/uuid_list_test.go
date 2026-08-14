package queries_test

import (
	"database/sql"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// uuid_list is how every delete path turns one bound argument into a set of
// uuids, so it has to accept both id spellings the API deals in -- the dashed
// form the store renders and the dashless wire form the JSON-RPC layer serves
// -- and reject anything that is neither.
//
// Lives here rather than in util because the macro is created by this package's
// DDL; the util version silently tested nothing once the idiom moved out of a
// Go string and into SQL.
// The uuid placeholder contract, pinned against a real database because the
// interesting behaviour is DuckDB's, not ours: both id spellings the API serves
// must delete, and a malformed id must raise rather than report a successful
// no-op.
func TestUUIDListRejectsMalformedIDs(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	require.NoError(t, err)
	defer db.Close()
	for _, stmt := range queries.Macros() {
		_, err := db.Exec(stmt.SQL)
		require.NoErrorf(t, err, "%s", stmt.Name)
	}
	_, err = db.Exec(`create table t (id uuid)`)
	require.NoError(t, err)
	_, err = db.Exec(`insert into t values ('720cea1f-6c57-2438-d9b1-098fa86ecc3b')`)
	require.NoError(t, err)

	query := `select count(*) from t where id in (select id from uuid_list(?))`

	for _, valid := range []string{
		"720cea1f-6c57-2438-d9b1-098fa86ecc3b", // dashed
		"720cea1f6c572438d9b1098fa86ecc3b",     // the wire form the API serves
	} {
		var n int
		require.NoError(t, db.QueryRow(query, []string{valid}).Scan(&n), "id %q", valid)
		assert.Equal(t, 1, n, "id %q must match the stored row", valid)
	}

	for _, bad := range []string{"not-a-uuid", ""} {
		var n int
		err := db.QueryRow(query, []string{bad}).Scan(&n)
		assert.Error(t, err,
			"a malformed id (%q) must raise, not silently match nothing -- a delete "+
				"reporting success having removed nothing is worse than an error", bad)
	}
}

// body_preview truncates a log body for the summary card, and the corpus does
// not test it: real log bodies in the reference capture top out at 166
// characters, so the truncation never fires and a macro that returned the body
// untouched would pass every end-to-end check.
func TestBodyPreviewTruncates(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	require.NoError(t, err)
	defer db.Close()
	for _, stmt := range queries.Macros() {
		_, err := db.Exec(stmt.SQL)
		require.NoErrorf(t, err, "%s", stmt.Name)
	}

	var long, short int
	var emptyOK, nullOK bool
	require.NoError(t, db.QueryRow(`
		select length(body_preview(repeat('x', 500))),
		       length(body_preview(repeat('y', 50))),
		       body_preview('') = '',
		       body_preview(NULL) is null
	`).Scan(&long, &short, &emptyOK, &nullOK))

	assert.Equal(t, 300, long, "a long body must be cut to the preview length")
	assert.Equal(t, 50, short, "a short body must pass through untouched")
	assert.True(t, emptyOK, "empty body must stay empty, not become NULL")
	assert.True(t, nullOK, "NULL body must stay NULL, not become an empty string")
}
