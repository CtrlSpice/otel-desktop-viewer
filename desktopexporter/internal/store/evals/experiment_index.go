package evals

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type ExperimentDB interface {
	WithDBRead(func(*sql.DB) error) error
	WithDBWrite(func(*sql.DB) error) error
}

type ExperimentIndexCandidate struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

// The database-backed workspace indexes native ingested datasets only.
const experimentCandidates = `select id, 'dataset' as kind from eval_datasets where not deleted and format='promptfoo-v3' and import_id in (select id from eval_imports where complete)`
const experimentIndexNeedsWork = `((i.status is null or i.status='pending' or (i.status='ready' and exists(
select 1 from eval_experiment_rows r left join eval_experiment_search s on s.dataset_id=r.dataset_id and s.ordinal=r.ordinal
where r.dataset_id=c.id and s.dataset_id is null))) or (i.status='ready' and exists(
select 1 from eval_experiments r where r.dataset_id=c.id and r.name <> coalesce(nullif(json_extract_string(r.adapter_id, '$[4]'), ''), r.evaluation))))`

func NextExperimentIndex(ctx context.Context, db *sql.DB) (ExperimentIndexCandidate, error) {
	var out ExperimentIndexCandidate
	err := db.QueryRowContext(ctx, `select c.id, c.kind from (`+experimentCandidates+`) c left join eval_experiment_indexes i on i.dataset_id = c.id where `+experimentIndexNeedsWork+` order by c.id limit 1`).Scan(&out.ID, &out.Kind)
	return out, err
}

func indexStatus(ctx context.Context, access ExperimentDB, id, kind, status, reason string, raw json.RawMessage) error {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	return access.WithDBWrite(func(db *sql.DB) error {
		_, err := db.ExecContext(ctx, `insert into eval_experiment_indexes values (?, ?, ?, ?, cast(? as json)) on conflict(dataset_id) do update set status=excluded.status, reason=excluded.reason, context=excluded.context`, id, kind, status, reason, string(raw))
		return err
	})
}

func clearExperimentIndex(ctx context.Context, access ExperimentDB, id string) error {
	for _, table := range []string{"eval_experiment_search", "eval_experiment_rows", "eval_experiments"} {
		key := "ordinal"
		if table == "eval_experiments" {
			key = "id"
		}
		for {
			var n int64
			err := access.WithDBWrite(func(db *sql.DB) error {
				result, err := db.ExecContext(ctx, `delete from `+table+` where dataset_id = ? and `+key+` in (select `+key+` from `+table+` where dataset_id = ? limit 64)`, id, id)
				if err != nil {
					return err
				}
				n, err = result.RowsAffected()
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
	return nil
}

// PurgeExperimentIndex removes only derived data, never a received source dataset.
func PurgeExperimentIndex(ctx context.Context, access ExperimentDB, id string) error {
	if err := clearExperimentIndex(ctx, access, id); err != nil {
		return err
	}
	return access.WithDBWrite(func(db *sql.DB) error {
		_, err := db.ExecContext(ctx, `delete from eval_experiment_indexes where dataset_id=?`, id)
		return err
	})
}

type sourceRow struct {
	Ordinal int64
	Raw     json.RawMessage
}

// readExperimentSource bounds payload materialization using byte sizes first. When experimentID
// is present it selects only that experiment, using a global-ordinal keyset cursor.
func readExperimentSource(ctx context.Context, db *sql.DB, id, experimentID string, after int64, limit int) ([]sourceRow, error) {
	from := `eval_records r join eval_datasets d on d.import_id=r.import_id and d.envelope=r.envelope`
	where := `d.id = ?`
	args := []any{id, after}
	where += ` and r.ordinal > ?`
	if experimentID != "" {
		where += ` and exists (select 1 from eval_experiment_rows m where m.dataset_id = ? and m.ordinal=r.ordinal and m.experiment_id=?)`
		args = append(args, id, experimentID)
	}
	sizes, err := db.QueryContext(ctx, `select r.ordinal, r.byte_size from `+from+` where `+where+` order by r.ordinal limit ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	end, bytes := after, int64(0)
	for sizes.Next() {
		var ordinal, size int64
		if err := sizes.Scan(&ordinal, &size); err != nil {
			sizes.Close()
			return nil, err
		}
		if bytes+size > MaxRecordBytes && end > after {
			break
		}
		bytes += size
		end = ordinal
	}
	err = sizes.Err()
	sizes.Close()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `select r.ordinal, cast(r.payload as varchar) from `+from+` where `+where+` and r.ordinal <= ? order by r.ordinal`, append(args, end)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []sourceRow{}
	for rows.Next() {
		var ordinal int64
		var raw string
		if err := rows.Scan(&ordinal, &raw); err != nil {
			return nil, err
		}
		out = append(out, sourceRow{ordinal, json.RawMessage(raw)})
	}
	return out, rows.Err()
}

func appendExperimentRows(ctx context.Context, db *sql.DB, rows []projectedExperimentRow) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, row := range rows {
		s := row.Summary
		counts := [5]int64{}
		counts[row.Outcome] = 1
		var started any
		if s.StartedAt != nil {
			stamp, _ := time.Parse(time.RFC3339Nano, *s.StartedAt)
			started = stamp.UTC()
		}
		_, err = tx.ExecContext(ctx, `insert into eval_experiments values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
on conflict(id) do update set pass_count=eval_experiments.pass_count+excluded.pass_count, partial_count=eval_experiments.partial_count+excluded.partial_count, fail_count=eval_experiments.fail_count+excluded.fail_count, fault_count=eval_experiments.fault_count+excluded.fault_count, unscored_count=eval_experiments.unscored_count+excluded.unscored_count`, s.ID, s.DatasetID, s.AdapterExperimentID, s.Name, naturalExperimentKey(s.Name), s.Evaluation, s.Source, s.Format, s.StartedAt, started, row.Ordinal, counts[0], counts[1], counts[2], counts[3], counts[4])
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `insert into eval_experiment_rows values (?, ?, ?)`, s.DatasetID, row.Ordinal, s.ID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `insert into eval_experiment_search values (?, ?, ?, ?)`, s.DatasetID, row.Ordinal, s.ID, row.SearchText)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// EnsureExperimentIndex performs projection outside the main store lock and writes
// bounded batches. The caller serializes indexing of the same dataset.
func EnsureExperimentIndex(ctx context.Context, access ExperimentDB, id, kind string) error {
	if kind != "dataset" {
		return fmt.Errorf("%w: only native eval datasets can be indexed", ErrInvalidEval)
	}
	// A source can be deleted while its bounded copy is being projected outside
	// the store lock. Never leave a newly recreated derived copy after deletion.
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var live bool
		err := access.WithDBRead(func(db *sql.DB) error {
			return db.QueryRowContext(cleanup, `select exists(select 1 from eval_datasets where id=? and not deleted)`, id).Scan(&live)
		})
		if err == nil && !live {
			_ = PurgeExperimentIndex(cleanup, access, id)
		}
	}()
	var status string
	err := access.WithDBRead(func(db *sql.DB) error {
		err := db.QueryRowContext(ctx, `select status from eval_experiment_indexes where dataset_id=?`, id).Scan(&status)
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	})
	if err != nil {
		return err
	}
	if status == "ready" {
		var missing bool
		err := access.WithDBRead(func(db *sql.DB) error {
			return db.QueryRowContext(ctx, `select exists(select 1 from (`+experimentCandidates+`) c left join eval_experiment_indexes i on i.dataset_id=c.id where c.id=? and `+experimentIndexNeedsWork+`)`, id).Scan(&missing)
		})
		if err != nil {
			return err
		}
		if !missing {
			return nil
		}
	}
	if status == "empty" || status == "unsupported" {
		return nil
	}
	if err := clearExperimentIndex(ctx, access, id); err != nil {
		return err
	}
	var metadata Metadata
	var rawContext string
	err = access.WithDBRead(func(db *sql.DB) error {
		return scanMetadata(db.QueryRowContext(ctx, `select `+metadataColumns+`, cast(context as varchar) from eval_datasets where id=? and not deleted`, id), &metadata, &rawContext)
	})
	if err != nil {
		return err
	}
	if err := indexStatus(ctx, access, id, kind, "pending", "", json.RawMessage(rawContext)); err != nil {
		return err
	}
	count, after := int64(0), int64(-1)
	projector := newExperimentProjector(metadata, json.RawMessage(rawContext))
	for {
		var rows []sourceRow
		err := access.WithDBRead(func(db *sql.DB) error {
			var err error
			rows, err = readExperimentSource(ctx, db, id, "", after, 64)
			return err
		})
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}
		projected := make([]projectedExperimentRow, 0, len(rows))
		for _, row := range rows {
			p, err := projector.row(row.Raw, row.Ordinal)
			if err != nil {
				return indexStatus(ctx, access, id, kind, "unsupported", err.Error(), json.RawMessage(rawContext))
			}
			projected = append(projected, p)
		}
		if err := access.WithDBWrite(func(db *sql.DB) error { return appendExperimentRows(ctx, db, projected) }); err != nil {
			return err
		}
		count += int64(len(rows))
		after = rows[len(rows)-1].Ordinal
	}
	status = "ready"
	reason := ""
	if count == 0 {
		status = "empty"
		reason = "No result rows; no experiment identities inferred."
	}
	return indexStatus(ctx, access, id, kind, status, reason, json.RawMessage(rawContext))
}
