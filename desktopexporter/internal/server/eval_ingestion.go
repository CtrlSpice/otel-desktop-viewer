//go:build evals

package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/evaldecode"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/evals"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
)

// Staging is request-owned plaintext temporary storage, removed on return.
// Persistent source data travels exclusively through the configured receiver.
func (s *Server) ingestEvals(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !s.features.EvalsEnabled(r.Context()) {
		http.Error(w, "Evaluation import is disabled. Enable evals.enabled on the viewer server.", 403)
		return
	}
	_ = http.NewResponseController(w).SetReadDeadline(time.Time{})
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
	err = evaldecode.ParseIngest(r.Context(), r.Body, func(part evaldecode.IngestPart) error {
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
	for {
		var deleted int
		err = s.store.WithDBRead(func(db *sql.DB) error {
			return db.QueryRowContext(r.Context(), `select count(*) from eval_datasets where import_id=? and deleted`, id).Scan(&deleted)
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
	if err := s.reconcileEvalEvidence(r.Context()); err != nil {
		http.Error(w, "could not inspect retained evaluation evidence: "+err.Error(), 500)
		return
	}
	complete, _, err := s.evalImportState(r.Context(), id)
	if err != nil {
		http.Error(w, "could not inspect evaluation import", 500)
		return
	}
	created := !complete
	if !complete {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			http.Error(w, "could not read evaluation staging", 500)
			return
		}
		decoder := json.NewDecoder(f)
		for {
			var part evaldecode.IngestPart
			err = decoder.Decode(&part)
			if err == io.EOF {
				break
			}
			if err == nil {
				err = s.forwardEvalPart(r.Context(), id, part)
			}
			if err != nil {
				http.Error(w, "evaluation submission did not complete: "+err.Error(), 502)
				return
			}
		}
		manifest, _ := json.Marshal(map[string]string{"name": name})
		err = s.forwardEvalPart(r.Context(), id, evaldecode.IngestPart{Envelope: -1, Row: -2, Count: int64(envelopes), Payload: manifest})
		if err != nil {
			http.Error(w, "evaluation manifest submission failed: "+err.Error(), 502)
			return
		}
		// Receiver acceptance is not persistence confirmation. Cancellation stops
		// waiting, but already accepted telemetry can still complete later.
		ctx := r.Context()
		// Bound waiting for missing evidence, not the size of an otherwise valid
		// transfer. Projection itself remains request-cancellable and may take
		// longer than this timer for a large file.
		deadline := time.NewTimer(time.Minute)
		defer deadline.Stop()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			if err := s.reconcileEvalEvidence(ctx); err != nil {
				http.Error(w, "could not project retained evaluation evidence: "+err.Error(), 500)
				return
			}
			var reason string
			complete, reason, err = s.evalImportState(ctx, id)
			if err != nil {
				http.Error(w, "could not inspect evaluation persistence", 500)
				return
			}
			if complete {
				break
			}
			if strings.HasPrefix(reason, "Conflicting") || strings.HasPrefix(reason, "Invalid") || strings.HasPrefix(reason, "Retained") {
				http.Error(w, "evaluation import remains incomplete: "+reason, 422)
				return
			}
			select {
			case <-ctx.Done():
				http.Error(w, "evaluation persistence was not confirmed; accepted telemetry may still arrive; retry the same file", 504)
				return
			case <-deadline.C:
				http.Error(w, "evaluation persistence was not confirmed; accepted telemetry may still arrive; retry the same file", 504)
				return
			case <-ticker.C:
			}
		}
	}
	ids := make([]string, envelopes)
	for n := range ids {
		ids[n] = evals.DatasetID(id, n)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ids": ids, "created": created, "complete": true})
}

// The pre-redaction internal hop must never use environment-configured proxies.
// One client/connection pool belongs to each Server and is closed on shutdown.
func newEvalReceiverClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy:           nil,
			DialContext:     (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			MaxIdleConns:    2,
			IdleConnTimeout: 90 * time.Second,
		},
		Timeout:       30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func (s *Server) forwardEvalPart(ctx context.Context, id string, part evaldecode.IngestPart) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	logs, err := evaldecode.Encode(id, part)
	if err != nil {
		return err
	}
	logs.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0).SetObservedTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	data, err := plogotlp.NewExportRequestFromLogs(logs).MarshalProto()
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.evalReceiverURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-protobuf")
	response, err := s.evalReceiverClient.Do(req)
	if err != nil {
		return fmt.Errorf("local OTLP receiver: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil {
		return err
	}
	if response.StatusCode != 200 {
		return fmt.Errorf("local OTLP receiver returned HTTP %d", response.StatusCode)
	}
	if len(body) > 1<<20 {
		return fmt.Errorf("OTLP response exceeds 1 MiB")
	}
	reply := plogotlp.NewExportResponse()
	if strings.Contains(response.Header.Get("Content-Type"), "application/json") {
		err = reply.UnmarshalJSON(body)
	} else {
		err = reply.UnmarshalProto(body)
	}
	if err != nil {
		return fmt.Errorf("invalid OTLP response: %w", err)
	}
	if reply.PartialSuccess().RejectedLogRecords() != 0 || reply.PartialSuccess().ErrorMessage() != "" {
		return fmt.Errorf("local OTLP receiver reported partial success")
	}
	return nil
}

func (s *Server) reconcileEvalEvidence(ctx context.Context) error {
	select {
	case s.evalEvidenceLock <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-s.evalEvidenceLock }()
	if err := evals.ReconcileEvidence(ctx, s.store); err != nil {
		return err
	}
	// Cache-only repairs share the same cancellable gate as evidence rebuilds;
	// two HTTP requests must not concurrently rebuild one experiment index.
	candidate, err := storeRead(s.store, func(db *sql.DB) (evals.ExperimentIndexCandidate, error) {
		return evals.NextExperimentIndex(ctx, db)
	})
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	return evals.EnsureExperimentIndex(ctx, s.store, candidate.ID, candidate.Kind)
}

func (s *Server) evalImportState(ctx context.Context, id string) (bool, string, error) {
	var complete bool
	var reason string
	err := s.store.WithDBRead(func(db *sql.DB) error {
		err := db.QueryRowContext(ctx, `select i.complete, coalesce(t.reason,'') from eval_imports i left join eval_transfers t on t.id=i.id where i.id=?`, id).Scan(&complete, &reason)
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	})
	return complete, reason, err
}
