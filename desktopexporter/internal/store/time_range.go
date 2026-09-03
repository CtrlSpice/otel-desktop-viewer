package store

import "github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/timerange"

// TimeRange is an inclusive time window. A nil endpoint is unbounded.
type TimeRange = timerange.TimeRange

// BoundedTimeRange builds the concrete range used by callers that already have
// both endpoints.
func BoundedTimeRange(start, end int64) TimeRange {
	return timerange.Bounded(start, end)
}
