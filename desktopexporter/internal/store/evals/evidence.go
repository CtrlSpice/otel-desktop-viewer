package evals

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/evaldecode"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"go.opentelemetry.io/collector/pdata/plog"
)

const evidenceFrom = `logs l join scopes s on s.id=l.scope_id where l.event_name='viewer.eval.import.v1' and s.name='otel-desktop-viewer.evaldecode' and s.version='1' and l.service_name='otel-desktop-viewer'`

var errUndiscoveredEvidence = errors.New("evaluation evidence arrived during reconstruction")

// ReconcileEvidence is called on explicit Evals reads and by an importing
// request waiting for persistence. The caller serializes calls. Each retained
// log is read separately; no whole transfer is materialized. All cache writes
// originate in retained logs, never in the HTTP upload or decoder staging file.
func ReconcileEvidence(ctx context.Context, db ExperimentDB) error {
	if err := db.WithDBWrite(func(conn *sql.DB) error { return ingest.InvalidateEvalEvidence(ctx, conn) }); err != nil {
		return err
	}
discover:
	for {
		var ids []string
		err := db.WithDBRead(func(conn *sql.DB) error {
			rows, err := conn.QueryContext(ctx, `select l.id::varchar from `+evidenceFrom+` and not exists(select 1 from eval_evidence e where e.log_id=l.id) order by l.id limit 64`)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					return err
				}
				ids = append(ids, id)
			}
			return rows.Err()
		})
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			break
		}
		for _, id := range ids {
			log, err := readEvidence(ctx, db, id)
			if err != nil {
				return err
			}
			importID := evaldecode.ImportID(log)
			if err := db.WithDBWrite(func(conn *sql.DB) error {
				tx, err := conn.BeginTx(ctx, nil)
				if err != nil {
					return err
				}
				defer tx.Rollback()
				// Retention may remove the record while it is decoded outside the lock.
				result, err := tx.ExecContext(ctx, `insert into eval_evidence select id, ? from logs where id=try_cast(? as uuid) on conflict do nothing`, importID, id)
				if err != nil {
					return err
				}
				n, err := result.RowsAffected()
				if err != nil {
					return err
				}
				if n == 0 || importID == "" {
					return tx.Commit()
				}
				_, err = tx.ExecContext(ctx, `insert into eval_transfers(id) values (?) on conflict(id) do update set dirty=true`, importID)
				if err != nil {
					return err
				}
				_, err = tx.ExecContext(ctx, `insert into eval_imports(id) values (?) on conflict(id) do update set complete=false`, importID)
				if err != nil {
					return err
				}
				return tx.Commit()
			}); err != nil {
				return err
			}
		}
	}
	for {
		var id string
		err := db.WithDBRead(func(conn *sql.DB) error {
			return conn.QueryRowContext(ctx, `select id from eval_transfers where dirty order by id limit 1`).Scan(&id)
		})
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		if err := rebuildEvidence(ctx, db, id); err != nil {
			// Restart bounded discovery rather than publishing an older snapshot.
			// Database calls honor ctx even under repeated concurrent arrivals.
			if errors.Is(err, errUndiscoveredEvidence) {
				goto discover
			}
			return err
		}
	}
}

func readEvidence(ctx context.Context, db ExperimentDB, id string) (plog.LogRecord, error) {
	var raw json.RawMessage
	err := db.WithDBRead(func(conn *sql.DB) error {
		var err error
		raw, err = logs.GetLogOTLP(ctx, conn, id)
		return err
	})
	if err != nil {
		return plog.NewLogRecord(), err
	}
	data, err := (&plog.JSONUnmarshaler{}).UnmarshalLogs(raw)
	if err != nil {
		return plog.NewLogRecord(), err
	}
	if data.LogRecordCount() != 1 {
		return plog.NewLogRecord(), fmt.Errorf("expected one retained evaluation log")
	}
	return data.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0), nil
}

func clearEvidenceCache(ctx context.Context, db ExperimentDB, id string) error {
	// Hide first, including when a rebuild is interrupted between write windows.
	if err := db.WithDBWrite(func(conn *sql.DB) error {
		_, err := conn.ExecContext(ctx, `update eval_imports set complete=false where id=?`, id)
		return err
	}); err != nil {
		return err
	}
	var datasets []string
	if err := db.WithDBRead(func(conn *sql.DB) error {
		rows, err := conn.QueryContext(ctx, `select id from eval_datasets where import_id=?`, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var dataset string
			if err := rows.Scan(&dataset); err != nil {
				return err
			}
			datasets = append(datasets, dataset)
		}
		return rows.Err()
	}); err != nil {
		return err
	}
	for _, dataset := range datasets {
		if err := PurgeExperimentIndex(ctx, db, dataset); err != nil {
			return err
		}
	}
	for _, table := range []string{"eval_records", "eval_parts"} {
		for {
			var n int64
			err := db.WithDBWrite(func(conn *sql.DB) error {
				result, err := conn.ExecContext(ctx, `delete from `+table+` where import_id=? and rowid in (select rowid from `+table+` where import_id=? limit 64)`, id, id)
				if err == nil {
					n, err = result.RowsAffected()
				}
				return err
			})
			if err != nil {
				return err
			}
			if n == 0 {
				break
			}
		}
	}
	return db.WithDBWrite(func(conn *sql.DB) error {
		_, err := conn.ExecContext(ctx, `update eval_datasets set context='{}'::json::variant, row_count=0, byte_size=0 where import_id=?`, id)
		return err
	})
}

func rebuildEvidence(ctx context.Context, db ExperimentDB, id string) error {
	if err := clearEvidenceCache(ctx, db, id); err != nil {
		return err
	}
	var physical int64
	if err := db.WithDBRead(func(conn *sql.DB) error {
		return conn.QueryRowContext(ctx, `select count(*) from eval_evidence where import_id=?`, id).Scan(&physical)
	}); err != nil {
		return err
	}
	last, reason, name := "", "", "Imported evaluation"
	envelopes := int64(-1)
	for {
		var ids []string
		if err := db.WithDBRead(func(conn *sql.DB) error {
			rows, err := conn.QueryContext(ctx, `select e.log_id::varchar from eval_evidence e join logs l on l.id=e.log_id where e.import_id=? and e.log_id::varchar > ? order by e.log_id limit 64`, id, last)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var logID string
				if err := rows.Scan(&logID); err != nil {
					return err
				}
				ids = append(ids, logID)
			}
			return rows.Err()
		}); err != nil {
			return err
		}
		if len(ids) == 0 {
			break
		}
		for _, logID := range ids {
			last = logID
			log, err := readEvidence(ctx, db, logID)
			if err != nil {
				return err
			}
			part, err := evaldecode.Decode(log)
			if err != nil {
				reason = "Retained evidence is invalid or changed by processing."
				continue
			}
			// Bytes describe the processed representation, never the original upload.
			part.Bytes = 0
			canonical, _ := json.Marshal(part)
			digest := fmt.Sprintf("%x", sha256.Sum256(canonical))
			duplicate := false
			if err := db.WithDBWrite(func(conn *sql.DB) error {
				var retained bool
				if err := conn.QueryRowContext(ctx, `select exists(select 1 from logs where id=try_cast(? as uuid))`, logID).Scan(&retained); err != nil {
					return err
				}
				if !retained {
					return fmt.Errorf("evaluation evidence expired during reconstruction; retry")
				}
				var previous string
				err := conn.QueryRowContext(ctx, `select digest from eval_parts where import_id=? and envelope=? and ordinal=?`, id, part.Envelope, part.Row).Scan(&previous)
				if err == nil {
					duplicate = true
					if previous != digest {
						reason = "Conflicting retained evidence for one logical source identity."
					}
					return nil
				}
				if err != sql.ErrNoRows {
					return err
				}
				_, err = conn.ExecContext(ctx, `insert into eval_parts values (?, ?, ?, ?)`, id, part.Envelope, part.Row, digest)
				if err != nil || part.Row == -2 {
					return err
				}
				part.Bytes = int64(len(part.Payload))
				return AppendParts(ctx, conn, id, name, []IngestPart{part})
			}); err != nil {
				return err
			}
			if !duplicate && part.Row == -2 {
				var manifest struct {
					Name string `json:"name"`
				}
				if err := json.Unmarshal(part.Payload, &manifest); err != nil || len(manifest.Name) == 0 || len(manifest.Name) > MaxNameBytes {
					reason = "Invalid retained import manifest."
				} else {
					name, envelopes = manifest.Name, part.Count
				}
			}
		}
	}
	if reason == "" {
		if envelopes < 1 {
			reason = "Waiting for retained import manifest."
		} else {
			var complete bool
			err := db.WithDBRead(func(conn *sql.DB) error {
				return conn.QueryRowContext(ctx, `select
				(select count(*) from eval_parts where import_id=? and ordinal=-1)=? and
				(select count(*) from eval_datasets d where d.import_id=? and d.envelope>=0 and d.envelope<?
				 and exists(select 1 from eval_parts p where p.import_id=d.import_id and p.envelope=d.envelope and p.ordinal=-1)
				 and d.row_count=(select count(*) from eval_records r where r.import_id=d.import_id and r.envelope=d.envelope)
				 and not exists(select 1 from eval_records r where r.import_id=d.import_id and r.envelope=d.envelope and (r.ordinal<0 or r.ordinal>=d.row_count)))=?
				 and not exists(select 1 from eval_parts where import_id=? and envelope>=?)`, id, envelopes, id, envelopes, envelopes, id, envelopes).Scan(&complete)
			})
			if err != nil {
				return err
			}
			if !complete {
				reason = "Waiting for complete retained envelope and result coverage."
			}
		}
	}
	if reason == "" {
		if err := db.WithDBWrite(func(conn *sql.DB) error {
			_, err := conn.ExecContext(ctx, `update eval_datasets set name=?, byte_size=octet_length(encode(to_json(context)::varchar))+(select coalesce(sum(byte_size),0) from eval_records r where r.import_id=eval_datasets.import_id and r.envelope=eval_datasets.envelope)+greatest(row_count-1,0) where import_id=?`, name, id)
			return err
		}); err != nil {
			return err
		}
		for n := int64(0); n < envelopes; n++ {
			dataset := DatasetID(id, int(n))
			var deleted bool
			if err := db.WithDBRead(func(conn *sql.DB) error {
				return conn.QueryRowContext(ctx, `select deleted from eval_datasets where id=?`, dataset).Scan(&deleted)
			}); err != nil {
				return err
			}
			if deleted {
				continue
			}
			if err := EnsureExperimentIndex(ctx, db, dataset, "dataset"); err != nil {
				return err
			}
			var status string
			if err := db.WithDBRead(func(conn *sql.DB) error {
				return conn.QueryRowContext(ctx, `select status from eval_experiment_indexes where dataset_id=?`, dataset).Scan(&status)
			}); err != nil {
				return err
			}
			if status != "ready" && status != "empty" {
				reason = "Retained evaluation cannot be indexed."
			}
		}
	}
	return db.WithDBWrite(func(conn *sql.DB) error {
		// Ingestion can retain more logs between reconstruction's write windows.
		// Tracking-table counts alone cannot establish a current snapshot. Check
		// relevant unregistered records while holding the publication write lock.
		undiscovered, err := hasUndiscoveredEvidence(ctx, conn, id)
		if err != nil {
			return err
		}
		if undiscovered {
			return errUndiscoveredEvidence
		}
		var unchanged bool
		if err := conn.QueryRowContext(ctx, `select (select count(*) from eval_evidence where import_id=?)=? and not exists(select 1 from eval_evidence e where e.import_id=? and not exists(select 1 from logs l where l.id=e.log_id))`, id, physical, id).Scan(&unchanged); err != nil {
			return err
		}
		if !unchanged {
			return fmt.Errorf("retained evaluation evidence changed during indexing; retry")
		}
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, `update eval_transfers set dirty=false, reason=? where id=?`, reason, id); err != nil {
			return err
		}
		if reason == "" {
			if _, err := tx.ExecContext(ctx, `update eval_imports set complete=true where id=?`, id); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}

// Called under the publication write lock. Page only IDs, then inspect one
// canonical stored body at a time; an unrelated large transfer must not make
// this check materialize all its source bodies. Every query is cancellable.
func hasUndiscoveredEvidence(ctx context.Context, db *sql.DB, importID string) (bool, error) {
	last := ""
	for {
		rows, err := db.QueryContext(ctx, `select l.id::varchar from `+evidenceFrom+` and l.id::varchar>? and not exists(select 1 from eval_evidence e where e.log_id=l.id) order by l.id limit 64`, last)
		if err != nil {
			return false, err
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return false, err
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return false, err
		}
		if len(ids) == 0 {
			return false, nil
		}
		for _, id := range ids {
			var matching bool
			// Stored map entries have unique keys, matching ImportID's lookup.
			err := db.QueryRowContext(ctx, `select exists(select 1 from logs l
			 where l.id=try_cast(? as uuid) and otel_value_kind(l.body.importID)='string'
			 and l.body.importID::varchar=?)`, id, importID).Scan(&matching)
			if err != nil {
				return false, err
			}
			if matching {
				return true, nil
			}
		}
		last = ids[len(ids)-1]
	}
}
