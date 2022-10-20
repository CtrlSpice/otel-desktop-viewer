package desktopexporter

import (
	"net/http"

	"go.opentelemetry.io/collector/pdata/ptrace"
)

type Server struct {
	traceStore      *TraceStore
	tracesMarshaler ptrace.Marshaler
	httpServer      *http.Server
}

// func (s *Server) getSpanCount(writer http.ResponseWriter, request *http.Request) {
// 	exporter.accumulator.mut.Lock()
// 	defer exporter.accumulator.mut.Unlock()

// 	spanCount := exporter.accumulator.spanCount
// 	io.WriteString(writer, "Hello World!\nI've accumulated "+
// 		strconv.Itoa(spanCount)+
// 		" spans.\n")
// }
