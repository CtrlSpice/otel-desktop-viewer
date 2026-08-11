package util

import (
	"database/sql"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCamelToSnake(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"single capital after lowercase", "traceID", "trace_id"},
		{"consecutive capitals", "traceIDField", "trace_idfield"},
		{"scope name", "scopeName", "scope_name"},
		{"PascalCase", "ScopeVersion", "scope_version"},
		{"all lowercase", "name", "name"},
		{"digit before capital", "value2Type", "value2_type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CamelToSnake(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

// The uuid placeholder contract, pinned against a real database because the
// interesting behaviour is DuckDB's, not ours: both id spellings the API serves
// must delete, and a malformed id must raise rather than report a successful
// no-op.
func TestUUIDPlaceholdersRejectMalformedIDs(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec(`create table t (id uuid)`)
	require.NoError(t, err)
	_, err = db.Exec(`insert into t values ('720cea1f-6c57-2438-d9b1-098fa86ecc3b')`)
	require.NoError(t, err)

	query := `select count(*) from t where id in (` + BuildUUIDPlaceholders(1) + `)`

	for _, valid := range []string{
		"720cea1f-6c57-2438-d9b1-098fa86ecc3b", // dashed
		"720cea1f6c572438d9b1098fa86ecc3b",     // the wire form the API serves
	} {
		var n int
		require.NoError(t, db.QueryRow(query, valid).Scan(&n), "id %q", valid)
		assert.Equal(t, 1, n, "id %q must match the stored row", valid)
	}

	for _, bad := range []string{"not-a-uuid", ""} {
		var n int
		err := db.QueryRow(query, bad).Scan(&n)
		assert.Error(t, err,
			"a malformed id (%q) must raise, not silently match nothing -- a delete "+
				"reporting success having removed nothing is worse than an error", bad)
	}
}
