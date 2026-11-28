package server

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap/zaptest"
	"golang.org/x/exp/jsonrpc2"
)

// Discovery finds scoped keys first, then samples a selected exact field.
func TestSearchAttributeMatches(t *testing.T) {
	handler := setupHandlerWithData(t)
	call := func(t *testing.T, handler *JSONRPCHandler, params any) []map[string]any {
		t.Helper()
		result, err := handler.Handle(context.Background(), createRequest("searchAttributeMatches", params))
		require.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		require.True(t, ok, "expected json.RawMessage, got %T", result)
		var out []map[string]any
		require.NoError(t, json.Unmarshal(raw, &out))
		return out
	}

	t.Run("finds keys then selected values", func(t *testing.T) {
		assert.Empty(t, call(t, handler, []string{"pumpkin"}))
		keys := call(t, handler, map[string]any{"term": "service", "signal": "traces"})
		require.NotEmpty(t, keys)
		for _, key := range keys {
			assert.Empty(t, key["sampleValues"])
		}
		got := call(t, handler, map[string]any{"term": "pumpkin", "signal": "traces", "owner": "resource", "path": []any{"attributes", "service.name"}, "type": "string"})
		require.NotEmpty(t, got, "a selected value present in the fixture must be found")

		var found map[string]any
		for _, m := range got {
			if m["name"] == "service.name" {
				found = m
			}
		}
		require.NotNil(t, found, "service.name should be among the matches")
		assert.Equal(t, "resource", found["attributeScope"])
		assert.Contains(t, found["sampleValues"], map[string]any{"kind": "string", "value": "pumpkin.pie"})
	})

	t.Run("no match and empty term return an empty list", func(t *testing.T) {
		assert.Empty(t, call(t, handler, []string{"no-such-text-anywhere"}))
		assert.Empty(t, call(t, handler, []string{""}),
			"an empty term is not a request for the whole dictionary")
	})

	t.Run("rejects malformed params", func(t *testing.T) {
		for _, params := range []any{[]string{}, []string{"a", "b"}, []int{1}} {
			_, err := handler.Handle(context.Background(), createRequest("searchAttributeMatches", params))
			assert.Error(t, err, "params %v", params)
		}
	})
}

// getFieldValueCompletions is the search box's value-completion source. Its guards are
// what keep an allowlist an allowlist, so they are pinned rather than left to
// the named-params table walk, which only checks the method has a name list.
func TestGetFieldValueCompletions(t *testing.T) {
	handler := setupHandlerWithData(t)
	ctx := context.Background()

	decode := func(t *testing.T, result any) []string {
		t.Helper()
		raw, ok := result.(json.RawMessage)
		require.True(t, ok, "expected json.RawMessage, got %T", result)
		var out []string
		require.NoError(t, json.Unmarshal(raw, &out))
		return out
	}

	t.Run("serves an allowlisted field", func(t *testing.T) {
		result, err := handler.Handle(ctx,
			createRequest("getFieldValueCompletions", []any{"traces", "name", "", 10}))
		require.NoError(t, err)
		assert.NotEmpty(t, decode(t, result))
	})

	t.Run("named params reach the same place", func(t *testing.T) {
		result, err := handler.Handle(ctx, createRequest("getFieldValueCompletions",
			map[string]any{
				"signal": "traces", "field": "name", "term": "", "limit": 10,
			}))
		require.NoError(t, err)
		assert.NotEmpty(t, decode(t, result))
	})

	t.Run("clamps a limit above the ceiling", func(t *testing.T) {
		// Not an error: a caller asking for everything gets a dropdown's
		// worth. The clamp is the only thing standing between a completion
		// keystroke and a full column dump.
		result, err := handler.Handle(ctx,
			createRequest("getFieldValueCompletions", []any{"traces", "name", "", 100000}))
		require.NoError(t, err)
		assert.LessOrEqual(t, len(decode(t, result)), 500)
	})

	t.Run("clamps a limit below one", func(t *testing.T) {
		for _, limit := range []any{0, -5} {
			result, err := handler.Handle(ctx,
				createRequest("getFieldValueCompletions", []any{"traces", "name", "", limit}))
			require.NoError(t, err, "limit %v", limit)
			assert.Len(t, decode(t, result), 1, "limit %v", limit)
		}
	})

	t.Run("refuses a field outside the allowlist", func(t *testing.T) {
		// statusMessage is a real span column, deliberately not completable:
		// the allowlist is a boundary, not a convenience.
		_, err := handler.Handle(ctx,
			createRequest("getFieldValueCompletions", []any{"traces", "statusMessage", "", 10}))
		require.Error(t, err)
	})

	t.Run("refuses an unknown signal", func(t *testing.T) {
		_, err := handler.Handle(ctx,
			createRequest("getFieldValueCompletions", []any{"spans", "name", "", 10}))
		require.Error(t, err)
	})

	t.Run("refuses a malformed call", func(t *testing.T) {
		cases := []struct {
			name   string
			params any
		}{
			{"too few params", []any{"traces", "name", ""}},
			{"too many params", []any{"traces", "name", "", 10, "extra"}},
			{"non-string signal", []any{1, "name", "", 10}},
			{"non-string field", []any{"traces", 1, "", 10}},
			{"non-string term", []any{"traces", "name", 1, 10}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := handler.Handle(ctx, createRequest("getFieldValueCompletions", tc.params))
				assert.Equal(t, jsonrpc2.ErrInvalidParams, err)
			})
		}
	})
}

func TestParseFieldValuesParamsClampsLimit(t *testing.T) {
	for _, tc := range []struct {
		limit int
		want  int64
	}{{limit: -5, want: 1}, {limit: 10, want: 10}, {limit: 1000, want: 500}} {
		raw := json.RawMessage(fmt.Sprintf(`["traces","name","term",%d]`, tc.limit))
		got, err := parseFieldValuesParams(raw)
		require.NoError(t, err)
		assert.Equal(t, fieldValuesParams{signal: "traces", field: "name", term: "term", limit: tc.want}, got)
	}
}

func TestAttributeDiscoveryLiteralTerms(t *testing.T) {
	h := setupHandler(t)
	cases := []struct {
		name, key, keyTerm, value, valueTerm, otherKey, otherValue string
	}{
		{"underscore-key", "under_score", "UNDER_", "100%done", "100%", "underXscore", "1000done"},
		{"percent-key", "percent%key", "PERCENT%", "value_with", "VALUE_", "percentXkey", "valueXwith"},
		{"backslash", `back\slash`, `BACK\S`, `C:\temp\file`, `C:\TEMP\`, `back\\slash`, `C:\\temp\\file`},
		{"combined", `mixed\%_key`, `MIXED\%_`, `mixed\%_value`, `MIXED\%_`, `mixed\abcXkey`, `mixed\abcXvalue`},
	}
	data := plog.NewLogs()
	rows := data.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords()
	for _, tc := range cases {
		// Duplicate values count once; lookalike keys and values must not match.
		for _, value := range []string{tc.value, tc.value, tc.otherValue} {
			attrs := rows.AppendEmpty().Attributes()
			attrs.PutStr(tc.key, value)
			attrs.PutStr(tc.otherKey, value)
		}
	}
	require.NoError(t, h.store.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(t.Context(), conn, data)
	}))
	s, err := NewServer("localhost:8000", 0, h.store, zaptest.NewLogger(t), nil)
	require.NoError(t, err)
	for _, tc := range cases {
		for _, stage := range []string{"keys", "values"} {
			t.Run(tc.name+"/"+stage, func(t *testing.T) {
				params := map[string]any{"term": tc.keyTerm, "signal": "logs"}
				count := int64(2)
				samples := []map[string]string{}
				if stage == "values" {
					params["term"] = tc.valueTerm
					params["owner"] = "log"
					params["path"] = []string{"attributes", tc.key}
					params["type"] = "string"
					count = 1
					samples = append(samples, map[string]string{"kind": "string", "value": tc.value})
				}
				t.Run("JSON", func(t *testing.T) {
					result, err := h.Handle(t.Context(), createRequest("searchAttributeMatches", params))
					require.NoError(t, err)
					want, err := json.Marshal([]map[string]any{{
						"name": tc.key, "attributeScope": "log", "owner": "log",
						"path": []string{"attributes", tc.key}, "type": "string",
						"matchCount": count, "sampleValues": samples,
					}})
					require.NoError(t, err)
					require.JSONEq(t, string(want), string(result.(json.RawMessage)))
				})
				t.Run("Arrow", func(t *testing.T) {
					wire, err := json.Marshal(params)
					require.NoError(t, err)
					record := readArrowDataset(t, s, "searchAttributeMatches", string(wire))
					require.Equal(t, int64(1), record.NumRows())
					for column, text := range map[int]string{0: tc.key, 1: "log", 2: "log", 4: "string"} {
						require.Equal(t, text, record.Column(column).(*array.String).Value(0))
					}
					require.Equal(t, count, record.Column(5).(*array.Int64).Value(0))
					values := record.Column(6).(*array.List).ListValues()
					require.Equal(t, len(samples), values.Len())
					if stage == "values" {
						require.Equal(t, tc.value, readArrowVariant(t, values, 0).Value())
					}
				})
			})
		}
	}
}
