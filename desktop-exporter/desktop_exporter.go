package desktopexporter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

type desktopExporter struct {
	logger          *zap.Logger
	traceStore      *TraceStore
	tracesMarshaler ptrace.Marshaler
}

func (exporter *desktopExporter) pushTraces(context context.Context, traces ptrace.Traces) error {
	extractSpans(context, traces, exporter.traceStore)
	for traceID, spans := range exporter.traceStore.traceMap {
		fmt.Println(traceID)
		for _, sp := range spans {
			jsonString, err := json.Marshal(sp)
			if err != nil {
				fmt.Println(err)
			} else {
				fmt.Println(string(jsonString))
			}
		}
	}

	// buf, err := exporter.tracesMarshaler.MarshalTraces(traces)
	// if err != nil {
	// 	return err
	// }
	// exporter.logger.Info(string(buf))
	return nil
}

func newDesktopExporter(logger *zap.Logger) *desktopExporter {

	return &desktopExporter{
		logger:          logger,
		traceStore:      NewTraceStore(),
		tracesMarshaler: ptrace.NewJSONMarshaler(),
	}
}

func (exporter *desktopExporter) Start(_ context.Context, host component.Host) error {
	//http.HandleFunc("/", exporter.getSpanCount)
	go func() { http.ListenAndServe(":8090", nil) }()
	return nil
}

func (exporter *desktopExporter) Shutdown(context.Context) error {
	exporter.logger.Info("SHUTDOWN FUNCTION")
	return nil
}
