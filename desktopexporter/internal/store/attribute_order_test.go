package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
)

// TestAttributeDiscoveryOrderIsTotal pins the order of the attribute
// discovery endpoints, which populate the search dropdowns and are the
// responses an agent or a golden test would cache.
//
// The queries ordered by (key, scope) already, which is total only while no
// key appears at one scope with two value types. That is not an exotic
// input: the same attribute key recorded as a string by one service and an
// int by another is ordinary telemetry, and every such tie made row order
// DuckDB's choice -- observed as a dropdown that reshuffled between calls.
// Ordering by type as well makes the sort key the full distinct tuple, so
// the response is deterministic by construction rather than by luck.
//
// The fixture ingests one key per signal in four value types to make the
// tie population real, then asserts two things separately: repeated calls
// are byte-identical, and the order is the declared (key, scope, type)
// sort rather than merely stable.
func TestAttributeDiscoveryOrderIsTotal(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	defer s.Close()

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC).UnixNano()

	putTyped := func(m pcommon.Map, key string, variant int) {
		switch variant {
		case 0:
			m.PutStr(key, "s")
		case 1:
			m.PutInt(key, 1)
		case 2:
			m.PutDouble(key, 1.5)
		case 3:
			m.PutBool(key, true)
		}
	}

	// Traces: the tied key on span scope, spread across four spans.
	traces := ptrace.NewTraces()
	for i := 0; i < 4; i++ {
		rs := traces.ResourceSpans().AppendEmpty()
		rs.Resource().Attributes().PutStr("service.name", "order-test")
		sp := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
		sp.SetTraceID([16]byte{0xAA, byte(i)})
		sp.SetSpanID([8]byte{0xAA, byte(i)})
		sp.SetName("span")
		sp.SetStartTimestamp(pcommon.Timestamp(base))
		sp.SetEndTimestamp(pcommon.Timestamp(base + 1000))
		putTyped(sp.Attributes(), "ambiguous.key", i)
		sp.Attributes().PutStr("zz.last", "v")
		sp.Attributes().PutStr("aa.first", "v")
	}

	// Logs: same shape on log scope.
	lg := plog.NewLogs()
	for i := 0; i < 4; i++ {
		rl := lg.ResourceLogs().AppendEmpty()
		rl.Resource().Attributes().PutStr("service.name", "order-test")
		rec := rl.ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()
		rec.SetTimestamp(pcommon.Timestamp(base))
		rec.Body().SetStr("m")
		putTyped(rec.Attributes(), "ambiguous.key", i)
	}

	// Metrics: same shape on datapoint scope.
	md := pmetric.NewMetrics()
	for i := 0; i < 4; i++ {
		rm := md.ResourceMetrics().AppendEmpty()
		rm.Resource().Attributes().PutStr("service.name", "order-test")
		g := rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
		g.SetName("order.gauge")
		dp := g.SetEmptyGauge().DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.Timestamp(base))
		dp.SetDoubleValue(1)
		putTyped(dp.Attributes(), "ambiguous.key", i)
	}

	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		if err := spans.Ingest(ctx, conn, traces, s.FlushedIDs()); err != nil {
			return err
		}
		if err := logs.Ingest(ctx, conn, lg, s.FlushedIDs()); err != nil {
			return err
		}
		return metrics.Ingest(ctx, conn, md, s.FlushedIDs())
	}))

	type attrDef struct {
		Name  string `json:"name"`
		Scope string `json:"attributeScope"`
		Type  string `json:"type"`
	}

	fetch := func(t *testing.T, get func(context.Context, *sql.DB, int64, int64) (json.RawMessage, error)) (string, []attrDef) {
		t.Helper()
		var raw json.RawMessage
		require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
			var err error
			raw, err = get(ctx, db, base-int64(time.Hour), base+int64(time.Hour))
			return err
		}))
		var defs []attrDef
		require.NoError(t, json.Unmarshal(raw, &defs))
		return string(raw), defs
	}

	endpoints := []struct {
		name string
		get  func(context.Context, *sql.DB, int64, int64) (json.RawMessage, error)
	}{
		{"traces", spans.GetTraceAttributes},
		{"logs", logs.GetLogAttributes},
		{"metrics", metrics.GetMetricAttributes},
	}

	for _, ep := range endpoints {
		t.Run(ep.name, func(t *testing.T) {
			first, defs := fetch(t, ep.get)

			// The tie population must exist, or this test proves nothing:
			// the tied key has to come back in all four types.
			tied := 0
			for _, d := range defs {
				if d.Name == "ambiguous.key" {
					tied++
				}
			}
			require.Equal(t, 4, tied,
				"fixture must produce a four-way (key, scope) tie")

			// Byte-stable across repeated calls.
			for i := 0; i < 3; i++ {
				again, _ := fetch(t, ep.get)
				require.Equal(t, first, again,
					"repeated calls must be byte-identical")
			}

			// And the declared order, not merely a stable one.
			sorted := make([]attrDef, len(defs))
			copy(sorted, defs)
			sort.SliceStable(sorted, func(i, j int) bool {
				a, b := sorted[i], sorted[j]
				if a.Name != b.Name {
					return a.Name < b.Name
				}
				if a.Scope != b.Scope {
					return a.Scope < b.Scope
				}
				return a.Type < b.Type
			})
			require.Equal(t, sorted, defs,
				"response must be sorted by (key, scope, type)")
		})
	}
}
