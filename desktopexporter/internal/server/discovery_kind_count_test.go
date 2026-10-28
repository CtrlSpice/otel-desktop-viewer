package server

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/internal/discovery"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestDiscoveryKindCountsDistinctRecordsWithinOwnerAndSignal(t *testing.T) {
	h := setupHandler(t)
	ctx := context.Background()
	data := ptrace.NewTraces()
	for index := range 103 {
		r := data.ResourceSpans().AppendEmpty()
		r.Resource().Attributes().PutStr("service.name", "checkout")
		scope := r.ScopeSpans().AppendEmpty()
		s := scope.Spans().AppendEmpty()
		s.SetTraceID(pcommon.TraceID{15: 1})
		s.SetSpanID(pcommon.SpanID{7: byte(index + 1)})
		s.SetStartTimestamp(100)
		s.SetEndTimestamp(101)
		s.SetName("needle")
		s.Attributes().PutStr("attempt", "1")
		if index == 99 {
			s.Attributes().PutInt("attempt", 1)
		}
		if index == 100 {
			r.Resource().Attributes().PutStr("service.name", "other")
		}
		if index == 101 {
			s.SetStartTimestamp(101)
		}
		if index == 102 {
			s.SetName("other")
		}
		if index < 3 {
			scope.Scope().Attributes().PutStr("attempt", "1")
		} else if index == 3 {
			scope.Scope().Attributes().PutInt("attempt", 1)
		}
		if index < 3 {
			values := [][]string{{"1", "2", "1"}, {"1"}, {"2"}}
			for _, value := range values[index] {
				event := s.Events().AppendEmpty()
				event.SetTimestamp(100)
				event.Attributes().PutStr("attempt", value)
			}
		}
		// One span carries both kinds; neither kind counts nor value counts
		// can be summed to recover the all-kind distinct-record denominator.
		if index == 0 || index == 3 {
			event := s.Events().AppendEmpty()
			event.SetTimestamp(100)
			event.Attributes().PutInt("attempt", 1)
		}
		if index == 0 {
			for _, value := range []string{"1", "2", "1"} {
				s.Links().AppendEmpty().Attributes().PutStr("attempt", value)
			}
		} else if index < 3 {
			s.Links().AppendEmpty().Attributes().PutInt("attempt", 1)
		}
	}
	require.NoError(t, h.store.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, data)
	}))
	for index := range 3 {
		ld := buildTestLogs()
		r := ld.ResourceLogs().At(0)
		r.Resource().Attributes().PutStr("service.name", "checkout")
		scope := r.ScopeLogs().At(0)
		scope.LogRecords().At(0).SetTimestamp(100)
		if index == 0 {
			scope.Scope().Attributes().PutStr("attempt", "1")
		} else {
			scope.Scope().Attributes().PutInt("attempt", 1)
		}
		require.NoError(t, h.store.WithConn(func(conn driver.Conn) error {
			return logs.Ingest(ctx, conn, ld)
		}))
		md := buildTestMetrics()
		rm := md.ResourceMetrics().At(0)
		rm.Resource().Attributes().PutStr("service.name", "checkout")
		sm := rm.ScopeMetrics().At(0)
		sm.Metrics().At(0).Gauge().DataPoints().At(0).SetTimestamp(100)
		if index < 2 {
			sm.Scope().Attributes().PutStr("attempt", "1")
		} else {
			sm.Scope().Attributes().PutInt("attempt", 1)
		}
		require.NoError(t, h.store.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md, h.store.FlushedIDs())
		}))
	}
	for _, tc := range []struct {
		signal, ownerType      string
		strings, ints, records int
		values                 int
	}{
		{"traces", "span", 99, 1, 100, 2},
		{"traces", "event", 3, 2, 4, 3},
		{"traces", "link", 1, 2, 3, 3},
		{"traces", "scope", 3, 1, 4, 2},
		{"logs", "scope", 1, 2, 3, 2},
		{"metrics", "scope", 2, 1, 3, 2},
	} {
		t.Run(tc.signal+"/"+tc.ownerType, func(t *testing.T) {
			for offset := range tc.values {
				result, err := h.Handle(ctx, createRequest("discoverTelemetry", map[string]any{"options": map[string]any{
					"kind": "values", "signal": tc.signal, "ownerType": tc.ownerType, "key": "attempt",
					"serviceName": "checkout", "startTime": "100", "endTime": "100", "limit": 1, "offset": offset,
					"queries": map[string]any{"traces": map[string]any{"type": "condition", "query": map[string]any{
						"field": map[string]any{"name": "name", "searchScope": "field", "type": "string"}, "fieldOperator": "=", "value": "needle",
					}}},
				}}))
				require.NoError(t, err)
				var page struct {
					Rows [][]struct {
						Value struct {
							Kind string `json:"kind"`
						} `json:"value"`
						Count             int                  `json:"count"`
						KindCount         int                  `json:"kindCount"`
						Denominator       int                  `json:"denominator"`
						RelativeFrequency float64              `json:"relativeFrequency"`
						FoundOn           []discovery.Location `json:"foundOn"`
					} `json:"rows"`
					Truncated bool `json:"truncated"`
				}
				require.NoError(t, json.Unmarshal(result.(json.RawMessage), &page))
				require.Len(t, page.Rows, 1)
				value := page.Rows[0][0]
				want := tc.strings
				if value.Value.Kind == "int64" {
					want = tc.ints
				} else {
					require.Equal(t, "string", value.Value.Kind)
				}
				require.Equal(t, want, value.KindCount)
				valueCount := want
				if tc.ownerType == "event" && value.Value.Kind == "string" {
					valueCount = 2 // Two values overlap on one of three spans.
				}
				require.Equal(t, valueCount, value.Count)
				require.Equal(t, tc.records, value.Denominator)
				require.Equal(t, float64(valueCount)/float64(tc.records), value.RelativeFrequency)
				require.Equal(t, []discovery.Location{{Signal: tc.signal, OwnerType: tc.ownerType}}, value.FoundOn)
				require.Equal(t, offset < tc.values-1, page.Truncated)
			}
		})
	}
}
