package logs

import (
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/timerange"
	"github.com/stretchr/testify/require"
)

func TestLogTimePredicateShapes(t *testing.T) {
	start, end := uint64(10), uint64(20)
	for _, tc := range []struct {
		name      string
		timeRange timerange.TimeRange
		wantWhere string
		wantArgs  []any
	}{
		{"unbounded", timerange.TimeRange{}, "true", []any{}},
		{"end only", timerange.TimeRange{End: &end}, "l.log_time <= $1::ubigint", []any{"20"}},
		{"start only", timerange.TimeRange{Start: &start}, "l.log_time >= $1::ubigint", []any{"10"}},
		{"bounded", timerange.TimeRange{Start: &start, End: &end}, "l.log_time >= $1::ubigint AND l.log_time <= $2::ubigint", []any{"10", "20"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, where, args, err := buildLogSQL(nil, tc.timeRange)
			require.NoError(t, err)
			require.Equal(t, tc.wantWhere, where)
			require.Equal(t, tc.wantArgs, args)
		})
	}
}
