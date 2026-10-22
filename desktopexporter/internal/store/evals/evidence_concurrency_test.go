package evals_test

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/evaldecode"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/evals"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/stretchr/testify/require"
)

type beforeEvidencePublication struct {
	*store.Store
	afterIndex func()
	fired      bool
}

func (s *beforeEvidencePublication) WithDBRead(fn func(*sql.DB) error) error {
	err := s.Store.WithDBRead(fn)
	if err != nil || s.fired {
		return err
	}
	var ready bool
	err = s.Store.WithDBRead(func(db *sql.DB) error {
		return db.QueryRow(`select exists(select 1 from eval_experiment_indexes where status='ready')`).Scan(&ready)
	})
	if err == nil && ready {
		s.fired = true
		s.afterIndex()
	}
	return err
}

func TestEvidenceArrivalBeforePublication(t *testing.T) {
	for _, kind := range []string{"conflict", "conflict-after-page", "duplicate", "other-transfer"} {
		t.Run(kind, func(t *testing.T) {
			s, err := store.NewStore(t.Context(), "", nil)
			require.NoError(t, err)
			defer s.Close()
			id := strings.Repeat("a", 64)
			put := func(id string, part evaldecode.IngestPart) error {
				data, err := evaldecode.Encode(id, part)
				if err != nil {
					return err
				}
				return s.WithConn(func(conn driver.Conn) error { return logs.Ingest(t.Context(), conn, data, s.FlushedIDs()) })
			}
			row := evaldecode.IngestPart{Payload: json.RawMessage(`{"provider":{"id":"p"},"promptId":"p","value":1}`)}
			require.NoError(t, put(id, row))
			require.NoError(t, put(id, evaldecode.IngestPart{Row: -1, Count: 1, Payload: json.RawMessage(`{"evalId":"e","config":{},"results":{"version":3,"results":[]}}`)}))
			require.NoError(t, put(id, evaldecode.IngestPart{Envelope: -1, Row: -2, Count: 1, Payload: json.RawMessage(`{"name":"fixture"}`)}))
			access := &beforeEvidencePublication{Store: s, afterIndex: func() {
				var complete bool
				require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
					return db.QueryRow(`select complete from eval_imports where id=?`, id).Scan(&complete)
				}))
				require.False(t, complete)
				incoming, incomingID := row, id
				if strings.HasPrefix(kind, "conflict") {
					incoming.Payload = json.RawMessage(`{"provider":{"id":"p"},"promptId":"p","value":2,"marker":"late-conflict"}`)
				}
				if kind == "other-transfer" {
					incomingID = strings.Repeat("b", 64)
				}
				// A separate writer retains the log after discovery/indexing, while
				// reconciliation is paused immediately before its publication check.
				done := make(chan error, 1)
				go func() {
					if kind == "conflict-after-page" {
						for range 65 {
							if err := put(strings.Repeat("b", 64), row); err != nil {
								done <- err
								return
							}
						}
					}
					err := put(incomingID, incoming)
					if err == nil && kind == "conflict-after-page" {
						err = s.WithDBWrite(func(db *sql.DB) error {
							// Fix the store-local UUID so the relevant arrival is after
							// a full page of unrelated observations, deterministically.
							_, err := db.Exec(`update logs set id='ffffffff-ffff-ffff-ffff-ffffffffffff' where contains(body::varchar,'late-conflict')`)
							return err
						})
					}
					done <- err
				}()
				require.NoError(t, <-done)
			}}
			require.NoError(t, evals.ReconcileEvidence(t.Context(), access))
			require.True(t, access.fired)
			require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
				var complete bool
				var reason string
				require.NoError(t, db.QueryRow(`select i.complete,t.reason from eval_imports i join eval_transfers t on t.id=i.id where i.id=?`, id).Scan(&complete, &reason))
				if strings.HasPrefix(kind, "conflict") {
					require.False(t, complete, "already-retained conflict must prevent publication")
					require.Contains(t, reason, "Conflicting")
				} else {
					require.True(t, complete)
				}
				if kind == "duplicate" {
					var n int
					require.NoError(t, db.QueryRow(`select count(*) from eval_evidence where import_id=?`, id).Scan(&n))
					require.Equal(t, 4, n, "discover the late duplicate before completing")
				}
				return nil
			}))
		})
	}
}
