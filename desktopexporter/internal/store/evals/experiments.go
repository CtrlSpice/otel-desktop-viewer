package evals

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type ExperimentSearch struct {
	Query     string
	Outcomes  []string
	Sort      string
	Direction string
	Limit     int64
	Offset    int64
}
type ExperimentIndexIssue struct {
	DatasetID string `json:"datasetID"`
	Kind      string `json:"kind"`
	Status    string `json:"status"`
	Reason    string `json:"reason"`
}
type ExperimentIndexState struct {
	Remaining     int64                  `json:"remaining"`
	Unsupported   int64                  `json:"unsupported"`
	Empty         int64                  `json:"empty"`
	Issues        []ExperimentIndexIssue `json:"issues"`
	HasMoreIssues bool                   `json:"hasMoreIssues"`
}
type ExperimentSearchPage struct {
	Experiments []ExperimentSummary  `json:"experiments"`
	Total       int64                `json:"total"`
	HasMore     bool                 `json:"hasMore"`
	Facets      []OutcomeCount       `json:"facets"`
	Indexing    ExperimentIndexState `json:"indexing"`
}

const visibleExperiments = `select r.* from eval_experiments r join eval_experiment_indexes i on i.dataset_id=r.dataset_id where i.status='ready' and i.source_kind='dataset' and r.format='promptfoo-v3' and exists(
select 1 from eval_datasets d join eval_imports f on f.id=d.import_id where d.id=r.dataset_id and f.complete and not d.deleted)`
const experimentColumns = `id, dataset_id, adapter_id, name, evaluation, source, format, started_at, pass_count, partial_count, fail_count, fault_count, unscored_count`

var outcomeColumns = []string{"pass_count", "partial_count", "fail_count", "fault_count", "unscored_count"}

func scanExperiment(row scanner) (ExperimentSummary, error) {
	var out ExperimentSummary
	var stamp sql.NullString
	var counts [5]int64
	err := row.Scan(&out.ID, &out.DatasetID, &out.AdapterExperimentID, &out.Name, &out.Evaluation, &out.Source, &out.Format, &stamp, &counts[0], &counts[1], &counts[2], &counts[3], &counts[4])
	if err == sql.ErrNoRows {
		return out, ErrEvalNotFound
	}
	if err != nil {
		return out, err
	}
	out.State = "Unknown"
	out.StateReason = "Received result rows do not establish experiment lifecycle or planned-slot coverage."
	out.AssessmentName = "Recorded outcome"
	if stamp.Valid {
		out.StartedAt = &stamp.String
	}
	out.Outcomes = []OutcomeCount{}
	for n, count := range counts {
		out.ResultCount += count
		if count > 0 {
			part := outcomeDefinitions[n]
			part.Count = count
			out.Outcomes = append(out.Outcomes, part)
		}
	}
	return out, nil
}

func SearchExperiments(ctx context.Context, db *sql.DB, query ExperimentSearch) (ExperimentSearchPage, error) {
	out := ExperimentSearchPage{Experiments: []ExperimentSummary{}, Facets: []OutcomeCount{}, Indexing: ExperimentIndexState{Issues: []ExperimentIndexIssue{}}}
	if query.Limit < 1 || query.Limit > 100 || query.Offset < 0 || query.Offset > MaxOffset || len(query.Query) > 1024 || len(query.Outcomes) > 5 {
		return out, fmt.Errorf("%w: invalid experiment query bounds", ErrInvalidEval)
	}
	if query.Sort != "name" && query.Sort != "started" {
		return out, fmt.Errorf("%w: sort must be name or started", ErrInvalidEval)
	}
	if query.Direction != "asc" && query.Direction != "desc" {
		return out, fmt.Errorf("%w: direction must be asc or desc", ErrInvalidEval)
	}
	where := "true"
	args := []any{}
	for _, word := range strings.Fields(query.Query) {
		where += ` and (contains(lower(v.name),lower(?)) or contains(lower(v.evaluation),lower(?)) or contains(lower(v.source),lower(?)) or exists(select 1 from eval_experiment_search x where x.experiment_id=v.id and contains(lower(x.text),lower(?))))`
		args = append(args, word, word, word, word)
	}
	var counts [5]int64
	err := db.QueryRowContext(ctx, `select count(*) filter(where pass_count>0),count(*) filter(where partial_count>0),count(*) filter(where fail_count>0),count(*) filter(where fault_count>0),count(*) filter(where unscored_count>0) from (`+visibleExperiments+`) v where `+where, args...).Scan(&counts[0], &counts[1], &counts[2], &counts[3], &counts[4])
	if err != nil {
		return out, err
	}
	for n, count := range counts {
		if count > 0 {
			part := outcomeDefinitions[n]
			part.Count = count
			out.Facets = append(out.Facets, part)
		}
	}
	if len(query.Outcomes) > 0 {
		clauses := []string{}
		for _, key := range query.Outcomes {
			found := false
			for n, part := range outcomeDefinitions {
				if key == part.Key {
					clauses = append(clauses, outcomeColumns[n]+">0")
					found = true
					break
				}
			}
			if !found {
				return out, fmt.Errorf("%w: unknown outcome %q", ErrInvalidEval, key)
			}
		}
		where += " and (" + strings.Join(clauses, " or ") + ")"
	}
	if err := db.QueryRowContext(ctx, `select count(*) from (`+visibleExperiments+`) v where `+where, args...).Scan(&out.Total); err != nil {
		return out, err
	}
	order := "name_key " + query.Direction + ", id asc"
	if query.Sort == "started" {
		order = "started_sort " + query.Direction + " nulls last, name_key asc, id asc"
	}
	rows, err := db.QueryContext(ctx, `select `+experimentColumns+` from (`+visibleExperiments+`) v where `+where+` order by `+order+` limit ? offset ?`, append(args, query.Limit, query.Offset)...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		experiment, err := scanExperiment(rows)
		if err != nil {
			rows.Close()
			return out, err
		}
		out.Experiments = append(out.Experiments, experiment)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	out.HasMore = query.Offset+int64(len(out.Experiments)) < out.Total
	err = db.QueryRowContext(ctx, `select count(*) filter(where `+experimentIndexNeedsWork+`),count(*) filter(where i.status='unsupported'),count(*) filter(where i.status='empty') from (`+experimentCandidates+`) c left join eval_experiment_indexes i on i.dataset_id=c.id`).Scan(&out.Indexing.Remaining, &out.Indexing.Unsupported, &out.Indexing.Empty)
	if err != nil {
		return out, err
	}
	issues, err := db.QueryContext(ctx, `select c.id,c.kind,i.status,i.reason from (`+experimentCandidates+`) c join eval_experiment_indexes i on i.dataset_id=c.id where i.status in ('unsupported','empty') order by c.id limit 26`)
	if err != nil {
		return out, err
	}
	defer issues.Close()
	for issues.Next() {
		var issue ExperimentIndexIssue
		if err := issues.Scan(&issue.DatasetID, &issue.Kind, &issue.Status, &issue.Reason); err != nil {
			return out, err
		}
		if len(out.Indexing.Issues) == 25 {
			out.Indexing.HasMoreIssues = true
			break
		}
		out.Indexing.Issues = append(out.Indexing.Issues, issue)
	}
	return out, issues.Err()
}

type ExperimentSourcePage struct {
	Summary     ExperimentSummary     `json:"summary"`
	ContextJSON string                `json:"contextJSON"`
	Rows        []ExperimentSourceRow `json:"rows"`
	NextAfter   int64                 `json:"nextAfter"`
	HasMore     bool                  `json:"hasMore"`
}
type ExperimentSourceRow struct {
	Ordinal int64  `json:"ordinal"`
	JSON    string `json:"json"`
}

// Source JSON is a bounded string in this typed DTO, so ordinary response.json()
// cannot round its numbers. This is not the whole-file escaped import transport.
func GetExperimentPage(ctx context.Context, db *sql.DB, id string, after int64, limit int) (ExperimentSourcePage, error) {
	out := ExperimentSourcePage{Rows: []ExperimentSourceRow{}, NextAfter: after}
	if err := ValidateID(id); err != nil {
		return out, err
	}
	if after < -1 || limit < 1 || limit > 100 {
		return out, fmt.Errorf("%w: after must be >= -1 and limit 1..100", ErrInvalidEval)
	}
	summary, err := scanExperiment(db.QueryRowContext(ctx, `select `+experimentColumns+` from (`+visibleExperiments+`) where id=?`, id))
	if err != nil {
		return out, err
	}
	out.Summary = summary
	if err := db.QueryRowContext(ctx, `select cast(context as varchar) from eval_experiment_indexes where dataset_id=? and source_kind='dataset' and status='ready'`, summary.DatasetID).Scan(&out.ContextJSON); err != nil {
		return out, err
	}
	rows, err := readExperimentSource(ctx, db, summary.DatasetID, id, after, limit)
	if err != nil {
		return out, err
	}
	for _, row := range rows {
		out.Rows = append(out.Rows, ExperimentSourceRow{row.Ordinal, string(row.Raw)})
		out.NextAfter = row.Ordinal
	}
	err = db.QueryRowContext(ctx, `select exists(select 1 from eval_experiment_rows where experiment_id=? and ordinal>?)`, id, out.NextAfter).Scan(&out.HasMore)
	return out, err
}
