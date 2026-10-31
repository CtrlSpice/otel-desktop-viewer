package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/features"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
	"github.com/CtrlSpice/otel-desktop-viewer/skills"
	"github.com/rs/cors"
	"go.opentelemetry.io/otel/propagation"
	"go.uber.org/zap"
	"golang.org/x/exp/jsonrpc2"
)

// maxRPCRequestBodyBytes caps how much of an /rpc request body we will read.
// JSON-RPC requests in this app are small (a method name and a few params), so
// 1 MB is generous headroom while preventing a buggy or malicious client from
// exhausting memory with an unbounded body.
const maxRPCRequestBodyBytes = 1 << 20 // 1 MB

type Server struct {
	evalServer
	server         http.Server
	store          *store.Store
	jsonrpcHandler *JSONRPCHandler
	logger         *zap.Logger
	tel            *telemetry.Telemetry
	features       *features.Flags

	serveDone chan struct{}
	startMu   sync.Mutex
}

// WithOTLPGRPCPort supplies the receiver port advertised by the setup guide.
func WithOTLPGRPCPort(port int) Option {
	return func(s *Server) { s.jsonrpcHandler.otlpGRPCPort = port }
}

func NewServer(endpoint string, otlpHTTPPort int, store *store.Store, logger *zap.Logger, tel *telemetry.Telemetry, options ...Option) (*Server, error) {
	if tel == nil {
		tel = telemetry.Disabled()
	}
	s := Server{
		server: http.Server{
			Addr: endpoint,

			// ReadHeaderTimeout and IdleTimeout are pure hygiene: they bound a
			// client that opens a connection and dribbles (or never sends)
			// headers, and reap idle keep-alives. Neither can affect a
			// legitimate request.
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			IdleTimeout:       120 * time.Second,

			// Deliberately no WriteTimeout. Go measures it from the end of
			// header read through the end of the response write, so it caps
			// *handler execution* too -- and a getTraceView over a large trace
			// is unbounded by design. A WriteTimeout here would abort exactly
			// the slow queries a trace viewer exists to serve, and would do it
			// by killing the connection with a partial body. Query cost is
			// bounded by cancellation instead: the client aborts, and the
			// context tears the DuckDB query down.
		},
		jsonrpcHandler: NewJSONRPCHandler(store, logger),
		store:          store,
		logger:         logger,
		tel:            tel,
	}
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid viewer endpoint: %w", err)
	}
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	if host == "::" {
		host = "::1"
	}
	s.evalServer = newEvalServer(host, otlpHTTPPort)

	for _, option := range options {
		option(&s)
	}
	s.jsonrpcHandler.otlpHTTPPort = otlpHTTPPort
	if err := s.initHandler(); err != nil {
		return nil, fmt.Errorf("could not initialize desktop exporter server: %w", err)
	}

	return &s, nil
}

// Start binds the listen address synchronously, then serves HTTP on a
// background goroutine. Bind failures (e.g. port in use) return immediately.
func (s *Server) Start() error {
	s.startMu.Lock()
	defer s.startMu.Unlock()

	if s.serveDone != nil {
		return errors.New("server already started")
	}

	ln, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		return err
	}

	done := make(chan struct{})
	s.serveDone = done
	go func() {
		defer close(done)
		err := s.server.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("HTTP server failed", zap.Error(err))
		}
	}()
	return nil
}

// Shutdown drains in-flight requests and waits for the serve goroutine to exit.
func (s *Server) Shutdown(ctx context.Context) error {
	defer s.closeEvals()
	s.startMu.Lock()
	done := s.serveDone
	s.startMu.Unlock()

	if done == nil {
		return nil
	}

	err := s.server.Shutdown(ctx)
	<-done
	return err
}

func (s *Server) initHandler() error {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /rpc", s.rpcHandler)
	mux.HandleFunc("POST /api/data/{method}", s.arrowHandler)
	mux.HandleFunc("GET /api/features", s.featureConfig)
	discovery, err := openRPCHandler()
	if err != nil {
		return err
	}
	mux.Handle("GET /openrpc.json", discovery)
	httpDiscovery, err := openAPIHandler()
	if err != nil {
		return err
	}
	mux.Handle("GET /openapi.json", httpDiscovery)
	mux.Handle("GET /llms.txt", llmsTextHandler())
	mux.HandleFunc("GET /export/{signal}/{id}", s.exportHandler)

	// Single-page app: serve a static asset when one exists at the request path,
	// otherwise fall back to index.html so client-side routes (/traces,
	// /traces/{id}, /metrics, /logs) resolve on hard load, refresh, and shared links.
	fsys, err := staticFS()
	if err != nil {
		return err
	}
	spa := spaHandler(fsys, s.logger)
	mux.Handle("/", spa)
	s.registerEvals(mux, spa)

	// CORS for the Vite frontend
	c := cors.New(cors.Options{
		AllowedOrigins: []string{"http://*", "https://*"},
		AllowedMethods: []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders: []string{"*"},
		ExposedHeaders: []string{"X-Arrow-Rows"},
	})
	s.server.Handler = c.Handler(mux)
	return nil
}

func llmsTextHandler() http.Handler {
	guide := skills.Guide()
	sum := sha256.Sum256([]byte(guide))
	etag := `"` + hex.EncodeToString(sum[:16]) + `"`

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		writer.Header().Set("Cache-Control", revalidateCacheControl)
		writer.Header().Set("ETag", etag)
		http.ServeContent(writer, request, "llms.txt", time.Time{}, strings.NewReader(guide))
	})
}

// spaHandler serves the single-page app. A GET/HEAD for an existing file is served
// as-is. Extension-less paths with no matching file fall back to index.html so the
// client router owns the route (e.g. a deep-linked /traces/{id} or a refreshed
// /metrics). Paths with a file extension that do not exist 404 normally. Non-GET
// requests fall through to the file server.
// Cache policy for the embedded frontend. Two populations, two rules.
//
// Everything under assets/ is content-hashed by Vite, so its URL changes
// whenever its bytes do and it can be cached forever. index.html is the
// opposite: a stable name that references those hashed files. It must be
// revalidated so upgrades cannot leave it pointing at absent assets. embed.FS
// supplies no Last-Modified value, so index.html gets an explicit ETag.
const (
	immutableCacheControl  = "public, max-age=31536000, immutable"
	revalidateCacheControl = "no-cache"
)

func spaHandler(fsys fs.FS, logger *zap.Logger) http.Handler {
	fileServer := http.FileServerFS(fsys)

	// Embedded bytes are immutable, so compute the validator once.
	indexBytes, indexErr := fs.ReadFile(fsys, "index.html")
	indexETag := ""
	if indexErr == nil {
		indexBytes = bytes.Replace(indexBytes, []byte("</head>"), []byte(`<link rel="service-desc" type="application/json" href="/openrpc.json"><link rel="service-desc" type="application/vnd.oai.openapi+json;version=3.1" href="/openapi.json"><link rel="help" type="text/plain" href="/llms.txt"></head>`), 1)
		sum := sha256.Sum256(indexBytes)
		indexETag = `"` + hex.EncodeToString(sum[:16]) + `"`
	}

	serveIndex := func(writer http.ResponseWriter, request *http.Request) {
		if indexErr != nil {
			logger.Error("reading index.html", zap.Error(indexErr))
			http.Error(writer, "Failed to load page", http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		writer.Header().Set("Cache-Control", revalidateCacheControl)
		writer.Header().Set("ETag", indexETag)
		// ServeContent rather than Write, for the conditional request handling:
		// it answers a matching If-None-Match with 304 and no body. The zero
		// mod time is what kept Last-Modified out; the ETag above replaces it.
		http.ServeContent(writer, request, "index.html", time.Time{}, bytes.NewReader(indexBytes))
	}

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/" {
			writer.Header().Add("Link", `</openrpc.json>; rel="service-desc"; type="application/json", </openapi.json>; rel="service-desc"; type="application/vnd.oai.openapi+json;version=3.1", </llms.txt>; rel="help"; type="text/plain"`)
		}
		if request.Method == http.MethodGet || request.Method == http.MethodHead {
			name := strings.TrimPrefix(path.Clean(request.URL.Path), "/")
			if info, err := fs.Stat(fsys, name); name == "" || err != nil || info.IsDir() {
				if path.Ext(name) == "" {
					serveIndex(writer, request)
					return
				}
			}
			// Keyed on the assets/ prefix rather than "anything the file
			// server handles", because a direct GET /index.html lands here
			// too -- and that is the one file this header must never reach.
			if strings.HasPrefix(name, "assets/") {
				writer.Header().Set("Cache-Control", immutableCacheControl)
			} else {
				writer.Header().Set("Cache-Control", revalidateCacheControl)
			}
		}
		fileServer.ServeHTTP(writer, request)
	})
}

func (s *Server) rpcHandler(writer http.ResponseWriter, request *http.Request) {
	requestCtx := (propagation.TraceContext{}).Extract(request.Context(), propagation.HeaderCarrier(request.Header))
	ctx, endRPC := s.tel.RPCRequest(requestCtx)
	var method string
	var responseBytes int
	var rpcError error
	defer func() { endRPC(method, responseBytes, rpcError) }()

	bodyLimit := int64(maxRPCRequestBodyBytes)
	limited := http.MaxBytesReader(writer, request.Body, bodyLimit)
	body, err := io.ReadAll(limited)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			rpcError = fmt.Errorf("%w: request body too large", jsonrpc2.ErrInvalidRequest)
			responseBytes = s.sendJSONRPCResponse(ctx, writer, jsonrpc2.ID{}, nil, rpcError)
			return
		}
		rpcError = jsonrpc2.ErrInternal
		responseBytes = s.sendJSONRPCResponse(ctx, writer, jsonrpc2.ID{}, nil, rpcError)
		return
	}

	// encoding/json replaces invalid UTF-8 silently. Reject malformed requests
	// before decoding.
	if !utf8.Valid(body) {
		rpcError = jsonrpc2.ErrParse
		responseBytes = s.sendJSONRPCResponse(ctx, writer, jsonrpc2.ID{}, nil, rpcError)
		return
	}
	message, err := jsonrpc2.DecodeMessage(body)
	if err != nil {
		rpcError = jsonrpc2.ErrParse
		responseBytes = s.sendJSONRPCResponse(ctx, writer, jsonrpc2.ID{}, nil, rpcError)
		return
	}

	rpcRequest, ok := message.(*jsonrpc2.Request)
	if !ok {
		rpcError = jsonrpc2.ErrInvalidRequest
		responseBytes = s.sendJSONRPCResponse(ctx, writer, jsonrpc2.ID{}, nil, rpcError)
		return
	}

	// DuckDB builds response JSON inside the query; EncodeMessage is the only
	// separately measured encoding step.
	method = rpcRequest.Method

	result, err := s.jsonrpcHandler.Handle(ctx, rpcRequest)
	if err != nil {
		rpcError = err
		responseBytes = s.sendJSONRPCResponse(ctx, writer, rpcRequest.ID, nil, err)
		return
	}

	responseBytes = s.sendJSONRPCResponse(ctx, writer, rpcRequest.ID, result, nil)
}

// sendJSONRPCResponse writes the response and returns its encoded size in
// bytes, which the caller records on the RPC span. Returns 0 when the response
// could not be produced. ctx carries the RPC span, so the encode child span
// hangs off the call it belongs to.
func (s *Server) sendJSONRPCResponse(ctx context.Context, writer http.ResponseWriter, id jsonrpc2.ID, result any, rpcError error) int {
	response, err := jsonrpc2.NewResponse(id, result, rpcError)
	if err != nil {
		s.logger.Error("creating JSON-RPC response", zap.Error(err))
		http.Error(writer, "Internal server error", http.StatusInternalServerError)
		return 0
	}

	// DuckDB query time includes JSON construction, so only EncodeMessage is
	// measured separately.
	endEncode := s.tel.RPCEncode(ctx)
	bytes, err := jsonrpc2.EncodeMessage(response)
	endEncode(err)
	if err != nil {
		s.logger.Error("encoding JSON-RPC response", zap.Error(err))
		http.Error(writer, "Internal server error", http.StatusInternalServerError)
		return 0
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.Write(bytes)
	return len(bytes)
}
