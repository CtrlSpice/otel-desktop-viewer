package spans_test

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestSpanNumericEnumsRoundTripAndRemainSearchable(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	traceID := [16]byte{15: 42}
	cases := []struct {
		kind        ptrace.SpanKind
		status      ptrace.StatusCode
		kindLabel   string
		statusLabel string
	}{
		{ptrace.SpanKindUnspecified, ptrace.StatusCodeUnset, "Unspecified", "Unset"},
		{ptrace.SpanKindServer, ptrace.StatusCodeError, "Server", "Error"},
		{ptrace.SpanKind(99), ptrace.StatusCode(99), "Unknown (99)", "Unknown (99)"},
		{ptrace.SpanKind(-1), ptrace.StatusCode(-1), "Unknown (-1)", "Unknown (-1)"},
		{ptrace.SpanKind(math.MinInt32), ptrace.StatusCode(math.MaxInt32), "Unknown (-2147483648)", "Unknown (2147483647)"},
	}

	data := ptrace.NewTraces()
	ss := data.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty()
	for i, tc := range cases {
		span := ss.Spans().AppendEmpty()
		span.SetTraceID(traceID)
		span.SetSpanID([8]byte{7: byte(i + 1)})
		span.SetName(tc.kindLabel)
		span.SetKind(tc.kind)
		span.Status().SetCode(tc.status)
		span.SetStartTimestamp(pcommon.Timestamp(i + 1))
		span.SetEndTimestamp(pcommon.Timestamp(i + 2))
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, data, s.FlushedIDs())
	}))

	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		rows, err := db.QueryContext(ctx, `select kind, status_code from spans order by span_id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for _, tc := range cases {
			require.True(t, rows.Next())
			var kind, status int32
			require.NoError(t, rows.Scan(&kind, &status))
			assert.Equal(t, int32(tc.kind), kind)
			assert.Equal(t, int32(tc.status), status)
		}
		return rows.Err()
	}))

	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return spans.SearchSpans(ctx, db, "0000000000000000000000000000002a", nil)
	})
	require.NoError(t, err)
	var detail struct {
		Spans []struct {
			SpanData struct {
				KindCode        int32  `json:"kindCode"`
				Kind            string `json:"kind"`
				StatusCodeValue int32  `json:"statusCodeValue"`
				StatusCode      string `json:"statusCode"`
			} `json:"spanData"`
		} `json:"spans"`
	}
	require.NoError(t, json.Unmarshal(raw, &detail))
	require.Len(t, detail.Spans, len(cases))
	for i, tc := range cases {
		assert.Equal(t, int32(tc.kind), detail.Spans[i].SpanData.KindCode)
		assert.Equal(t, tc.kindLabel, detail.Spans[i].SpanData.Kind)
		assert.Equal(t, int32(tc.status), detail.Spans[i].SpanData.StatusCodeValue)
		assert.Equal(t, tc.statusLabel, detail.Spans[i].SpanData.StatusCode)
	}

	for _, tc := range []struct {
		field    string
		typeName string
		value    any
	}{
		{"kind", "string", "Server"},
		{"kindCode", "int64", int64(99)},
		{"statusCode", "string", "Error"},
		{"statusCodeValue", "int64", int64(-1)},
	} {
		query := &search.QueryNode{ID: tc.field, Type: "condition", Query: &search.Query{
			Field:         &search.FieldDefinition{Name: tc.field, Type: tc.typeName, SearchScope: "field"},
			FieldOperator: "=", Value: fmt.Sprint(tc.value),
		}}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.SearchTraces(ctx, db, store.BoundedTimeRange(0, 10), query)
		})
		require.NoError(t, err)
		var summaries []map[string]any
		require.NoError(t, json.Unmarshal(raw, &summaries))
		require.Len(t, summaries, 1)
	}

	raw, err = readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return spans.SearchTraces(ctx, db, store.BoundedTimeRange(0, 10), nil)
	})
	require.NoError(t, err)
	var summaries []map[string]any
	require.NoError(t, json.Unmarshal(raw, &summaries))
	require.Len(t, summaries, 1)
	assert.Equal(t, float64(1), summaries[0]["errorCount"], "only status code 2 is an error")
}
