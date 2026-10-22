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
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/evals"
)

// Upload/parsing touch only viewer-owned temporary storage, never the DB lock.
// Staging is deleted on every return. Only a completely validated file is written.
func (s *Server) ingestEvals(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !s.features.EvalsEnabled(r.Context()) {
		http.Error(w, "Evaluation import is disabled. Enable evals.enabled on the viewer server.", http.StatusForbidden)
		return
	}
	// Large Blob uploads are cancellation-bound, not capped by the ordinary
	// server's 30-second body deadline.
	_ = http.NewResponseController(w).SetReadDeadline(time.Time{})
	w.Header().Set("Cache-Control", "no-store")
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "Imported evaluation"
	}
	if !utf8.ValidString(name) || len(name) > evals.MaxNameBytes || strings.ContainsRune(name, 0) {
		http.Error(w, "name must be UTF-8 text of at most 256 bytes", 400)
		return
	}
	f, err := os.CreateTemp("", "viewer-eval-stage-*")
	if err != nil {
		http.Error(w, "could not create evaluation staging file", 500)
		return
	}
	defer os.Remove(f.Name())
	defer f.Close()
	hash := sha256.New()
	encoder := json.NewEncoder(io.MultiWriter(f, hash))
	encoder.SetEscapeHTML(false)
	envelopes := 0
	err = evals.ParseIngest(r.Context(), r.Body, func(part evals.IngestPart) error {
		if part.Row == -1 {
			envelopes++
		}
		return encoder.Encode(part)
	})
	if err != nil {
		http.Error(w, "Invalid evaluation input: "+err.Error(), 400)
		return
	}
	id := fmt.Sprintf("%x", hash.Sum(nil))
	// Deletion is intentional. A subsequent explicit import gets a fresh,
	// deterministic generation rather than returning deleted dataset IDs.
	for {
		var deleted int
		err = s.store.WithDBRead(func(db *sql.DB) error {
			return db.QueryRowContext(r.Context(), `select count(*) from eval_datasets where import_id = ? and deleted`, id).Scan(&deleted)
		})
		if err != nil {
			http.Error(w, "could not inspect evaluation import", 500)
			return
		}
		if deleted == 0 {
			break
		}
		id = fmt.Sprintf("%x", sha256.Sum256([]byte(id+":reimport")))
	}
	if _, active := s.evalImports.LoadOrStore(id, struct{}{}); active {
		http.Error(w, "this import is already in progress; retry after it finishes", 409)
		return
	}
	defer s.evalImports.Delete(id)
	// A pending copy with no live owner is from an interrupted request/process.
	var pending bool
	err = s.store.WithDBRead(func(db *sql.DB) error {
		var complete bool
		err := db.QueryRowContext(r.Context(), `select complete from eval_imports where id = ?`, id).Scan(&complete)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		pending = err == nil && !complete
		return err
	})
	if err != nil {
		http.Error(w, "could not inspect evaluation import", 500)
		return
	}
	if pending {
		s.discardEvalImport(id)
	}
	created := false
	err = s.store.WithDBWrite(func(db *sql.DB) error {
		var complete bool
		err := db.QueryRowContext(r.Context(), `select complete from eval_imports where id = ?`, id).Scan(&complete)
		if err == nil {
			if !complete {
				return fmt.Errorf("this import is already in progress or needs interrupted-import recovery")
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		_, err = db.ExecContext(r.Context(), `insert into eval_imports (id) values (?)`, id)
		created = err == nil
		return err
	})
	if err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	if created {
		published := false
		defer func() {
			if !published {
				s.discardEvalImport(id)
			}
		}()
		_, err = f.Seek(0, io.SeekStart)
		decoder := json.NewDecoder(f)
		for err == nil {
			parts := make([]evals.IngestPart, 0, 64)
			bytes := 0
			for len(parts) < 64 && bytes < evals.MaxRecordBytes {
				var part evals.IngestPart
				err = decoder.Decode(&part)
				if err != nil {
					break
				}
				bytes += len(part.Payload)
				parts = append(parts, part)
			}
			if err != nil && err != io.EOF {
				break
			}
			if len(parts) > 0 {
				writeErr := s.store.WithDBWrite(func(db *sql.DB) error { return evals.AppendParts(r.Context(), db, id, name, parts) })
				if writeErr != nil {
					err = writeErr
					break
				}
			}
			if err == io.EOF {
				err = nil
				break
			}
		}
		if err == nil {
			for n := 0; n < envelopes; n++ {
				if err = s.ensureEvalExperimentIndex(r.Context(), evals.DatasetID(id, n), "dataset"); err != nil {
					break
				}
			}
		}
		if err == nil {
			err = s.store.WithDBWrite(func(db *sql.DB) error { return evals.Publish(r.Context(), db, id) })
		}
		if err != nil {
			http.Error(w, "evaluation import did not complete: "+err.Error(), 500)
			return
		}
		published = true
	}
	ids := make([]string, envelopes)
	for n := range ids {
		ids[n] = evals.DatasetID(id, n)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ids": ids, "created": created, "complete": true})
}

// Cancellation cleanup uses short write windows too. A crash/cleanup timeout
// can leave hidden pending rows; they are never returned as completed datasets.
func (s *Server) discardEvalImport(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var datasets []string
	if err := s.store.WithDBRead(func(db *sql.DB) error {
		rows, err := db.QueryContext(ctx, `select id from eval_datasets where import_id=? and import_id in (select id from eval_imports where not complete)`, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			datasets = append(datasets, id)
		}
		return rows.Err()
	}); err != nil {
		return
	}
	for _, datasetID := range datasets {
		if err := evals.PurgeExperimentIndex(ctx, s.store, datasetID); err != nil {
			return
		}
	}
	for {
		var count int64
		err := s.store.WithDBWrite(func(db *sql.DB) error {
			result, err := db.ExecContext(ctx, `delete from eval_records where import_id = ? and import_id in (select id from eval_imports where not complete) and (envelope, ordinal) in (select envelope, ordinal from eval_records where import_id = ? limit 64)`, id, id)
			if err != nil {
				return err
			}
			count, err = result.RowsAffected()
			return err
		})
		if err != nil {
			return
		}
		if count == 0 {
			break
		}
	}
	_ = s.store.WithDBWrite(func(db *sql.DB) error {
		_, err := db.ExecContext(ctx, `delete from eval_datasets where import_id = ? and import_id in (select id from eval_imports where not complete)`, id)
		if err != nil {
			return err
		}
		_, err = db.ExecContext(ctx, `delete from eval_imports where id = ? and not complete`, id)
		return err
	})
}
