package commitbridge

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"testing"

	"github.com/duckdb/duckdb-go/v2"
	"github.com/stretchr/testify/require"
)

func TestCommittedWrites(t *testing.T) {
	observer := New()
	t.Cleanup(observer.Close)
	connector, err := duckdb.NewConnector("", observer.Init)
	require.NoError(t, err)
	db := sql.OpenDB(connector)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	exec := func(query string) {
		t.Helper()
		_, err := db.Exec(query)
		require.NoError(t, err)
	}
	exec("create table records (id integer primary key)")
	before, changed, _ := observer.Snapshot()
	exec("insert into records values (1)")
	after, _, _ := observer.Snapshot()
	require.Equal(t, before+1, after)
	select {
	case <-changed:
	default:
		t.Fatal("successful write did not notify")
	}
	tx, err := db.Begin()
	require.NoError(t, err)
	_, err = tx.Exec("insert into records values (10), (11)")
	require.NoError(t, err)
	uncommitted, _, _ := observer.Snapshot()
	require.Equal(t, after, uncommitted)
	require.NoError(t, tx.Commit())
	after++
	committed, _, _ := observer.Snapshot()
	require.Equal(t, after, committed)

	// SELECTs must not create an SSE -> query -> SSE refresh loop.
	exec("select * from records")
	tx, err = db.Begin()
	require.NoError(t, err)
	_, err = tx.Exec("select * from records")
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	revision, _, _ := observer.Snapshot()
	require.Equal(t, after, revision)

	tx, err = db.Begin()
	require.NoError(t, err)
	_, err = tx.Exec("insert into records values (2)")
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())
	_, err = db.Exec("insert into records values (1)")
	require.Error(t, err)
	revision, _, _ = observer.Snapshot()
	require.Equal(t, after, revision)

	// A new pool connection gets its own native state and the same observer.
	conn, err := db.Conn(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	exec("delete from records where id = 1")
	revision, _, _ = observer.Snapshot()
	require.Equal(t, after+1, revision)
	_, err = conn.ExecContext(context.Background(), "insert into records values (3)")
	require.NoError(t, err)
	revision, _, _ = observer.Snapshot()
	require.Equal(t, after+2, revision)

	// Cleanup remains safe even if a native connection outlives its observer.
	observer.Close()
	exec("delete from records")
	revision, closedChannel, closed := observer.Snapshot()
	require.True(t, closed)
	require.Equal(t, after+2, revision)
	<-closedChannel
}

func TestDedicatedConnectionAndIsolation(t *testing.T) {
	first, second := New(), New()
	defer first.Close()
	defer second.Close()
	for _, observer := range []*Observer{first, second} {
		connector, err := duckdb.NewConnector("", observer.Init)
		require.NoError(t, err)
		conn, err := connector.Connect(context.Background())
		require.NoError(t, err)
		_, err = conn.(driver.ExecerContext).ExecContext(context.Background(), "create table records (id integer)", nil)
		require.NoError(t, err)
		require.NoError(t, conn.Close())
		require.NoError(t, connector.Close())
	}
	one, _, _ := first.Snapshot()
	two, _, _ := second.Snapshot()
	require.Equal(t, uint64(1), one)
	require.Equal(t, one, two)
}
