package timerange

// TimeRange is an inclusive time window. A nil endpoint is unbounded.
type TimeRange struct {
	Start *uint64
	End   *uint64
}

func Bounded[T ~int | ~int64 | ~uint64](start, end T) TimeRange {
	if start < 0 || end < 0 {
		panic("OTLP timestamp bounds cannot be negative")
	}
	startTimestamp, endTimestamp := uint64(start), uint64(end)
	return TimeRange{Start: &startTimestamp, End: &endTimestamp}
}
