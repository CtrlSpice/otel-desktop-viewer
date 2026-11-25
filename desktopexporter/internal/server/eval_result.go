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
	"strings"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/evaldecode"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/evals"
	"go.opentelemetry.io/collector/pdata/plog"
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
	if evaldecode.ValidateResult(body) != nil {
		http.Error(w, "invalid Promptfoo result JSON", 400)
		return
	}
	provider, err := evals.LiveRowProvider(body, run, result)
	if err != nil {
		http.Error(w, "expected a Promptfoo result with the named id and a provider.id", 400)
		return
	}
	ctx := r.Context()
	// Hash a structured tuple: two strings must not collide at their boundary.
	group, _ := json.Marshal([]string{"live-result-v1", run, provider})
	id := resultHash(string(group))
	if _, busy := s.evalImports.LoadOrStore(id, struct{}{}); busy {
		http.Error(w, "publication for this model is in progress", 409)
		return
	}
	defer s.evalImports.Delete(id)
	var ordinal int64
	runKey, resultKey, digest := resultHash(run), resultHash(result), fmt.Sprintf("%x", sha256.Sum256(body))
	recordID := resultHash("live-row-v1:" + runKey + resultKey)
	data, err := evaldecode.Encode(recordID, evaldecode.IngestPart{Row: 0, Payload: body})
	if err != nil {
		http.Error(w, "invalid result", 400)
		return
	}
	log := data.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)
	log.SetEventName(evaldecode.ResultEvent)
	log.Attributes().PutStr("eval.run.id", run)
	log.Attributes().PutStr("eval.result.digest", digest)
	copy := plog.NewLogs()
	data.CopyTo(copy)
	normalizer, err := importRedactors()
	if err != nil || normalizer.logs.ConsumeLogs(ctx, copy) != nil {
		http.Error(w, "could not normalize result for readback", 500)
		return
	}
	expected, err := evaldecode.Decode(copy.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0))
	if err != nil {
		http.Error(w, "could not decode reference result", 500)
		return
	}
	retainedSHA, err := storeRead(s.store, func(db *sql.DB) (string, error) { return evals.ExpectedLiveSHA(ctx, db, expected.Payload) })
	if err != nil {
		http.Error(w, "could not compute reference digest", 500)
		return
	}
	log.Attributes().PutStr("eval.result.redacted_sha256", retainedSHA)
	err = s.store.WithDBWrite(func(db *sql.DB) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		var previous, previousSHA string
		err = tx.QueryRowContext(ctx, `select digest, coalesce(expected_sha,''), ordinal from eval_result_claims where run_key=? and result_key=?`, runKey, resultKey).Scan(&previous, &previousSHA, &ordinal)
		switch {
		case err == nil:
			if previous != digest || previousSHA != retainedSHA {
				return errResultConflict
			}
		case errors.Is(err, sql.ErrNoRows):
			if err := evals.IgnorePreclaimLiveEvidence(ctx, tx, recordID); err != nil {
				return err
			}
			if err := tx.QueryRowContext(ctx, `select coalesce(max(ordinal)+1, 0) from eval_result_claims where import_id=?`, id).Scan(&ordinal); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `insert into eval_result_claims (run_key,result_key,digest,import_id,ordinal,expected_sha) values (?,?,?,?,?,?)`, runKey, resultKey, digest, id, ordinal, retainedSHA); err != nil {
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
		_, reason, err := s.evalImportState(ctx, recordID)
		if err != nil {
			return false, err
		}
		if strings.HasPrefix(reason, "Conflicting") || strings.HasPrefix(reason, "Retained") || strings.HasPrefix(reason, "Invalid") {
			return false, errors.New("failed retained result verification")
		}
		return storeRead(s.store, func(db *sql.DB) (bool, error) {
			var found bool
			err := db.QueryRowContext(ctx, `select exists(select 1 from eval_datasets d join eval_imports i on i.id=d.import_id join eval_records r on r.import_id=d.import_id and r.envelope=d.envelope join eval_live_members m on m.dataset_id=d.id where d.import_id=? and m.group_id=? and m.ordinal=? and r.ordinal=0 and i.complete and not d.deleted)`, recordID, dataset, ordinal).Scan(&found)
			return found, err
		})
	}
	ok, err := visible(ctx)
	if err != nil {
		http.Error(w, "could not inspect retained result", 500)
		return
	}
	if !ok {
		log.Attributes().PutInt("eval.result.ordinal", ordinal)
		if err := s.forwardEvalLogs(ctx, data); err != nil {
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
	_ = json.NewEncoder(w).Encode(map[string]any{"datasetID": dataset, "resultID": result, "ordinal": ordinal, "complete": true, "retainedSHA256": retainedSHA})
}
