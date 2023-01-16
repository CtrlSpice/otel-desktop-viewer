package desktopexporter

import "time"

func (trace *TraceData) GetTraceSummary() (TraceSummary) {
	rootSpan, err := trace.getRootSpan()
	
	if err == ErrMissingRootSpan {
		return TraceSummary{
			HasRootSpan:     false,
			RootServiceName: "",
			RootName:        "",
			RootStartTime:   time.Time{},
			RootEndTime:	 time.Time{},
			SpanCount:       uint32(len(trace.Spans)),
			TraceID:         trace.TraceID,
		}
	}

	return TraceSummary{
		HasRootSpan:     true,
		RootServiceName: rootSpan.Attributes["service.name"].(string),
		RootName:        rootSpan.Name,
		RootStartTime:   rootSpan.StartTime,
		RootEndTime:	 rootSpan.EndTime,
		SpanCount:       uint32(len(trace.Spans)),
		TraceID:         trace.TraceID,
	}
}

func (trace *TraceData) getRootSpan()(SpanData, error){
	for i := 0; i < len(trace.Spans); i++ {
		if trace.Spans[i].ParentSpanID == "" {
			return trace.Spans[i], nil
		}
	}
	return SpanData{}, ErrMissingRootSpan
}
