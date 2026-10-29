package server

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/internal/discovery"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Compare actual default-optimizer plans, not the number of CTE declarations.
// The old SQL exists only here as a semantic/performance regression oracle.
func TestDiscoveryOwnershipPlans(t *testing.T) {
	h := setupHandler(t)
	data := ptrace.NewTraces()
	for i := range 64 {
		resource := data.ResourceSpans().AppendEmpty()
		service := "chosen"
		if i%2 == 1 {
			service = "other"
		}
		resource.Resource().Attributes().PutStr("service.name", service)
		scope := resource.ScopeSpans().AppendEmpty()
		span := scope.Spans().AppendEmpty()
		span.SetTraceID(pcommon.TraceID{15: 1})
		span.SetSpanID(pcommon.SpanID{7: byte(i + 1)})
		span.SetStartTimestamp(100)
		span.SetEndTimestamp(101)
		for j := range 4 {
			event := span.Events().AppendEmpty()
			event.SetTimestamp(pcommon.Timestamp(100 + j))
			event.Attributes().PutInt("value", int64(j%2))
		}
	}
	md := buildTestMetrics()
	point := md.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).Gauge().DataPoints().At(0)
	point.SetTimestamp(100)
	for i := range 4 {
		e := point.Exemplars().AppendEmpty()
		e.SetTimestamp(pcommon.Timestamp(100 + i))
		e.FilteredAttributes().PutInt("value", int64(i%2))
	}
	require.NoError(t, h.store.WithConn(func(conn driver.Conn) error {
		if err := spans.Ingest(t.Context(), conn, data); err != nil {
			return err
		}
		return metrics.Ingest(t.Context(), conn, md)
	}))
	for _, tc := range []struct{ signal, owner, legacy string }{
		{"traces", "event", `SELECT struct_pack(trace_id := s.trace_id, span_id := s.span_id) AS record_id, e.timestamp AS time_ns, s.service_name, s.resource, e.attributes FROM spans s JOIN events e ON e.trace_id=s.trace_id AND e.span_id=s.span_id`},
		{"metrics", "exemplar", `SELECT d.id AS record_id, e.timestamp AS time_ns, m.service_name, d.resource, e.attributes FROM metric_datapoints d JOIN metrics m ON m.id=d.metric_id JOIN exemplars e ON e.metric_datapoint_id=d.id`},
	} {
		t.Run(tc.owner, func(t *testing.T) {
			start, end := "101", "102"
			service := "chosen"
			if tc.signal == "metrics" {
				service = "test-svc"
			}
			query := discovery.Query{StartTime: &start, EndTime: &end, ServiceName: &service}
			records, err := discovery.RecordsFrom(tc.signal, tc.owner, query)
			require.NoError(t, err)
			location := discovery.Location{Signal: tc.signal, OwnerType: tc.owner}
			statements := []string{discovery.Values(query, location, tc.legacy, "value"), discovery.Values(query, location, records, "value")}
			require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
				var previous []byte
				for i, statement := range statements {
					var result []byte
					err := db.QueryRowContext(t.Context(), "SELECT coalesce(to_json(list(entry)), '[]')::VARCHAR FROM ("+statement+")").Scan(&result)
					require.NoError(t, err)
					if i == 0 {
						previous = result
					} else {
						require.JSONEq(t, string(previous), string(result))
					}
					var key, plan string
					err = db.QueryRowContext(t.Context(), "EXPLAIN ANALYZE "+statement).Scan(&key, &plan)
					require.NoError(t, err)
					t.Logf("%s version=%d\n%s", tc.owner, i, plan)
					// Event traversal must no longer scan spans once for parents
					// and again through the events view.
					if tc.owner == "event" {
						require.Equal(t, []int{2, 1}[i], strings.Count(plan, "Table: memory.main.spans"))
					}
					if tc.owner == "exemplar" {
						require.Equal(t, []int{3, 1}[i], strings.Count(plan, "Table: memory.main.metric_records"))
					}
				}
				return nil
			}))
		})
	}
}

func TestHomeDiscoveryNativePipelinePlan(t *testing.T) {
	h := setupHandler(t)
	key := "value"
	plan, err := homeValuesPlan(discoveryOptions{Kind: "home-values", OwnerType: "span", Key: &key, ValueKind: "int64"})
	require.NoError(t, err)
	t.Logf("HOME_SQL\n%s\nEND_HOME_SQL", plan.statement)
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		var key, explain string
		err := db.QueryRowContext(t.Context(), "EXPLAIN ANALYZE "+plan.statement, plan.args...).Scan(&key, &explain)
		require.NoError(t, err)
		t.Log(explain)
		return nil
	}))
}

func TestDiscoveryMetadataStillUsesLatestLogicalDescriptor(t *testing.T) {
	h := setupHandler(t)
	for i := range 2 {
		md := buildTestMetrics()
		metric := md.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0)
		metric.Metadata().PutInt("version", int64(i))
		metric.Gauge().DataPoints().At(0).SetTimestamp(pcommon.Timestamp(100 + i))
		require.NoError(t, h.store.WithConn(func(conn driver.Conn) error { return metrics.Ingest(t.Context(), conn, md) }))
	}
	page := homePage(t, h, map[string]any{"ownerType": "metadata", "key": "version"})
	require.Len(t, page.Values, 1)
	require.Equal(t, "1", page.Values[0].Preview)
	require.EqualValues(t, 2, page.Values[0].Count)
	encoded, err := json.Marshal(page)
	require.NoError(t, err)
	require.NotEmpty(t, encoded)
}
