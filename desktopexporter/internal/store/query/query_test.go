package query_test

import (
	"context"
	"database/sql"
	"math"
	"path/filepath"
	"testing"

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

func TestExecutePreservesExactScalarAndNestedValues(t *testing.T) {
	withConnection(t, "", func(conn *sql.Conn) {
		result, err := query.Execute(context.Background(), conn, `select
          9007199254740993::bigint as signed,
          18446744073709551615::ubigint as unsigned,
          123.4500::decimal(10,4) as amount,
          '-0.0'::double as signed_zero,
          json('{"n":9007199254740993,"k":1,"k":2}') as payload,
          [9007199254740993::bigint, null] as values,
          {'amount': 123.4500::decimal(10,4), 'values': [1::bigint, 2::bigint]} as record,
          map(['large'], [9007199254740993::bigint]) as mapping`, 25)
		require.NoError(t, err)
		require.Len(t, result.Rows, 1)
		assert.Equal(t, []any{
			"9007199254740993",
			"18446744073709551615",
			"123.4500",
			"0x8000000000000000",
			`{"n":9007199254740993,"k":1,"k":2}`,
			[]any{"9007199254740993", nil},
			map[string]any{"amount": "123.4500", "values": []any{"1", "2"}},
			[]query.MapEntry{{Key: "large", Value: "9007199254740993"}},
		}, result.Rows[0])
		assert.Equal(t, "json-text", result.Columns[4].Encoding)
		assert.Equal(t, "recursive", result.Columns[5].Encoding)
	})
}

func TestExecuteRejectsOnlyNestedShapesWithoutAnApprovedWireContract(t *testing.T) {
	withConnection(t, "", func(conn *sql.Conn) {
		for _, statement := range []string{
			"select map([1::integer], ['one'])",
			"select union_value(number := 1::bigint)",
			"select interval '1 month 2 days 3 microseconds'",
		} {
			_, err := query.Execute(context.Background(), conn, statement, 25)
			assert.ErrorIs(t, err, query.ErrUnsupportedType, statement)
		}
	})
}

func TestExecutePreservesNonFiniteFloatBits(t *testing.T) {
	withConnection(t, "", func(conn *sql.Conn) {
		_, err := conn.ExecContext(context.Background(), "create table query_float_fixture(position integer, value double)")
		require.NoError(t, err)
		for i, value := range []float64{1.25, math.Copysign(0, -1), math.Inf(1), math.Float64frombits(0x7ff8000000000001)} {
			_, err = conn.ExecContext(context.Background(), "insert into query_float_fixture values (?, ?)", i, value)
			require.NoError(t, err)
		}

		result, err := query.Execute(context.Background(), conn,
			"select value from query_float_fixture order by position", 25)
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

		for _, tc := range []struct {
			name      string
			statement string
			limit     uint64
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
				assert.Equal(t, tc.truncated, result.Truncated)
			})
		}

		result, err := query.Execute(context.Background(), conn,
			`select json('{"exact":9007199254740993}') as repeated, json('{"second":2}') as repeated`, 25)
		require.NoError(t, err)
		assert.Equal(t, []string{"repeated", "repeated"}, []string{result.Columns[0].Name, result.Columns[1].Name})
		assert.Equal(t, []any{`{"exact":9007199254740993}`, `{"second":2}`}, result.Rows[0])
	})
}

func TestExecuteReadOnlyFailureLeavesConnectionReusable(t *testing.T) {
	withConnection(t, "", func(conn *sql.Conn) {
		for _, statement := range []string{
			"create table nope(i integer)",
			"delete from spans",
			"select 1; select 2",
			"copy spans to '/tmp/query-test.csv'",
		} {
			_, err := query.Execute(context.Background(), conn, statement, 25)
			assert.ErrorIs(t, err, query.ErrReadOnly, statement)
		}

		_, err := conn.ExecContext(context.Background(), "create sequence query_test_sequence")
		require.NoError(t, err)
		_, err = query.Execute(context.Background(), conn, "select nextval('query_test_sequence')", 25)
		require.Error(t, err)

		result, err := query.Execute(context.Background(), conn, "select 42::integer as answer", 25)
		require.NoError(t, err)
		assert.Equal(t, "42", result.Rows[0][0])
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
