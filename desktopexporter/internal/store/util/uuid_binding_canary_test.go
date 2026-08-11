package util

import (
	"database/sql"
	"testing"

	"github.com/duckdb/duckdb-go/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// UUIDList casts twice -- bind as varchar[], convert to uuid in SQL -- because
// the driver cannot bind a uuid list directly. This pins that limitation so we
// find out when it lifts, rather than carrying the workaround forever on the
// strength of a comment nobody re-checks.
//
// If this test starts failing, that is good news: read the failure message. It
// means the driver has gained the behaviour we wanted, and UUIDList can drop to
// `select unnest(?::uuid[])`.
//
// Pinning current behaviour is normally an anti-pattern, since it cements
// whatever the code happens to do. It earns its place here because the
// behaviour being pinned is a *third-party* limitation we route around, and
// because one half of it fails silently -- which is the half that would
// otherwise cost someone a long afternoon.
func TestDriverStillCannotBindUUIDLists(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`create table t (id uuid)`)
	require.NoError(t, err)
	const id = "11111111-1111-1111-1111-111111111111"
	_, err = db.Exec(`insert into t values (?::uuid)`, id)
	require.NoError(t, err)

	t.Run("strings cannot bind as uuid[]", func(t *testing.T) {
		var n int
		err := db.QueryRow(`select count(*) from t where id in (select unnest(?::uuid[]))`,
			[]string{id}).Scan(&n)
		require.Error(t, err,
			"driver now binds []string as uuid[]: UUIDList can drop the varchar hop")
	})

	t.Run("duckdb.UUID binds but matches nothing", func(t *testing.T) {
		u := duckdb.UUID(uuid.MustParse(id))
		var n int
		err := db.QueryRow(`select count(*) from t where id in (select unnest(?::uuid[]))`,
			[]duckdb.UUID{u}).Scan(&n)
		require.NoError(t, err, "this form has always bound without error")
		assert.Equal(t, 0, n,
			"driver now round-trips duckdb.UUID as a parameter correctly: "+
				"UUIDList can bind []duckdb.UUID directly and drop both casts")

		// The silent part, stated outright: the value arrives altered, which is
		// why the failure is zero rows rather than an error.
		var got string
		require.NoError(t, db.QueryRow(`select unnest(?::uuid[])::varchar`,
			[]duckdb.UUID{u}).Scan(&got))
		assert.NotEqual(t, id, got,
			"duckdb.UUID now survives parameter binding intact; see above")
	})

	t.Run("the form we ship works", func(t *testing.T) {
		var n int
		require.NoError(t, db.QueryRow(
			`select count(*) from t where id in (select unnest(?::varchar[])::uuid)`,
			[]string{id}).Scan(&n))
		assert.Equal(t, 1, n, "the shipped form must match the stored row")
	})
}
