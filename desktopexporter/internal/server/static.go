package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"go.uber.org/zap"
)

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
