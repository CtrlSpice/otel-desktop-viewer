package server

import (
	"context"
	"database/sql"
	"errors"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"go.uber.org/zap"
)

func (s *Server) exportHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	signal := r.PathValue("signal")
	if signal != "traces" && signal != "logs" && signal != "metrics" {
		http.NotFound(w, r)
		return
	}
	formats := r.URL.Query()["format"]
	if len(formats) > 1 || (len(formats) == 1 && formats[0] != "json") {
		http.Error(w, "format must be json", http.StatusBadRequest)
		return
	}
	id, err := normalizeUUID(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid export identifier", http.StatusBadRequest)
		return
	}
	if signal == "traces" {
		id = strings.ReplaceAll(id, "-", "")
	}
	data, err := storeRead(s.store, func(db *sql.DB) ([]byte, error) {
		return readExport(r.Context(), db, signal, id)
	})
	if err != nil {
		switch {
		case errors.Is(err, spans.ErrTraceIDNotFound), errors.Is(err, logs.ErrLogRefNotFound), errors.Is(err, metrics.ErrMetricIDNotFound):
			http.Error(w, "export record not found", http.StatusNotFound)
		case errors.Is(err, metrics.ErrUnsupportedMetricType):
			http.Error(w, "unsupported stored Metric type", http.StatusUnprocessableEntity)
		default:
			if r.Context().Err() != nil {
				return
			}
			s.logger.Error("export failed", zap.String("signal", signal), zap.Error(err))
			http.Error(w, "could not reconstruct export", http.StatusInternalServerError)
		}
		return
	}
	filename := strings.TrimSuffix(signal, "s") + "-" + id + ".json"
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
}

func readExport(ctx context.Context, db *sql.DB, signal, id string) ([]byte, error) {
	switch signal {
	case "traces":
		return spans.GetTraceOTLP(ctx, db, id)
	case "logs":
		return logs.GetLogOTLP(ctx, db, id)
	default: // The handler has validated the signal as metrics.
		return metrics.GetMetricOTLP(ctx, db, id)
	}
}
