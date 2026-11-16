package server

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/exp/jsonrpc2"
)

func TestQueryUsesDefaultAndExplicitLimits(t *testing.T) {
	handler := setupHandler(t)

	result, err := handler.Handle(context.Background(), createRequest("query", []any{"select * from range(26)"}))
	require.NoError(t, err)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	var response struct {
		Rows      [][]json.RawMessage `json:"rows"`
		Truncated bool                `json:"truncated"`
	}
	require.NoError(t, json.Unmarshal(encoded, &response))
	assert.Len(t, response.Rows, 25)
	assert.True(t, response.Truncated)

	result, err = handler.Handle(context.Background(), createRequest("query", map[string]any{"sql": "SHOW TABLES", "limit": 2}))
	require.NoError(t, err)
	encoded, err = json.Marshal(result)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"columns":[`)
	assert.Contains(t, string(encoded), `"truncated":`)

	result, err = handler.Handle(context.Background(), createRequest("query", []any{"DESCRIBE spans"}))
	require.NoError(t, err)
	encoded, err = json.Marshal(result)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"column_name"`)
}

func TestQueryPreservesNativeIntegerAndTopLevelJSON(t *testing.T) {
	handler := setupHandler(t)
	result, err := handler.Handle(context.Background(), createRequest("query", []any{
		`select 9007199254740993::bigint as n, json('{"n":9007199254740993}') as payload`,
	}))
	require.NoError(t, err)

	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"rows":[[9007199254740993,{"n":9007199254740993}]]`)
}

func TestQueryReadsViewerTablesAndMacros(t *testing.T) {
	handler := setupHandlerWithData(t)

	result, err := handler.Handle(context.Background(), createRequest("query", []any{`
		select l.event_name,
		       body_preview(otel_value_json(l.body)) as preview,
		       trace_id_wire(l.trace_id) as trace_id,
		       span_id_wire(l.span_id) as span_id,
		       attribute_document(l.resource.attributes) as resource_attributes
		from logs l
		where l.service_name = 'pumpkin.pie'`,
	}))
	require.NoError(t, err)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)

	var response struct {
		Rows [][]json.RawMessage `json:"rows"`
	}
	require.NoError(t, json.Unmarshal(encoded, &response))
	require.Len(t, response.Rows, 1)
	require.Len(t, response.Rows[0], 5)
	assert.JSONEq(t, `"request.failed"`, string(response.Rows[0][0]))
	assert.JSONEq(t, `"test log message"`, string(response.Rows[0][1]))
	assert.JSONEq(t, `"00000000000000000000000000000001"`, string(response.Rows[0][2]))
	assert.JSONEq(t, `"0000000000000001"`, string(response.Rows[0][3]))
	assert.Contains(t, string(response.Rows[0][4]), `"key":"service.name"`)
	assert.Contains(t, string(response.Rows[0][4]), `"value":{"kind":"string","value":"pumpkin.pie"}`)

	result, err = handler.Handle(context.Background(), createRequest("query", map[string]any{
		"sql": `select count(*)::bigint as matched
			from spans s
			join logs l on l.trace_id = s.trace_id
			where s.service_name = 'pumpkin.pie'`,
	}))
	require.NoError(t, err)
	encoded, err = json.Marshal(result)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"columns":[{"name":"matched","type":"BIGINT"}],
		"rows":[[1]],
		"truncated":false
	}`, string(encoded))
}

func TestQueryPublicResponseCoversLimitBoundariesAndDuplicateColumns(t *testing.T) {
	handler := setupHandler(t)
	for _, tc := range []struct {
		name   string
		params any
		want   string
	}{
		{name: "zero", params: []any{"select 1", 0}, want: `{"columns":[{"name":"1","type":"INTEGER"}],"rows":[],"truncated":true}`},
		{name: "maximum", params: map[string]any{"sql": "select 1", "limit": uint64(math.MaxUint64)}, want: `{"columns":[{"name":"1","type":"INTEGER"}],"rows":[[1]],"truncated":false}`},
		{name: "empty duplicate columns", params: []any{"select 1 as duplicate, 2 as duplicate where false"}, want: `{"columns":[{"name":"duplicate","type":"INTEGER"},{"name":"duplicate","type":"INTEGER"}],"rows":[],"truncated":false}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := handler.Handle(context.Background(), createRequest("query", tc.params))
			require.NoError(t, err)
			encoded, err := json.Marshal(result)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(encoded))
		})
	}
}

func TestQueryPublicCancellationLeavesStoreReusable(t *testing.T) {
	handler := setupHandler(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := handler.Handle(ctx, createRequest("query", []any{
		"select sum(i) from range(1000000000) input(i)",
	}))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRequestCanceled)

	result, err := handler.Handle(context.Background(), createRequest("query", []any{"select 1"}))
	require.NoError(t, err)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"rows":[[1]]`)
}

func TestQueryPreparationCancellationUsesRequestCanceled(t *testing.T) {
	handler := setupHandler(t)
	statement := "SELECT " + strings.Repeat("1,", 100_000) + "1"
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	result, err := handler.Handle(ctx, createRequest("query", []any{statement}))

	assert.Nil(t, result)
	assert.Equal(t, ErrRequestCanceled, err)
	assert.False(t, errors.Is(err, jsonrpc2.ErrInvalidParams), "%v", err)
}

func TestQueryExplicitCancellationLeavesStoreReusable(t *testing.T) {
	handler := setupHandler(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	timer := time.AfterFunc(100*time.Millisecond, cancel)
	defer timer.Stop()

	_, err := handler.Handle(ctx, createRequest("query", []any{
		"select sum(i) from range(1000000000) input(i)",
	}))
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	require.ErrorIs(t, err, ErrRequestCanceled)
	assert.NotErrorIs(t, err, ErrInvalidQuery)

	result, err := handler.Handle(context.Background(), createRequest("query", []any{"select 1"}))
	require.NoError(t, err)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"rows":[[1]]`)
}

func TestQueryPublicReadsReopenedPersistentStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "viewer.db")
	initial, err := store.NewStore(context.Background(), path, zap.NewNop())
	require.NoError(t, err)
	require.NoError(t, initial.Close())

	reopened, err := store.NewStore(context.Background(), path, zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	handler := NewJSONRPCHandler(reopened, zap.NewNop())
	result, err := handler.Handle(context.Background(), createRequest("query", map[string]any{
		"sql": "select count(*)::bigint as span_count from spans",
	}))
	require.NoError(t, err)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"rows":[[0]]`)
}

func TestQueryRejectsInvalidParametersAndWrites(t *testing.T) {
	handler := setupHandler(t)
	for _, params := range []any{
		[]any{},
		[]any{"select 1", 1, 2},
		[]any{1},
		[]any{"select 1", nil},
		[]any{"select 1", -1},
		[]any{"select 1", 1.5},
		[]any{"delete from spans"},
		[]any{"select 1; select 2"},
	} {
		_, err := handler.Handle(context.Background(), createRequest("query", params))
		assert.ErrorIs(t, err, jsonrpc2.ErrInvalidParams, "params %#v: %v", params, err)
	}

	_, err := handler.Handle(context.Background(), createRequest("query", []any{
		"select cast(value as integer) from (values ('failed')) input(value)",
	}))
	assert.ErrorIs(t, err, ErrInvalidQuery)
}

func TestQueryRejectedStatementsPreserveStoredTelemetry(t *testing.T) {
	handler := setupHandler(t)
	require.NoError(t, handler.store.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(t.Context(), conn, buildTestTraces())
	}))
	for _, statement := range []string{
		"delete from spans; select 1",
		"commit; delete from spans; select 1",
		"rollback; drop table spans; select 1",
	} {
		_, err := handler.Handle(t.Context(), createRequest("query", []any{statement}))
		require.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)
		result, err := handler.Handle(t.Context(), createRequest("query", []any{"select count(*) from spans"}))
		require.NoError(t, err)
		encoded, err := json.Marshal(result)
		require.NoError(t, err)
		assert.Contains(t, string(encoded), `"rows":[[1]]`, statement)
	}
}

func TestParseQueryParams(t *testing.T) {
	for _, tc := range []struct {
		name          string
		raw           string
		wantStatement string
		wantLimit     uint64
		wantError     string
	}{
		{name: "default limit", raw: `["select 1"]`, wantStatement: "select 1", wantLimit: 25},
		{name: "explicit maximum", raw: `["select 1",18446744073709551615]`, wantStatement: "select 1", wantLimit: math.MaxUint64},
		{name: "wrong count", raw: `[]`, wantError: "invalid params"},
		{name: "statement type", raw: `[1]`, wantError: "sql must be a string"},
		{name: "limit type", raw: `["select 1",null]`, wantError: "limit must be a non-negative whole number"},
		{name: "negative limit", raw: `["select 1",-1]`, wantError: "limit must be a non-negative whole number"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseQueryParams(json.RawMessage(tc.raw))
			if tc.wantError != "" {
				require.Error(t, err)
				assert.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)
				assert.Contains(t, err.Error(), tc.wantError)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantStatement, got.statement)
			assert.Equal(t, tc.wantLimit, got.limit)
		})
	}
}
