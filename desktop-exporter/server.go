package desktopexporter

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
)

type Server struct {
	server     http.Server
	traceStore *TraceStore
}

func getTracesHandler(traceStore *TraceStore) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		traceStore.mut.Lock()
		defer traceStore.mut.Unlock()

		writer.WriteHeader(http.StatusOK)

		traces, err := json.Marshal(traceStore.traceMap)
		if err != nil {
			fmt.Printf("error marshalling traceStore: %s\n", err)
		} else {
			fmt.Fprintf(writer, "Hello traceStore:\n%s\n", traces)
		}
	}
}

func getTraceIDHandler(traceStore *TraceStore) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		traceStore.mut.Lock()
		defer traceStore.mut.Unlock()

		writer.WriteHeader(http.StatusOK)

		traceID := mux.Vars(request)["id"]
		spans, err := json.Marshal(traceStore.traceMap[traceID])
		if err != nil {
			fmt.Printf("error marshalling trace %s: %s\n", traceID, err)
		} else {
			fmt.Fprintf(writer, "Hello trace %s:\n%s\n", traceID, spans)
		}
	}
}

func NewServer(traceStore *TraceStore) *Server {
	router := mux.NewRouter()
	router.HandleFunc("/traces", getTracesHandler(traceStore))
	router.HandleFunc("/trace/{id}", getTraceIDHandler(traceStore))
	router.PathPrefix("/").Handler(http.FileServer(http.Dir("./desktop-exporter/static/")))

	return &Server{
		server: http.Server{
			Addr:    "localhost:8000",
			Handler: router,
		},
		traceStore: traceStore,
	}
}

func (s Server) Start() error {
	return s.server.ListenAndServe()
}

func (s Server) Close() error {
	return s.server.Close()
}
