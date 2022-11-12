package desktopexporter

import (
	"time"
)

func (trace *TraceData) GetTraceSummary() (*TraceSummary, error) {
	duration, err := trace.getTraceDuration()
	if err != nil {
		return nil, err
	}

	return &TraceSummary{
		TraceID:    trace.Spans[0].TraceID,
		SpanCount:  uint32(len(trace.Spans)),
		DurationMS: duration.Milliseconds(),
	}, nil

}

func (trace *TraceData) getTraceDuration() (time.Duration, error) {
	if len(trace.Spans) < 1 {
		return 0, ErrEmptySpansSlice
	}

	// Determine the total duration of the trace
	traceStartTime := trace.Spans[0].StartTime
	traceEndTime := trace.Spans[0].EndTime
	for i := 1; i < len(trace.Spans); i++ {

		if trace.Spans[i].StartTime.Before(traceStartTime) {
			traceStartTime = trace.Spans[i].StartTime
		}

		if trace.Spans[i].EndTime.After(traceEndTime) {
			traceEndTime = trace.Spans[i].EndTime
		}
	}
	return traceEndTime.Sub(traceStartTime), nil
}
