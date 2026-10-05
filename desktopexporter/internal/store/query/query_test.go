package query_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/query"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
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
		assertJSONEqual(t, `[
          {"kind":"int64","value":"9007199254740993"},
          {"kind":"string","value":"18446744073709551615"},
          {"kind":"string","value":"123.4500"},
          {"kind":"double","value":"0x8000000000000000"},
          {"kind":"string","value":"{\"n\":9007199254740993,\"k\":1,\"k\":2}"},
          {"kind":"array","value":[{"kind":"int64","value":"9007199254740993"},{"kind":"empty","value":null}]},
          {"kind":"map","value":[
            {"key":"amount","value":{"kind":"string","value":"123.4500"}},
            {"key":"values","value":{"kind":"array","value":[{"kind":"int64","value":"1"},{"kind":"int64","value":"2"}]}}
          ]},
          {"kind":"map","value":[
            {"key":{"kind":"string","value":"large"},"value":{"kind":"int64","value":"9007199254740993"}}
          ]}
        ]`, result.Rows[0])
	})
}

func TestExecuteReportsUnionAsTheRemainingRepresentationBlocker(t *testing.T) {
	withConnection(t, "", func(conn *sql.Conn) {
		_, err := query.Execute(context.Background(), conn, "select union_value(number := 1::bigint)", 25)
		assert.ErrorIs(t, err, query.ErrUnsupportedType)

		result, err := query.Execute(context.Background(), conn, "select interval '1 month 2 days 3 microseconds'", 25)
		require.NoError(t, err)
		assert.Equal(t, query.Value{Kind: "string", Value: "1 month 2 days 00:00:00.000003"}, result.Rows[0][0])
	})
}

func TestExecutePreservesNativeMapKeyTypesAndNestedValues(t *testing.T) {
	withConnection(t, "", func(conn *sql.Conn) {
		result, err := query.Execute(context.Background(), conn, `select
          map(
            [9007199254740993::bigint, '-9223372036854775808'::bigint],
            [[18446744073709551615::ubigint], [9007199254740993::ubigint]]
          ) as signed_keys,
          map(
            [18446744073709551615::ubigint],
            [map(['nested'], [123.4500::decimal(10,4)])]
          ) as unsigned_key,
          map([123.4500::decimal(10,4)], ['decimal key']) as decimal_key,
		  map([{'id': 9007199254740993::bigint}], ['struct key']) as struct_key,
		  map([json('{"n":9007199254740993,"k":1,"k":2}')], ['json key']) as json_key,
		  map([]::integer[], []::varchar[]) as empty_map,
		  null::map(integer, varchar) as null_map`, 25)
		require.NoError(t, err)
		require.Len(t, result.Rows, 1)
		assertJSONEqual(t, `[
          {"kind":"map","value":[
            {"key":{"kind":"int64","value":"9007199254740993"},"value":{"kind":"array","value":[{"kind":"string","value":"18446744073709551615"}]}},
            {"key":{"kind":"int64","value":"-9223372036854775808"},"value":{"kind":"array","value":[{"kind":"string","value":"9007199254740993"}]}}
          ]},
          {"kind":"map","value":[
            {"key":{"kind":"string","value":"18446744073709551615"},"value":{"kind":"map","value":[
              {"key":{"kind":"string","value":"nested"},"value":{"kind":"string","value":"123.4500"}}
            ]}}
          ]},
          {"kind":"map","value":[{"key":{"kind":"string","value":"123.4500"},"value":{"kind":"string","value":"decimal key"}}]},
          {"kind":"map","value":[{"key":{"kind":"map","value":[{"key":"id","value":{"kind":"int64","value":"9007199254740993"}}]},"value":{"kind":"string","value":"struct key"}}]},
          {"kind":"map","value":[{"key":{"kind":"string","value":"{\"n\":9007199254740993,\"k\":1,\"k\":2}"},"value":{"kind":"string","value":"json key"}}]},
          {"kind":"map","value":[]},
          {"kind":"empty","value":null}
        ]`, result.Rows[0])
		assert.Equal(t, "MAP(BIGINT, UBIGINT[])", result.Columns[0].DuckDBType)
		assert.Equal(t, "MAP(UBIGINT, MAP(VARCHAR, DECIMAL(10,4)))", result.Columns[1].DuckDBType)
		assert.Equal(t, "MAP(DECIMAL(10,4), VARCHAR)", result.Columns[2].DuckDBType)
		assert.Equal(t, `MAP(STRUCT(id BIGINT), VARCHAR)`, result.Columns[3].DuckDBType)
		assert.Equal(t, "MAP(JSON, VARCHAR)", result.Columns[4].DuckDBType)

		encoded, err := json.Marshal(result.Rows[0])
		require.NoError(t, err)
		assert.Contains(t, string(encoded), `"key":{"kind":"int64","value":"9007199254740993"}`)
		assert.Contains(t, string(encoded), `"key":{"kind":"map","value":[{"key":"id"`)
		assert.NotContains(t, string(encoded), `"9007199254740993":`)
	})
}

func TestExecutePreservesCompositeMapKeysAndJSONAtRecursivePositions(t *testing.T) {
	withConnection(t, "", func(conn *sql.Conn) {
		result, err := query.Execute(context.Background(), conn, `select
          map(
            ['outer'],
            [map(
              [{'id': 9007199254740993::bigint}],
              [json('{"n":9007199254740993,"k":1,"k":2}')]
            )]
          ) as nested_map,
          [map([{'id': 9007199254740993::bigint}], ['list value'])] as map_list,
          {'map field': map([{'id': 9007199254740993::bigint}], ['struct value'])} as map_struct`, 25)
		require.NoError(t, err)
		require.Len(t, result.Rows, 1)
		assertJSONEqual(t, `[
          {"kind":"map","value":[
            {"key":{"kind":"string","value":"outer"},"value":{"kind":"map","value":[
              {"key":{"kind":"map","value":[{"key":"id","value":{"kind":"int64","value":"9007199254740993"}}]},
               "value":{"kind":"string","value":"{\"n\":9007199254740993,\"k\":1,\"k\":2}"}}
            ]}}
          ]},
          {"kind":"array","value":[
            {"kind":"map","value":[
              {"key":{"kind":"map","value":[{"key":"id","value":{"kind":"int64","value":"9007199254740993"}}]},
               "value":{"kind":"string","value":"list value"}}
            ]}
          ]},
          {"kind":"map","value":[
            {"key":"map field","value":{"kind":"map","value":[
              {"key":{"kind":"map","value":[{"key":"id","value":{"kind":"int64","value":"9007199254740993"}}]},
               "value":{"kind":"string","value":"struct value"}}
            ]}}
          ]}
        ]`, result.Rows[0])
		assert.Equal(t, "MAP(VARCHAR, MAP(STRUCT(id BIGINT), JSON))", result.Columns[0].DuckDBType)
		assert.Equal(t, "MAP(STRUCT(id BIGINT), VARCHAR)[]", result.Columns[1].DuckDBType)
		assert.Equal(t, `STRUCT("map field" MAP(STRUCT(id BIGINT), VARCHAR))`, result.Columns[2].DuckDBType)
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
			{query.Value{Kind: "double", Value: 1.25}},
			{query.Value{Kind: "double", Value: "0x8000000000000000"}},
			{query.Value{Kind: "double", Value: "0x7ff0000000000000"}},
			{query.Value{Kind: "double", Value: "0x7ff8000000000001"}},
		}, result.Rows)
	})
}

func TestExecutePreservesTemporalInfinitiesAsText(t *testing.T) {
	withConnection(t, "", func(conn *sql.Conn) {
		result, err := query.Execute(context.Background(), conn,
			"select 'infinity'::timestamp, '-infinity'::date", 25)
		require.NoError(t, err)
		assert.Equal(t, []any{
			query.Value{Kind: "string", Value: "infinity"},
			query.Value{Kind: "string", Value: "-infinity"},
		}, result.Rows[0])
	})
}

func TestExecutePreservesStoredCanonicalTelemetryValues(t *testing.T) {
	withConnection(t, "", func(conn *sql.Conn) {
		value := pcommon.NewValueMap()
		value.Map().PutInt("large", math.MaxInt64)
		items := value.Map().PutEmptySlice("items")
		items.AppendEmpty().SetDouble(math.Float64frombits(0x7ff8000000000001))
		items.AppendEmpty().SetStr("text")
		encoded, err := util.EncodeValue(value)
		require.NoError(t, err)

		_, err = conn.ExecContext(context.Background(), `
          insert into attributes values
            ('00112233-4455-6677-8899-aabbccddeeff'::uuid, 'canonical', ?::json),
            ('10112233-4455-6677-8899-aabbccddeeff'::uuid, 'duplicate-entries',
             '{"kind":"map","value":[
               {"key":"same","value":{"kind":"int64","value":"1"}},
               {"key":"same","value":{"kind":"string","value":"1"}}
             ]}'::json)`, string(encoded))
		require.NoError(t, err)

		result, err := query.Execute(context.Background(), conn,
			"select key, value from attributes order by key", 25)
		require.NoError(t, err)
		require.Len(t, result.Rows, 2)
		assert.Equal(t, query.Value{Kind: "string", Value: "canonical"}, result.Rows[0][0])
		assert.JSONEq(t, string(encoded), mustJSON(t, result.Rows[0][1]))
		assert.JSONEq(t, `{"kind":"map","value":[
          {"key":"same","value":{"kind":"int64","value":"1"}},
          {"key":"same","value":{"kind":"string","value":"1"}}
        ]}`, mustJSON(t, result.Rows[1][1]))
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
		assert.Equal(t, []any{
			query.Value{Kind: "string", Value: `{"exact":9007199254740993}`},
			query.Value{Kind: "string", Value: `{"second":2}`},
		}, result.Rows[0])

		result, err = query.Execute(context.Background(), conn, "select 1::integer", ^uint64(0))
		require.NoError(t, err)
		assert.Equal(t, ^uint64(0), result.Limit)
		assert.False(t, result.Truncated)
		assert.Equal(t, query.Value{Kind: "int64", Value: "1"}, result.Rows[0][0])
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
		assert.Equal(t, query.Value{Kind: "int64", Value: "42"}, result.Rows[0][0])
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
		assert.Equal(t, query.Value{Kind: "int64", Value: "1"}, result.Rows[0][0])
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
				assert.Equal(t, query.Value{Kind: "int64", Value: "0"}, result.Rows[0][0])
			})
		})
	}
}

func assertJSONEqual(t *testing.T, want string, got any) {
	t.Helper()
	assert.JSONEq(t, want, mustJSON(t, got))
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return string(encoded)
}
