//go:build evals

package evals

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	storeevals "github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/evals"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
)

func evalExperimentHTTPError(w http.ResponseWriter, err error) {
	status := 500
	if errors.Is(err, storeevals.ErrInvalidEval) || errors.Is(err, search.ErrInvalidQuery) {
		status = 400
	}
	if errors.Is(err, storeevals.ErrEvalNotFound) {
		status = 404
	}
	http.Error(w, err.Error(), status)
}

func (h *Handler) searchEvalExperiments(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if err := h.reconcileEvalEvidence(r.Context()); err != nil {
		evalExperimentHTTPError(w, err)
		return
	}
	query := storeevals.ExperimentSearch{Outcomes: r.URL.Query()["outcome"], Sort: r.URL.Query().Get("sort"), Direction: r.URL.Query().Get("direction"), Limit: 50}
	if r.URL.Query().Has("q") {
		http.Error(w, "use the shared search criteria tree", 400)
		return
	}
	if criteria := r.URL.Query().Get("criteria"); criteria != "" {
		if len(criteria) > 64*1024 {
			http.Error(w, "search criteria too large", 400)
			return
		}
		var err error
		query.Criteria, err = search.ParseQueryTree(json.RawMessage(criteria))
		if err != nil {
			evalExperimentHTTPError(w, err)
			return
		}
	}
	if query.Sort == "" {
		query.Sort = "started"
	}
	if query.Direction == "" {
		if query.Sort == "name" {
			query.Direction = "asc"
		} else {
			query.Direction = "desc"
		}
	}
	for key, target := range map[string]*int64{"limit": &query.Limit, "offset": &query.Offset} {
		if value := r.URL.Query().Get(key); value != "" {
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				http.Error(w, "invalid "+key, 400)
				return
			}
			*target = parsed
		}
	}
	page, err := storeRead(h.store, func(db *sql.DB) (storeevals.ExperimentSearchPage, error) {
		return storeevals.SearchExperiments(r.Context(), db, query)
	})
	if err != nil {
		evalExperimentHTTPError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(page)
}

func (h *Handler) getEvalExperimentPage(w http.ResponseWriter, r *http.Request) {
	if err := h.reconcileEvalEvidence(r.Context()); err != nil {
		evalExperimentHTTPError(w, err)
		return
	}
	after, limit := int64(-1), int64(100)
	for key, target := range map[string]*int64{"after": &after, "limit": &limit} {
		if value := r.URL.Query().Get(key); value != "" {
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				http.Error(w, "invalid "+key, 400)
				return
			}
			*target = parsed
		}
	}
	if limit < 1 || limit > 100 {
		http.Error(w, "limit must be 1..100", 400)
		return
	}
	page, err := storeRead(h.store, func(db *sql.DB) (storeevals.ExperimentSourcePage, error) {
		return storeevals.GetExperimentPage(r.Context(), db, r.PathValue("id"), after, int(limit))
	})
	if err != nil {
		evalExperimentHTTPError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(page)
}
