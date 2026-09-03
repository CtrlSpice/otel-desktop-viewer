package metrics

import (
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/timerange"
	"github.com/stretchr/testify/require"
)

func TestMetricSummaryTimePredicateShapes(t *testing.T) {
	start, end := int64(10), int64(20)
	for _, tc := range []struct {
		name        string
		timeRange   timerange.TimeRange
		wantWhere   string
		wantDPWhere string
		wantArgs    []any
	}{
		{"unbounded", timerange.TimeRange{}, "exists (select 1 from datapoints d where d.metric_ingest_id = m.id)", "", []any{}},
		{"end only", timerange.TimeRange{End: &end}, "exists (select 1 from datapoints d where d.metric_ingest_id = m.id and d.timestamp <= time_end)", "where d.timestamp <= time_end", []any{end}},
		{"start only", timerange.TimeRange{Start: &start}, "exists (select 1 from datapoints d where d.metric_ingest_id = m.id and d.timestamp >= time_start)", "where d.timestamp >= time_start", []any{start}},
		{"bounded", timerange.TimeRange{Start: &start, End: &end}, "exists (select 1 from datapoints d where d.metric_ingest_id = m.id and d.timestamp >= time_start AND d.timestamp <= time_end)", "where d.timestamp >= time_start AND d.timestamp <= time_end", []any{start, end}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, where, args, err := buildMetricSQL(nil, tc.timeRange)
			require.NoError(t, err)
			require.Equal(t, tc.wantWhere, where)
			require.Equal(t, tc.wantDPWhere, metricDatapointWhere(tc.timeRange))
			require.Equal(t, tc.wantArgs, args)
		})
	}
}
