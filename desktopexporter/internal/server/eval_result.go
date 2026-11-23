//go:build evals

package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/evaldecode"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/evals"
)

var errResultConflict = errors.New("different contents already claimed for this result")

func resultHash(value string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(value))) }

func (s *Server) putEvalResult(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	run, result := r.PathValue("runID"), r.PathValue("resultID")
	if run == "" || result == "" {
		http.Error(w, "run and result IDs are required", 400)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, evaldecode.MaxRecordBytes))
	if err != nil {
		http.Error(w, "cannot read result within the evaluation record size limit", 400)
		return
	}
	var row struct {
		ID       string `json:"id"`
		Provider struct {
			ID string `json:"id"`
		} `json:"provider"`
	}
	if evaldecode.ValidateResult(body) != nil || json.Unmarshal(body, &row) != nil || row.ID != result || row.Provider.ID == "" {
		http.Error(w, "expected a Promptfoo result with the named id and a provider.id", 400)
		return
	}
	ctx := r.Context()
	// Hash a structured tuple: two strings must not collide at their boundary.
	group, _ := json.Marshal([]string{"live-result-v1", run, row.Provider.ID})
	id := resultHash(string(group))
	if _, busy := s.evalImports.LoadOrStore(id, struct{}{}); busy {
		http.Error(w, "publication for this model is in progress", 409)
		return
	}
	defer s.evalImports.Delete(id)
	var ordinal int64
	runKey, resultKey, digest := resultHash(run), resultHash(result), fmt.Sprintf("%x", sha256.Sum256(body))
	err = s.store.WithDBWrite(func(db *sql.DB) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		var previous string
		err = tx.QueryRowContext(ctx, `select digest, ordinal from eval_result_claims where run_key=? and result_key=?`, runKey, resultKey).Scan(&previous, &ordinal)
		switch {
		case err == nil:
			if previous != digest {
				return errResultConflict
			}
		case errors.Is(err, sql.ErrNoRows):
			if err := tx.QueryRowContext(ctx, `select coalesce(max(ordinal)+1, 0) from eval_result_claims where import_id=?`, id).Scan(&ordinal); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `insert into eval_result_claims values (?,?,?,?,?)`, runKey, resultKey, digest, id, ordinal); err != nil {
				return err
			}
		default:
			return err
		}
		return tx.Commit()
	})
	if errors.Is(err, errResultConflict) {
		http.Error(w, err.Error(), 409)
		return
	}
	if err != nil {
		http.Error(w, "could not claim evaluation result", 500)
		return
	}
	dataset := evals.DatasetID(id, 0)
	visible := func(ctx context.Context) (bool, error) {
		if err := s.reconcileEvalEvidence(ctx); err != nil {
			return false, err
		}
		return storeRead(s.store, func(db *sql.DB) (bool, error) {
			var found bool
			err := db.QueryRowContext(ctx, `select exists(select 1 from eval_datasets d join eval_imports i on i.id=d.import_id join eval_records r on r.import_id=d.import_id and r.envelope=d.envelope where d.id=? and r.ordinal=? and i.complete and not d.deleted)`, dataset, ordinal).Scan(&found)
			return found, err
		})
	}
	ok, err := visible(ctx)
	if err != nil {
		http.Error(w, "could not inspect retained result", 500)
		return
	}
	if !ok {
		if ordinal == 0 {
			header, _ := json.Marshal(map[string]any{"evalId": run, "config": map[string]any{}, "results": map[string]any{"version": 3, "results": []any{}}})
			manifest, _ := json.Marshal(map[string]any{"name": "Live evaluation", "liveGroup": id})
			for _, part := range []evaldecode.IngestPart{{Envelope: 0, Row: -1, Format: "promptfoo-v3", Payload: header}, {Envelope: -1, Row: -2, Count: 1, Payload: manifest}} {
				if err := s.forwardEvalPart(ctx, id, part); err != nil {
					http.Error(w, "receiver did not accept run metadata", 502)
					return
				}
			}
		}
		if err := s.forwardEvalPart(ctx, id, evaldecode.IngestPart{Row: ordinal, Payload: body}); err != nil {
			http.Error(w, "receiver did not accept result", 502)
			return
		}
		wait, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for {
			if ok, err = visible(wait); ok {
				break
			}
			if wait.Err() != nil {
				http.Error(w, "result persistence not confirmed", 504)
				return
			}
			if err != nil {
				http.Error(w, "could not verify retained result", 500)
				return
			}
			select {
			case <-wait.Done():
				http.Error(w, "result persistence not confirmed", 504)
				return
			case <-tick.C:
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"datasetID": dataset, "resultID": result, "ordinal": ordinal, "complete": true})
}
