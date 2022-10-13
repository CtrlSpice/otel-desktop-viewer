package desktopexporter

import (
	"context"
	"sync"

	"go.opentelemetry.io/collector/pdata/ptrace"
)

type Accumulator struct {
	mut       sync.Mutex
	traceData []ptrace.Traces
	spanCount int
}

func NewAccumulator() *Accumulator {
	return &Accumulator{
		traceData: make([]ptrace.Traces, 0, 10000),
		spanCount: 0,
	}
}

func (a *Accumulator) add(_ context.Context, traces ptrace.Traces) {
	a.mut.Lock()
	defer a.mut.Unlock()
	a.traceData = append(a.traceData, traces)
	a.spanCount += traces.SpanCount()
}
