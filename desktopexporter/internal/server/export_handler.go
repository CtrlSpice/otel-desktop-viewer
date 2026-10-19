package server

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

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
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		http.Error(w, "invalid export query", http.StatusBadRequest)
		return
	}
	formats := query["format"]
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
	selection := make(map[string]string)
	for key, values := range query {
		if key == "format" {
			continue
		}
		if len(values) != 1 || values[0] == "" ||
			!(signal == "metrics" && (key == "seriesRef" || key == "datapointRef") || signal == "traces" && key == "spanId") {
			http.Error(w, "invalid export selection", http.StatusBadRequest)
			return
		}
		if key == "spanId" {
			if len(values[0]) != 16 {
				http.Error(w, "invalid span ID", http.StatusBadRequest)
				return
			}
			value, err := strconv.ParseUint(values[0], 16, 64)
			if err != nil {
				http.Error(w, "invalid span ID", http.StatusBadRequest)
				return
			}
			selection[key] = strconv.FormatUint(value, 10)
		} else {
			value, err := normalizeUUID(values[0])
			if err != nil {
				http.Error(w, "invalid export reference", http.StatusBadRequest)
				return
			}
			selection[key] = value
		}
	}
	data, err := storeRead(s.store, func(db *sql.DB) ([]byte, error) {
		if signal == "metrics" {
			return metrics.GetMetricSelectionOTLP(r.Context(), db, id, selection["seriesRef"], selection["datapointRef"])
		}
		if signal == "traces" {
			return spans.GetTraceSelectionOTLP(r.Context(), db, id, selection["spanId"])
		}
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
	for _, key := range []string{"spanId", "seriesRef", "datapointRef"} {
		if value := selection[key]; value != "" {
			filename = strings.TrimSuffix(filename, ".json") + "-" + key + "-" + value + ".json"
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	// ServeContent supports HEAD for size checks and byte ranges for explicitly
	// bounded inspector previews. Download requests still receive the full export.
	http.ServeContent(w, r, filename, time.Time{}, bytes.NewReader(data))
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
