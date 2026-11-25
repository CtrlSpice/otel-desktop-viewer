package evals

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/evaldecode"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
)

// HTTP validation and retained projection use the same result semantics.
func LiveRowProvider(raw json.RawMessage, run, result string) (string, error) {
	row := sourceMap(raw)
	provider := sourceText(sourceMap(row["provider"])["id"])
	if provider == "" || result == "" || sourceText(row["id"]) != result {
		return "", fmt.Errorf("result id or provider.id missing or mismatched")
	}
	if id, present := row["evaluationId"]; present && sourceText(id) != run {
		return "", fmt.Errorf("evaluationId differs from run ID")
	}
	_, err := (experimentProjector{evaluationID: run}).row(raw, 0)
	return provider, err
}

type liveMember struct {
	group, adapter, experiment string
	liveClaim
}

// Only a local PUT writes these values. Read them once per reconstruction.
type liveClaim struct {
	runKey, resultKey, digest, importGroup, expectedSHA string
	ordinal                                             int64
}

func readLiveClaim(ctx context.Context, db *sql.DB, id string) (*liveClaim, error) {
	var c liveClaim
	err := db.QueryRowContext(ctx, `select run_key, result_key, digest, import_id, coalesce(expected_sha,''), ordinal
		from eval_result_claims where sha256('live-row-v1:' || run_key || result_key)=?`, id).
		Scan(&c.runKey, &c.resultKey, &c.digest, &c.importGroup, &c.expectedSHA, &c.ordinal)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// Compare the same serialized VARIANT representation returned by the rows API.
// The expected bytes have already passed through the pinned redactor on a copy.
// Neither the original nor redacted row is included in errors.
func ExpectedLiveSHA(ctx context.Context, db *sql.DB, expected json.RawMessage) (string, error) {
	var want string
	if err := db.QueryRowContext(ctx, `select sha256(to_json(?::json::variant)::varchar)`, nativeJSON(expected)).Scan(&want); err != nil {
		return "", fmt.Errorf("could not compare retained result")
	}
	return want, nil
}

var errLiveClaimMismatch = errors.New("conflicting retained live result claim")
var errLiveReadback = errors.New("retained result failed readback verification")

// At the first PUT, register any result logs that arrived before this claim as
// observed but unassociated. A later rebuild must not adopt them as PUT evidence.
func IgnorePreclaimLiveEvidence(ctx context.Context, tx *sql.Tx, recordID string) error {
	_, err := tx.ExecContext(ctx, `insert into eval_evidence
		select l.id, '' from `+evidenceFrom+` and l.event_name='viewer.eval.result.v1'
		and otel_value_kind(l.body.importID)='string' and l.body.importID::varchar=?
		on conflict do nothing`, recordID)
	return err
}

func liveMemberFromLog(log plog.LogRecord, part IngestPart) (liveMember, IngestPart, error) {
	run, hasRun := log.Attributes().Get("eval.run.id")
	ordinal, hasOrdinal := log.Attributes().Get("eval.result.ordinal")
	digest, hasDigest := log.Attributes().Get("eval.result.digest")
	expected, hasExpected := log.Attributes().Get("eval.result.redacted_sha256")
	if !hasExpected || expected.Type() != pcommon.ValueTypeStr || ValidateID(expected.Str()) != nil {
		return liveMember{}, IngestPart{}, errLiveReadback
	}
	row := sourceMap(part.Payload)
	result := sourceText(row["id"])
	if !hasRun || !hasOrdinal || !hasDigest || run.Type() != pcommon.ValueTypeStr || run.Str() == "" || ordinal.Type() != pcommon.ValueTypeInt || ordinal.Int() < 0 || part.Row != 0 || part.Envelope != 0 || digest.Type() != pcommon.ValueTypeStr || ValidateID(digest.Str()) != nil {
		return liveMember{}, IngestPart{}, fmt.Errorf("invalid live result")
	}
	provider, err := LiveRowProvider(part.Payload, run.Str(), result)
	if err != nil {
		return liveMember{}, IngestPart{}, err
	}
	hash := func(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }
	m := liveMember{liveClaim: liveClaim{runKey: hash(run.Str()), resultKey: hash(result), digest: digest.Str(), ordinal: ordinal.Int(), expectedSHA: expected.Str()}}
	if hash("live-row-v1:"+m.runKey+m.resultKey) != evaldecode.ImportID(log) {
		return liveMember{}, IngestPart{}, fmt.Errorf("wrong live result identity")
	}
	tuple, _ := json.Marshal([]string{"live-result-v1", run.Str(), provider})
	m.importGroup = hash(string(tuple))
	m.group = DatasetID(m.importGroup, 0)
	var key bytes.Buffer
	e := json.NewEncoder(&key)
	e.SetEscapeHTML(false)
	_ = e.Encode([]string{m.group, "run", run.Str(), provider})
	m.adapter = javascriptJSONString(string(bytes.TrimSuffix(key.Bytes(), []byte("\n"))))
	m.experiment = hash("eval-run-v1:" + m.adapter)
	header, _ := json.Marshal(map[string]any{"evalId": run.Str(), "config": map[string]any{}, "results": map[string]any{"version": 3, "results": []any{}}})
	return m, IngestPart{Envelope: 0, Row: -1, Count: 1, Format: "promptfoo-v3", Payload: header}, nil
}

func (m liveMember) save(ctx context.Context, db *sql.DB, dataset string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Gate indexing, search and visibility on both the pre-receiver claim and
	// the exact serialized row that the detail endpoint would return.
	var verified bool
	if err = tx.QueryRowContext(ctx, `select exists(select 1 from eval_records r join eval_datasets d on r.import_id=d.import_id and r.envelope=d.envelope where d.id=? and r.ordinal=0 and sha256(to_json(r.payload)::varchar)=?)`, dataset, m.expectedSHA).Scan(&verified); err != nil {
		return err
	}
	if !verified {
		return errLiveReadback
	}
	var matches bool
	if err = tx.QueryRowContext(ctx, `select exists(select 1 from eval_result_claims where run_key=? and result_key=? and digest=? and import_id=? and ordinal=? and expected_sha=?)`, m.runKey, m.resultKey, m.digest, m.importGroup, m.ordinal, m.expectedSHA).Scan(&matches); err != nil {
		return err
	}
	if !matches {
		return errLiveClaimMismatch
	}
	if _, err = tx.ExecContext(ctx, `insert into eval_live_members values (?,?,?,?,?) on conflict(dataset_id) do update set group_id=excluded.group_id, ordinal=excluded.ordinal, adapter_id=excluded.adapter_id, experiment_id=excluded.experiment_id`, dataset, m.group, m.ordinal, m.adapter, m.experiment); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `update eval_datasets set source='live-result' where id=?`, dataset); err != nil {
		return err
	}
	return tx.Commit()
}

// Only persisted, indexed row datasets participate. Losing one source record
// removes it from the group without invalidating other results or their cursors.
const liveVisible = `select p.*, m.group_id, m.experiment_id as live_id, m.adapter_id as live_adapter, m.ordinal as live_ordinal
from (` + physicalExperiments + `) p join eval_live_members m on m.dataset_id=p.dataset_id`

const visibleExperiments = `select * from (` + physicalExperiments + `) where source<>'live-result'
union all select live_id as id,group_id as dataset_id,live_adapter as adapter_id,first(name order by live_ordinal) as name,
first(name_key order by live_ordinal) as name_key,first(evaluation order by live_ordinal) as evaluation,
'live-result' as source,'promptfoo-v3' as format,min(started_at) as started_at,min(started_sort) as started_sort,
min(live_ordinal) as first_ordinal,sum(pass_count) as pass_count,sum(partial_count) as partial_count,
sum(fail_count) as fail_count,sum(fault_count) as fault_count,sum(unscored_count) as unscored_count
from (` + liveVisible + `) group by live_id,group_id,live_adapter`

func livePage(ctx context.Context, db *sql.DB, out ExperimentSourcePage, after int64, limit int) (ExperimentSourcePage, error) {
	const from = ` from (` + liveVisible + `) v join eval_datasets d on d.id=v.dataset_id
join eval_records r on r.import_id=d.import_id and r.envelope=d.envelope and r.ordinal=0 where v.live_id=? and v.live_ordinal>?`
	// First select only row sizes so the normal byte limit applies to detail reads.
	rows, err := db.QueryContext(ctx, `select live_ordinal,r.byte_size`+from+` order by live_ordinal limit ?`, out.Summary.ID, after, limit)
	if err != nil {
		return out, err
	}
	end, total := after, int64(0)
	for rows.Next() {
		var n, size int64
		if err = rows.Scan(&n, &size); err != nil {
			rows.Close()
			return out, err
		}
		if end > after && total+size > MaxRecordBytes {
			break
		}
		end = n
		total += size
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if err = db.QueryRowContext(ctx, `select to_json(d.context)::varchar from (`+liveVisible+`) v join eval_datasets d on d.id=v.dataset_id where live_id=? limit 1`, out.Summary.ID).Scan(&out.ContextJSON); err != nil {
		return out, err
	}
	rows, err = db.QueryContext(ctx, `select live_ordinal,to_json(r.payload)::varchar`+from+` and live_ordinal<=? order by live_ordinal`, out.Summary.ID, after, end)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var r ExperimentSourceRow
		if err = rows.Scan(&r.Ordinal, &r.JSON); err != nil {
			rows.Close()
			return out, err
		}
		out.Rows = append(out.Rows, r)
		out.NextAfter = r.Ordinal
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	err = db.QueryRowContext(ctx, `select exists(select 1 from (`+liveVisible+`) where live_id=? and live_ordinal>?)`, out.Summary.ID, out.NextAfter).Scan(&out.HasMore)
	return out, err
}
