package server

import (
	"database/sql/driver"
	"encoding/json"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap/zaptest"
)

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
						require.Equal(t, tc.value, values.(*array.String).Value(0))
					}
				})
			})
		}
	}
}
