package timerange

// TimeRange is an inclusive time window. A nil endpoint is unbounded.
type TimeRange struct {
	Start *int64
	End   *int64
}

func Bounded(start, end int64) TimeRange {
	return TimeRange{Start: &start, End: &end}
}
