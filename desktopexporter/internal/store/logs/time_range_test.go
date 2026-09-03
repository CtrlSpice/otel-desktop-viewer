package logs

import (
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/timerange"
	"github.com/stretchr/testify/require"
)

func TestLogTimePredicateShapes(t *testing.T) {
	start, end := int64(10), int64(20)
	for _, tc := range []struct {
		name      string
		timeRange timerange.TimeRange
		wantWhere string
		wantArgs  []any
	}{
		{"unbounded", timerange.TimeRange{}, "true", []any{}},
		{"end only", timerange.TimeRange{End: &end}, "l.log_time <= time_end", []any{end}},
		{"start only", timerange.TimeRange{Start: &start}, "l.log_time >= time_start", []any{start}},
		{"bounded", timerange.TimeRange{Start: &start, End: &end}, "l.log_time >= time_start AND l.log_time <= time_end", []any{start, end}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, where, args, err := buildLogSQL(nil, tc.timeRange)
			require.NoError(t, err)
			require.Equal(t, tc.wantWhere, where)
			require.Equal(t, tc.wantArgs, args)
		})
	}
}
