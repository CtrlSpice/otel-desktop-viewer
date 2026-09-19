package spans_test

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"testing"

	"database/sql/driver"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestSpanTimestampsRoundTripAcrossUint64Range(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	values := []uint64{0, 1<<63 - 1, 1 << 63, ^uint64(0)}

	data := ptrace.NewTraces()
	ss := data.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty()
	traceID := pcommon.TraceID{15: 1}
	for i, timestamp := range values {
		span := ss.Spans().AppendEmpty()
		span.SetTraceID(traceID)
		span.SetSpanID(pcommon.SpanID{7: byte(i + 1)})
		span.SetName("boundary")
		span.SetStartTimestamp(pcommon.Timestamp(timestamp))
		span.SetEndTimestamp(pcommon.Timestamp(timestamp))
		event := span.Events().AppendEmpty()
		event.SetName("boundary")
		event.SetTimestamp(pcommon.Timestamp(timestamp))
	}
	skewed := ss.Spans().AppendEmpty()
	skewed.SetTraceID(traceID)
	skewed.SetSpanID(pcommon.SpanID{7: 5})
	skewed.SetName("skewed")
	skewed.SetStartTimestamp(pcommon.Timestamp(^uint64(0)))
	skewed.SetEndTimestamp(0)

	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, data, s.FlushedIDs())
	}))

	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		rows, err := db.QueryContext(ctx, `select s.start_time, s.end_time, e.timestamp
			from spans s join events e using (trace_id, span_id)
			order by s.span_id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for _, want := range values {
			require.True(t, rows.Next())
			var start, end, event uint64
			require.NoError(t, rows.Scan(&start, &end, &event))
			require.Equal(t, want, start)
			require.Equal(t, want, end)
			require.Equal(t, want, event)
		}
		return rows.Err()
	}))

	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return spans.SearchTraces(ctx, db, store.BoundedTimeRange(uint64(0), ^uint64(0)), nil)
	})
	require.NoError(t, err)
	var summaries []map[string]any
	require.NoError(t, json.Unmarshal(raw, &summaries))
	require.Equal(t, "0", summaries[0]["startTime"])
	require.Equal(t, "18446744073709551615", summaries[0]["durationNs"])

	raw, err = readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return spans.SearchSpans(ctx, db, hex.EncodeToString(traceID[:]), nil)
	})
	require.NoError(t, err)
	var trace struct {
		TraceStart string `json:"traceStart"`
		Spans      []struct {
			SpanData struct {
				Name  string `json:"name"`
				Start string `json:"start"`
				Dur   string `json:"dur"`
			} `json:"spanData"`
		} `json:"spans"`
	}
	require.NoError(t, json.Unmarshal(raw, &trace))
	require.Equal(t, "0", trace.TraceStart)
	for _, node := range trace.Spans {
		if node.SpanData.Name == "skewed" {
			require.Equal(t, "18446744073709551615", node.SpanData.Start)
			require.Equal(t, "-18446744073709551615", node.SpanData.Dur)
			return
		}
	}
	t.Fatal("skewed span not returned")
}
