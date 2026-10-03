package query_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func withConnection(t *testing.T, dbPath string, fn func(*sql.Conn)) {
	t.Helper()
	viewerStore, err := store.NewStore(context.Background(), dbPath, zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, viewerStore.Close()) })
	require.NoError(t, viewerStore.WithDBRead(func(db *sql.DB) error {
		conn, err := db.Conn(context.Background())
		require.NoError(t, err)
		defer conn.Close()
		fn(conn)
		return nil
	}))
}

func TestExecutePreservesExactScalarValues(t *testing.T) {
	withConnection(t, "", func(conn *sql.Conn) {
		result, err := query.Execute(context.Background(), conn, `select
          9007199254740993::bigint as repeated,
          18446744073709551615::ubigint as repeated,
          123.4500::decimal(10,4) as amount,
          null::varchar as absent,
          ''::varchar as empty,
          false as zero,
          '00112233-4455-6677-8899-aabbccddeeff'::uuid as id,
          from_hex('00ff') as bytes,
          '-0.0'::double as signed_zero`, 25)
		require.NoError(t, err)
		require.Len(t, result.Rows, 1)
		assert.Equal(t, []string{"repeated", "repeated", "amount", "absent", "empty", "zero", "id", "bytes", "signed_zero"}, columnNames(result.Columns))
		assert.Equal(t, []any{
			"9007199254740993", "18446744073709551615", "123.4500", nil, "", false,
			"00112233-4455-6677-8899-aabbccddeeff", "AP8=", "0x8000000000000000",
		}, result.Rows[0])
	})
}

func TestExecutePreservesDoubleWireConvention(t *testing.T) {
	withConnection(t, "", func(conn *sql.Conn) {
		_, err := conn.ExecContext(context.Background(), "create table query_double_fixture(position integer, value double)")
		require.NoError(t, err)
		values := []float64{
			1.25,
			math.Copysign(0, -1),
			math.Inf(1),
			math.Float64frombits(0x7ff8000000000001),
		}
		for i, value := range values {
			_, err = conn.ExecContext(context.Background(), "insert into query_double_fixture values (?, ?)", i, value)
			require.NoError(t, err)
		}
		result, err := query.Execute(context.Background(), conn,
			"select value from query_double_fixture order by position", 25)
		require.NoError(t, err)
		assert.Equal(t, [][]any{
			{1.25},
			{"0x8000000000000000"},
			{"0x7ff0000000000000"},
			{"0x7ff8000000000001"},
		}, result.Rows)
	})
}

func TestExecuteLimitsAndIntrospection(t *testing.T) {
	withConnection(t, "", func(conn *sql.Conn) {
		for _, statement := range []string{"SHOW TABLES", "DESCRIBE spans"} {
			result, err := query.Execute(context.Background(), conn, statement, 25)
			require.NoError(t, err, statement)
			assert.NotEmpty(t, result.Columns, statement)
		}
		macroResult, err := query.Execute(context.Background(), conn, `select
          span_id_wire(1::ubigint) as span_id,
          attribute_int64('{"kind":"int64","value":"9007199254740993"}'::json) as exact_attribute`, 25)
		require.NoError(t, err)
		assert.Equal(t, []any{"0000000000000001", "9007199254740993"}, macroResult.Rows[0])

		for _, tc := range []struct {
			name      string
			statement string
			limit     int
			rows      int
			truncated bool
		}{
			{"zero with output", "select * from range(1)", 0, 0, true},
			{"exact default", "select * from range(25)", 25, 25, false},
			{"default lookahead", "select * from range(26)", 25, 25, true},
			{"explicit SQL limit", "select * from range(100) limit 3", 25, 3, false},
			{"empty", "select * from range(0)", 25, 0, false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				result, err := query.Execute(context.Background(), conn, tc.statement, tc.limit)
				require.NoError(t, err)
				assert.Len(t, result.Rows, tc.rows)
				assert.Equal(t, tc.rows, result.RowCount)
				assert.Equal(t, tc.truncated, result.Truncated)
			})
		}
	})
}

func TestExecuteRejectsUnsafeAndLossyResults(t *testing.T) {
	withConnection(t, "", func(conn *sql.Conn) {
		for _, statement := range []string{
			"create table nope(i integer)",
			"insert into spans by name select 1",
			"begin transaction",
			"select 1; select 2",
			"PRAGMA version",
			"attach ':memory:' as other",
			"copy spans to '/tmp/query-test.csv'",
			"install json",
			"load json",
			"set threads = 1",
			"call checkpoint()",
			"explain select 1",
		} {
			_, err := query.Execute(context.Background(), conn, statement, 25)
			assert.ErrorIs(t, err, query.ErrReadOnly, statement)
		}

		for _, statement := range []string{
			`select json('{"n":9007199254740993,"k":1,"k":2}') as payload`,
			"select [9007199254740993::bigint] as values",
			"select {'value': 1::bigint} as value",
		} {
			_, err := query.Execute(context.Background(), conn, statement, 25)
			assert.ErrorIs(t, err, query.ErrUnsupportedType, statement)
		}

		result, err := query.Execute(context.Background(), conn,
			`select json('{"n":9007199254740993,"k":1,"k":2}')::varchar as payload`, 25)
		require.NoError(t, err)
		assert.Equal(t, `{"n":9007199254740993,"k":1,"k":2}`, result.Rows[0][0])
	})
}

func TestExecuteReadOnlyTransactionAndConnectionRecovery(t *testing.T) {
	withConnection(t, "", func(conn *sql.Conn) {
		_, err := conn.ExecContext(context.Background(), "create sequence query_test_sequence")
		require.NoError(t, err)

		_, err = query.Execute(context.Background(), conn, "select nextval('query_test_sequence')", 25)
		require.Error(t, err)

		result, err := query.Execute(context.Background(), conn, "select 42::integer as answer", 25)
		require.NoError(t, err)
		assert.Equal(t, "42", result.Rows[0][0])
	})
}

func TestExecuteCancellationLeavesConnectionReusable(t *testing.T) {
	withConnection(t, "", func(conn *sql.Conn) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		_, err := query.Execute(ctx, conn, "select sum(i) from range(1000000000) t(i)", 25)
		require.Error(t, err)
		assert.True(t, errors.Is(err, context.DeadlineExceeded), "%v", err)

		result, err := query.Execute(context.Background(), conn, "select 1::integer", 25)
		require.NoError(t, err)
		assert.Equal(t, "1", result.Rows[0][0])
	})
}

func TestExecuteReadsBothStoreModes(t *testing.T) {
	for _, tc := range []struct {
		name string
		path func(*testing.T) string
	}{
		{"in-memory", func(*testing.T) string { return "" }},
		{"persistent", func(t *testing.T) string { return filepath.Join(t.TempDir(), "viewer.db") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withConnection(t, tc.path(t), func(conn *sql.Conn) {
				result, err := query.Execute(context.Background(), conn,
					"select count(*)::bigint as span_count from spans", 25)
				require.NoError(t, err)
				assert.Equal(t, "0", result.Rows[0][0])
			})
		})
	}
}

func TestExecuteRejectsOversizedResult(t *testing.T) {
	withConnection(t, "", func(conn *sql.Conn) {
		_, err := query.Execute(context.Background(), conn,
			fmt.Sprintf("select repeat('x', %d)", query.MaxBytes), 25)
		assert.ErrorIs(t, err, query.ErrResultTooLarge)
	})
}

func columnNames(columns []query.Column) []string {
	names := make([]string, len(columns))
	for i, column := range columns {
		names[i] = column.Name
	}
	return names
}
