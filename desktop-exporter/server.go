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

		jsonTraces, err := json.Marshal(traceStore.traceMap)
		if err != nil {
			panic(fmt.Errorf("error marshalling traceStore: %s\n", err))
		} else {
			writer.WriteHeader(http.StatusOK)
			writer.Header().Set("Content-Type", "application/json")
			writer.Write(jsonTraces)
		}
	}
}

func getTraceIDHandler(traceStore *TraceStore) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		traceStore.mut.Lock()
		defer traceStore.mut.Unlock()

		traceID := mux.Vars(request)["id"]
		jsonTrace, err := json.Marshal(traceStore.traceMap[traceID])
		if err != nil {
			fmt.Printf("error marshalling trace %s: %s\n", traceID, err)
		} else {
			writer.WriteHeader(http.StatusOK)
			writer.Header().Set("Content-Type", "application/json")
			writer.Write(jsonTrace)
		}
	}
}

func NewServer(traceStore *TraceStore) *Server {
	router := mux.NewRouter()
	router.HandleFunc("/traces", getTracesHandler(traceStore))
	router.HandleFunc("/traces/{id}", getTraceIDHandler(traceStore))
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
