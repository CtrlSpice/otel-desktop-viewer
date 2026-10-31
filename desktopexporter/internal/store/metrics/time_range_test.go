package metrics

import (
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/timerange"
	"github.com/stretchr/testify/require"
)

func TestMetricSummaryTimePredicateShapes(t *testing.T) {
	start, end := uint64(10), uint64(20)
	for _, tc := range []struct {
		name        string
		timeRange   timerange.TimeRange
		wantWhere   string
		wantDPWhere string
		wantArgs    []any
	}{
		{"unbounded", timerange.TimeRange{}, "exists (select 1 from metric_datapoints d where d.metric_id = s.id)", "", []any{}},
		{"end only", timerange.TimeRange{End: &end}, "exists (select 1 from metric_datapoints d where d.metric_id = s.id and d.timestamp <= $1::ubigint)", "where d.timestamp <= $1::ubigint", []any{"20"}},
		{"start only", timerange.TimeRange{Start: &start}, "exists (select 1 from metric_datapoints d where d.metric_id = s.id and d.timestamp >= $1::ubigint)", "where d.timestamp >= $1::ubigint", []any{"10"}},
		{"bounded", timerange.TimeRange{Start: &start, End: &end}, "exists (select 1 from metric_datapoints d where d.metric_id = s.id and d.timestamp >= $1::ubigint AND d.timestamp <= $2::ubigint)", "where d.timestamp >= $1::ubigint AND d.timestamp <= $2::ubigint", []any{"10", "20"}},
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
