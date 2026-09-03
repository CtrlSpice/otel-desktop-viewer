//go:build waterfallbench

package main

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
)

const (
	armCFlatFormat      = "odv.trace-waterfall.flat.v1"
	armCFlatPath        = "/benchmark-api/trace-waterfall/flat-rows"
	armCHealthPath      = "/benchmark-api/healthz"
	maxArmCRequestBytes = 4 << 10
)

var errArmCTraceNotFound = errors.New("Arm C trace not found")

//go:embed arm_c_flat_trace.sql
var armCFlatTraceSQL string

type armCRequest struct {
	TraceID string `json:"traceID"`
}

type armCServer struct {
	httpServer http.Server

	mu   sync.Mutex
	done chan struct{}
}

func newArmCServer(endpoint string, benchmarkStore *store.Store) *armCServer {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+armCHealthPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = fmt.Fprintln(w, armCFlatFormat)
	})
	mux.Handle("POST "+armCFlatPath, armCFlatHandler(benchmarkStore))

	return &armCServer{httpServer: http.Server{
		Addr:              endpoint,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}}
}

func (s *armCServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done != nil {
		return errors.New("Arm C server already started")
	}

	listener, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return err
	}
	done := make(chan struct{})
	s.done = done
	go func() {
		defer close(done)
		_ = s.httpServer.Serve(listener)
	}()
	return nil
}

func (s *armCServer) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()
	if done == nil {
		return nil
	}

	err := s.httpServer.Shutdown(ctx)
	<-done
	return err
}

func armCFlatHandler(benchmarkStore *store.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
			return
		}

		request, err := decodeArmCRequest(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		raw, err := queryArmCFlatTrace(r.Context(), benchmarkStore, request.TraceID)
		if errors.Is(err, errArmCTraceNotFound) {
			http.Error(w, "trace not found", http.StatusNotFound)
			return
		}
		if err != nil {
			if r.Context().Err() != nil {
				return
			}
			http.Error(w, "could not query trace", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(raw)
	})
}

func decodeArmCRequest(w http.ResponseWriter, r *http.Request) (armCRequest, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxArmCRequestBytes)
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request armCRequest
	if err := decoder.Decode(&request); err != nil {
		return armCRequest{}, fmt.Errorf("invalid request: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return armCRequest{}, errors.New("invalid request: expected one JSON object")
	}
	if request.TraceID != strings.ToLower(request.TraceID) || len(request.TraceID) != 32 {
		return armCRequest{}, errors.New("traceID must be 32 lowercase hexadecimal characters")
	}
	if err := validateHexID(request.TraceID, 16); err != nil {
		return armCRequest{}, fmt.Errorf("traceID: %w", err)
	}
	return request, nil
}

func queryArmCFlatTrace(ctx context.Context, benchmarkStore *store.Store, traceID string) ([]byte, error) {
	var raw []byte
	err := benchmarkStore.WithDBRead(func(db *sql.DB) error {
		return db.QueryRowContext(ctx, armCFlatTraceSQL, traceID).Scan(&raw)
	})
	if err != nil {
		return nil, fmt.Errorf("query Arm C flat trace: %w", err)
	}
	if raw == nil {
		return nil, errArmCTraceNotFound
	}
	return raw, nil
}
