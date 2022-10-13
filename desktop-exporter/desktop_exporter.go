package desktopexporter

import (
	"context"
	"io"
	"net/http"
	"strconv"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

type desktopExporter struct {
	logger          *zap.Logger
	accumulator     *Accumulator
	tracesMarshaler ptrace.Marshaler
}

func (exporter *desktopExporter) getSpanCount(writer http.ResponseWriter, request *http.Request) {
	exporter.accumulator.mut.Lock()
	defer exporter.accumulator.mut.Unlock()

	spanCount := exporter.accumulator.spanCount
	io.WriteString(writer, "Hello World!\nI've accumulated "+
		strconv.Itoa(spanCount)+
		" spans.\n")
}

func (exporter *desktopExporter) pushTraces(context context.Context, traces ptrace.Traces) error {
	exporter.logger.Info("TracesExporter", zap.Int("#spans", traces.SpanCount()))

	exporter.accumulator.add(context, traces)

	buf, err := exporter.tracesMarshaler.MarshalTraces(traces)
	if err != nil {
		return err
	}

	exporter.logger.Info(string(buf))
	return nil
}

func newDesktopExporter(logger *zap.Logger) *desktopExporter {

	return &desktopExporter{
		logger:          logger,
		accumulator:     NewAccumulator(),
		tracesMarshaler: ptrace.NewJSONMarshaler(),
	}
}

func (exporter *desktopExporter) Start(_ context.Context, host component.Host) error {
	http.HandleFunc("/", exporter.getSpanCount)
	go func() { http.ListenAndServe(":8090", nil) }()
	return nil
}

func (exporter *desktopExporter) Shutdown(context.Context) error {
	exporter.logger.Info("SHUTDOWN FUNCTION")
	return nil
}
