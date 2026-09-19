package logs_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"database/sql/driver"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

type traceLogSummaryJSON struct {
	ID             string  `json:"id"`
	Timestamp      string  `json:"timestamp"`
	SpanID         *string `json:"spanID"`
	SeverityText   string  `json:"severityText"`
	SeverityNumber int32   `json:"severityNumber"`
	ServiceName    string  `json:"serviceName"`
	BodyPreview    string  `json:"bodyPreview"`
}

func TestGetTraceLogsPreservesTraceScopedRowsAndOrder(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	traceID := mustDecodeTraceIDLogs("00000000000000000000000000000099")
	otherTraceID := mustDecodeTraceIDLogs("00000000000000000000000000000098")
	directSpanID := mustDecodeSpanIDLogs("0000000000000001")
	missingSpanID := mustDecodeSpanIDLogs("0000000000000007")

	traces := ptrace.NewTraces()
	resourceSpans := traces.ResourceSpans().AppendEmpty()
	scopeSpans := resourceSpans.ScopeSpans().AppendEmpty()
	span := scopeSpans.Spans().AppendEmpty()
	span.SetTraceID(traceID)
	span.SetSpanID(directSpanID)
	span.SetName("direct-owner")
	span.SetStartTimestamp(1)
	span.SetEndTimestamp(2)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, traces, s.FlushedIDs())
	}))

	data := plog.NewLogs()
	resourceLogs := data.ResourceLogs().AppendEmpty()
	resourceLogs.Resource().Attributes().PutStr("service.name", "trace-service")
	scopeLogs := resourceLogs.ScopeLogs().AppendEmpty()

	traceOnly := scopeLogs.LogRecords().AppendEmpty()
	traceOnly.SetTraceID(traceID)
	traceOnly.SetTimestamp(0)
	traceOnly.SetObservedTimestamp(100)
	traceOnly.Body().SetStr("trace-only")

	missingSpan := scopeLogs.LogRecords().AppendEmpty()
	missingSpan.SetTraceID(traceID)
	missingSpan.SetSpanID(missingSpanID)
	missingSpan.SetTimestamp(200)
	missingSpan.SetObservedTimestamp(999)
	missingSpan.Body().SetStr("missing-span")

	for _, body := range []string{"tie-a", "tie-b"} {
		record := scopeLogs.LogRecords().AppendEmpty()
		record.SetTraceID(traceID)
		record.SetSpanID(directSpanID)
		record.SetTimestamp(300)
		record.SetObservedTimestamp(999)
		record.SetSeverityText("INFO")
		record.SetSeverityNumber(plog.SeverityNumberInfo)
		record.Body().SetStr(body)
	}

	otherTrace := scopeLogs.LogRecords().AppendEmpty()
	otherTrace.SetTraceID(otherTraceID)
	otherTrace.SetSpanID(directSpanID)
	otherTrace.SetTimestamp(50)
	otherTrace.Body().SetStr("same-span-other-trace")

	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, data, s.FlushedIDs())
	}))

	// Fix the equal-time IDs so the expected ID tie-break is explicit rather
	// than dependent on random ingest UUIDs.
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		if _, err := db.ExecContext(ctx,
			`update logs set id = ?::uuid where body_preview(body) = ?`,
			"00000000-0000-0000-0000-0000000000ff", "tie-a"); err != nil {
			return err
		}
		_, err := db.ExecContext(ctx,
			`update logs set id = ?::uuid where body_preview(body) = ?`,
			"00000000-0000-0000-0000-000000000010", "tie-b")
		return err
	}))

	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return logs.GetTraceLogs(ctx, db, "00000000000000000000000000000099")
	})
	require.NoError(t, err)

	var got []traceLogSummaryJSON
	require.NoError(t, json.Unmarshal(raw, &got))
	require.Len(t, got, 4)
	require.Equal(t, []string{"100", "200", "300", "300"}, []string{
		got[0].Timestamp, got[1].Timestamp, got[2].Timestamp, got[3].Timestamp,
	})
	require.Nil(t, got[0].SpanID)
	require.Equal(t, "trace-only", got[0].BodyPreview)
	require.Equal(t, "0000000000000007", *got[1].SpanID)
	require.Equal(t, "missing-span", got[1].BodyPreview)
	require.Equal(t, "00000000-0000-0000-0000-000000000010", got[2].ID)
	require.Equal(t, "tie-b", got[2].BodyPreview)
	require.Equal(t, "00000000-0000-0000-0000-0000000000ff", got[3].ID)
	require.Equal(t, "tie-a", got[3].BodyPreview)
	for _, entry := range got {
		require.NotEqual(t, "same-span-other-trace", entry.BodyPreview)
	}

	var shape []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &shape))
	for _, entry := range shape {
		require.Len(t, entry, 7)
		for _, key := range []string{"id", "timestamp", "spanID", "severityText", "severityNumber", "serviceName", "bodyPreview"} {
			require.Contains(t, entry, key)
		}
		require.NotContains(t, entry, "body")
		require.NotContains(t, entry, "attributes")
		require.NotContains(t, entry, "traceID")
	}

	empty, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return logs.GetTraceLogs(ctx, db, "00000000000000000000000000000097")
	})
	require.NoError(t, err)
	require.JSONEq(t, `[]`, string(empty))
}

func TestGetTraceLogsIgnoresSearchTimeRangeAndHasNoDefaultLimit(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	traceID := mustDecodeTraceIDLogs("00000000000000000000000000000096")

	data := plog.NewLogs()
	resourceLogs := data.ResourceLogs().AppendEmpty()
	scopeLogs := resourceLogs.ScopeLogs().AppendEmpty()
	for i := 0; i < 101; i++ {
		record := scopeLogs.LogRecords().AppendEmpty()
		record.SetTraceID(traceID)
		record.SetTimestamp(pcommon.Timestamp(10_000 + i))
		record.Body().SetStr("complete")
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, data, s.FlushedIDs())
	}))

	clipped, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return logs.Search(ctx, db, store.BoundedTimeRange(10_010, 10_011), nil)
	})
	require.NoError(t, err)
	var searchResult []logSummaryJSON
	require.NoError(t, json.Unmarshal(clipped, &searchResult))
	require.Len(t, searchResult, 2)

	complete, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return logs.GetTraceLogs(ctx, db, "00000000000000000000000000000096")
	})
	require.NoError(t, err)
	var traceResult []traceLogSummaryJSON
	require.NoError(t, json.Unmarshal(complete, &traceResult))
	require.Len(t, traceResult, 101)
	require.Equal(t, "10000", traceResult[0].Timestamp)
	require.Equal(t, "10100", traceResult[100].Timestamp)
}

func TestGetTraceLogsHonorsCanceledContext(t *testing.T) {
	t.Parallel()
	s, _ := storetest.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return logs.GetTraceLogs(ctx, db, "00000000000000000000000000000099")
	})
	require.ErrorIs(t, err, context.Canceled)
}
