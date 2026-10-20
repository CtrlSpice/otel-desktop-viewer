package server

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/evals"
	"golang.org/x/exp/jsonrpc2"
)

func isEvalSnapshotMethod(method string) bool {
	switch method {
	case "importEvalSnapshot", "searchEvalSnapshots", "getEvalSnapshot", "deleteEvalSnapshot":
		return true
	default:
		return false
	}
}

func (h *JSONRPCHandler) importEvalSnapshot(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	params, err := decodePositionalParams(req.Params, 4, 4)
	if err != nil {
		return nil, err
	}
	var values [4]string
	for i, param := range params {
		value, ok := param.(string)
		if !ok {
			return nil, jsonrpc2.ErrInvalidParams
		}
		values[i] = value
	}
	var result evals.ImportResult
	err = h.store.WithDBWrite(func(db *sql.DB) error {
		var err error
		result, err = evals.Import(ctx, db, values[0], values[1], values[2], values[3])
		return err
	})
	if err != nil {
		return nil, h.handleStoreError(ctx, err)
	}
	return result, nil
}

func (h *JSONRPCHandler) searchEvalSnapshots(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	// Absent params, null, [] and {} all select the default page.
	var params []any
	var err error
	if len(req.Params) > 0 {
		params, err = decodePositionalParams(req.Params, 0, 2)
	}
	if err != nil {
		return nil, err
	}
	limit, offset := int64(evals.DefaultLimit), int64(0)
	for i, target := range []*int64{&limit, &offset} {
		if len(params) > i && params[i] != nil {
			value, err := parseTimestampParam(params[i], []string{"limit", "offset"}[i])
			if err != nil {
				return nil, err
			}
			*target = value
		}
	}
	return handlerRead(ctx, h, func(db *sql.DB) (evals.Page, error) { return evals.Search(ctx, db, limit, offset) })
}

func (h *JSONRPCHandler) getEvalSnapshot(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	id, err := parseSingleStringParam(req.Params)
	if err != nil {
		return nil, err
	}
	return handlerRead(ctx, h, func(db *sql.DB) (evals.Snapshot, error) { return evals.Get(ctx, db, id) })
}

func (h *JSONRPCHandler) deleteEvalSnapshot(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	id, err := parseSingleStringParam(req.Params)
	if err != nil {
		return nil, err
	}
	var result evals.DeleteResult
	err = h.store.WithDBWrite(func(db *sql.DB) error {
		var err error
		result, err = evals.Delete(ctx, db, id)
		return err
	})
	if err != nil {
		return nil, h.handleStoreError(ctx, err)
	}
	return result, nil
}

func (s *Server) evalSnapshotDownload(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.RawQuery != "" {
		http.Error(w, "snapshot download takes no query parameters", http.StatusBadRequest)
		return
	}
	snapshot, err := storeRead(s.store, func(db *sql.DB) (evals.Snapshot, error) {
		return evals.Get(r.Context(), db, r.PathValue("id"))
	})
	if err != nil {
		switch {
		case errors.Is(err, evals.ErrInvalidSnapshot):
			http.Error(w, "invalid snapshot identifier", http.StatusBadRequest)
		case errors.Is(err, evals.ErrSnapshotNotFound):
			http.NotFound(w, r)
		default:
			if r.Context().Err() != nil {
				return
			}
			s.jsonrpcHandler.handleStoreError(r.Context(), err)
			http.Error(w, "could not read snapshot", http.StatusInternalServerError)
		}
		return
	}
	filename := "eval-snapshot-" + snapshot.ID + ".json"
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", "attachment; filename="+strconv.Quote(filename))
	// Full downloads, HEAD and ranges all use the original bytes, never a JSON
	// serializer. The store bounds each materialized artifact to 16 MiB.
	http.ServeContent(w, r, filename, time.Time{}, strings.NewReader(snapshot.OriginalJSON))
}
