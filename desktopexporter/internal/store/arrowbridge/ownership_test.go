package arrowbridge

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/query"
	"github.com/stretchr/testify/require"
)

func TestForeignConnectionsCannotConsumeInput(t *testing.T) {
	db, writer := openDB(t, "")
	_, err := db.ExecContext(t.Context(), createSQL)
	require.NoError(t, err)
	owner, err := ownerID(t.Context(), writer)
	require.NoError(t, err)
	record, alloc := batch(t, 4097)
	token, release, err := publish(t.Context(), writer, record)
	require.NoError(t, err)
	record.Release()
	defer func() { release(); alloc.AssertSize(t, 0) }()
	scan := fmt.Sprintf("otel_arrow_scan(%d::ubigint)", token)
	statement := "select count(*) from " + scan
	reader, err := db.Conn(t.Context())
	require.NoError(t, err)
	defer reader.Close()
	for _, initialized := range []bool{false, true} {
		if initialized {
			require.NoError(t, reader.Raw(func(raw any) error { return Load(t.Context(), raw.(driver.Conn)) }))
		}
		for _, sql := range []string{statement, "EXPLAIN " + statement, statement + " LIMIT 0"} {
			_, err := reader.ExecContext(t.Context(), sql)
			require.ErrorContains(t, err, "different connection")
		}
		prepared, err := reader.PrepareContext(t.Context(), statement)
		if prepared != nil {
			require.NoError(t, prepared.Close())
		}
		require.ErrorContains(t, err, "different connection")
		// The same rejection applies through the submitted read-only query API.
		_, err = query.Execute(t.Context(), reader, statement, 25)
		require.ErrorContains(t, err, "different connection")
		_, err = query.Execute(t.Context(), reader, "select 1", 25)
		require.NoError(t, err)
	}
	_, err = reader.ExecContext(t.Context(), fmt.Sprintf("SET SESSION otel_arrow_owner = %d", owner))
	require.ErrorContains(t, err, "assigned by the bridge")
	_, err = reader.ExecContext(t.Context(), "SET GLOBAL otel_arrow_owner = 0")
	require.Error(t, err)
	for _, sql := range []string{"SET SESSION otel_arrow_owner = 0", "RESET otel_arrow_owner"} {
		_, err = reader.ExecContext(t.Context(), sql)
		require.NoError(t, err)
		_, err = reader.ExecContext(t.Context(), statement)
		require.ErrorContains(t, err, "different connection")
	}
	// A different database shares the process registry, but not the batch owner.
	_, otherWriter := openDB(t, "")
	_, err = otherWriter.(driver.ExecerContext).ExecContext(t.Context(), statement, nil)
	require.ErrorContains(t, err, "different connection")
	_, err = writer.(driver.ExecerContext).ExecContext(t.Context(), strings.Replace(insertSQL, "appended_data", scan, 1), nil)
	require.NoError(t, err)
	var count int
	require.NoError(t, db.QueryRowContext(t.Context(), "select count(*) from landed").Scan(&count))
	require.Equal(t, 4097, count)
	_, err = writer.(driver.ExecerContext).ExecContext(t.Context(), statement, nil)
	require.ErrorContains(t, err, "already consumed")
}

func TestPreparedInputRechecksOwnerAndLifetime(t *testing.T) {
	_, writer := openDB(t, "")
	for _, expire := range []bool{false, true} {
		record, alloc := batch(t, 4097)
		token, release, err := publish(t.Context(), writer, record)
		require.NoError(t, err)
		record.Release()
		statement := fmt.Sprintf("select count(*) from otel_arrow_scan(%d::ubigint)", token)
		prepared, err := writer.Prepare(statement)
		require.NoError(t, err)
		if expire {
			release()
		} else {
			_, err = writer.(driver.ExecerContext).ExecContext(t.Context(), "SET SESSION otel_arrow_owner = 0", nil)
			require.NoError(t, err)
		}
		_, err = prepared.(driver.StmtExecContext).ExecContext(t.Context(), nil)
		if expire {
			require.ErrorContains(t, err, "expired")
		} else {
			require.ErrorContains(t, err, "different connection")
		}
		require.NoError(t, prepared.Close())
		release()
		release()
		alloc.AssertSize(t, 0)
	}
	record, alloc := batch(t, 3)
	require.NoError(t, Insert(t.Context(), writer, "select * from appended_data", record))
	record.Release()
	alloc.AssertSize(t, 0)
}
