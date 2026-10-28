package searchindex

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
)

type DiscoveryOptions struct {
	Term   string
	Signal string
	Owner  string
	Path   []any
	Kind   string
}
type discoveryMatch struct {
	Name    string            `json:"name"`
	Scope   string            `json:"attributeScope"`
	Owner   string            `json:"owner"`
	Path    []any             `json:"path"`
	Kind    string            `json:"type"`
	Count   int64             `json:"matchCount"`
	Samples []json.RawMessage `json:"sampleValues"`
	pathID  int64
}

// Resolve shared contexts once. A correlated resource_ref OR scope_ref join can
// become a nested-loop scan for every matching owner, even when its signal is
// already known. Distinct references permit a single equality hash join.
func discoveryOwners(signalParameter string) string {
	return `with discovery_context_refs as materialized (
	 select distinct unnest([resource_ref,scope_ref]) ref from search_owners where signal=` + signalParameter + `
	), discovery_owners as materialized (
	 select id,record_key from search_owners where ` + signalParameter + `='' or signal=` + signalParameter + `
	 union
	 select o.id,o.record_key from search_owners o join discovery_context_refs r using(ref) where o.signal=''
	) `
}

func NormalizeSignal(signal string) (string, error) {
	switch signal {
	case "", "span", "log", "metric", "experiment":
		return signal, nil
	case "traces":
		return "span", nil
	case "logs":
		return "log", nil
	case "metrics":
		return "metric", nil
	default:
		return "", fmt.Errorf("unknown search signal %q", signal)
	}
}

// Discover searches names/owners first. An exact path requests values across all
// applicable owners unless Owner narrows it. Omitted Signal includes all signals.
// Compact catalogue rows are scanned, never all native attribute maps.
func Discover(ctx context.Context, db *sql.DB, options DiscoveryOptions) (json.RawMessage, error) {
	return discover(ctx, db, options, false)
}
func discover(ctx context.Context, db *sql.DB, options DiscoveryOptions, allKeys bool) (json.RawMessage, error) {
	signal, err := NormalizeSignal(options.Signal)
	if err != nil {
		return nil, err
	}
	selected := len(options.Path) > 0
	if strings.TrimSpace(options.Term) == "" && !selected && !allKeys {
		return json.RawMessage("[]"), nil
	}
	pattern := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(options.Term) + "%"
	pathText := "null"
	if len(options.Path) > 0 {
		pathText = jsonText(options.Path)
	}
	args := []any{signal, options.Owner, pathText, options.Kind, pattern}
	filter := `p.key ilike $5`
	if selected {
		filter = `(v.value ilike $5 or ($5='%%' and v.kind in('map','array','empty')))`
	}
	q := discoveryOwners("$1") + `select p.id,p.key,p.attribute_scope,p.owner,p.path::varchar,p.kind,count(distinct v.id)
	 from search_paths p join search_postings l on l.path_id=p.id join discovery_owners o on o.id=l.owner_id join search_values v on v.id=l.value_id
	 where p.is_attribute
	 and ($2='' or p.owner=$2) and ($3='null' or p.path=$3::json) and ($4='' or p.kind=$4) and (` + filter + `)
	 group by p.id,p.key,p.attribute_scope,p.owner,p.path,p.kind order by count(distinct v.id) desc,p.key,p.attribute_scope,p.kind,p.path::varchar`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	out := []discoveryMatch{}
	for rows.Next() {
		m := discoveryMatch{Samples: []json.RawMessage{}}
		var path string
		if err := rows.Scan(&m.pathID, &m.Name, &m.Scope, &m.Owner, &path, &m.Kind, &m.Count); err != nil {
			rows.Close()
			return nil, err
		}
		if err := json.Unmarshal([]byte(path), &m.Path); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if selected {
		for i := range out {
			m := &out[i]
			r, err := db.QueryContext(ctx, discoveryOwners("$3")+`select min(o.record_key) from search_postings l join discovery_owners o on o.id=l.owner_id join search_values v on v.id=l.value_id
			 where l.path_id=$1 and (v.value ilike $2 or ($2='%%' and v.kind in('map','array','empty')))
		 group by v.identity,v.value order by v.value,v.identity limit 3`, m.pathID, pattern, signal)
			if err != nil {
				return nil, err
			}
			var samples []string
			for r.Next() {
				var record string
				if err := r.Scan(&record); err != nil {
					r.Close()
					return nil, err
				}
				samples = append(samples, record)
			}
			if err := r.Err(); err != nil {
				r.Close()
				return nil, err
			}
			r.Close()
			raw, err := nativeSamples(ctx, db, m.Owner, m.Path, samples, signal)
			if err != nil {
				return nil, err
			}
			m.Samples = raw
		}
	}
	return json.Marshal(out)
}

func Definitions(ctx context.Context, db *sql.DB, signal string) (json.RawMessage, error) {
	raw, err := discover(ctx, db, DiscoveryOptions{Signal: signal}, true)
	if err != nil {
		return nil, err
	}
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	for _, row := range rows {
		delete(row, "matchCount")
		delete(row, "sampleValues")
	}
	return json.Marshal(rows)
}

// Fetch the selected representatives together. Child views can expand nested
// records, so a query per representative repeats the same native payload scan.
func nativeSamples(ctx context.Context, db *sql.DB, owner string, path []any, records []string, signal string) ([]json.RawMessage, error) {
	if len(path) == 0 {
		return nil, fmt.Errorf("missing selected path")
	}
	if len(records) == 0 {
		return []json.RawMessage{}, nil
	}
	document, from, identity := "", "", "t.id::varchar"
	switch owner {
	case "span":
		from = "spans t"
		identity = "replace(t.trace_id::varchar,'-','')||':'||span_id_wire(t.span_id)"
		document = "t.attributes"
	case "event":
		from = "events t"
		document = "t.attributes"
	case "link":
		from = "links t"
		document = "t.attributes"
	case "log":
		from = "logs t"
		document = "t.attributes"
		if path[0] == "body" {
			document = "t.body"
		}
	case "metric":
		from = "metrics t"
		document = "t.metadata"
	case "datapoint":
		from = "metric_datapoints t"
		document = "t.attributes"
	case "exemplar":
		from = "exemplars t"
		document = "t.attributes"
	case "resource", "scope":
		var sources []string
		switch signal {
		case "span":
			sources = []string{"spans"}
		case "log":
			sources = []string{"logs"}
		case "metric":
			sources = []string{"metrics", "metric_datapoints"}
		default:
			sources = []string{"spans", "logs", "metrics", "metric_datapoints"}
		}
		for _, table := range sources {
			if from != "" {
				from += " union all "
			}
			from += "select " + owner + " as context from " + table
		}
		from = "(" + from + ") t"
		document = "t.context.attributes"
		identity = "t.context.id::varchar"
	default:
		return nil, fmt.Errorf("unsupported discovery owner %q", owner)
	}
	params := make([]search.NamedParam, len(records))
	wanted := make([]string, len(records))
	for i, record := range records {
		params[i] = search.NamedParam{Name: fmt.Sprintf("record%d", i), Value: record}
		wanted[i] = fmt.Sprintf("(%d,$%d)", i, i+1)
	}
	value, err := search.ResolveValue(document, path[1:], "", &params)
	if err != nil {
		return nil, err
	}
	args := make([]any, len(params))
	for i, p := range params {
		args[i] = p.Value
	}
	rows, err := db.QueryContext(ctx, `with representatives as (
	 select wanted.ordinal, `+value.ValueSQL+` as value from `+from+`
	 join (values `+strings.Join(wanted, ",")+`) wanted(ordinal,record) on `+identity+`=wanted.record
	 qualify row_number() over(partition by wanted.ordinal)=1
	) select otel_value_json(value)::varchar from representatives order by ordinal`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]json.RawMessage, 0, len(records))
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		out = append(out, json.RawMessage(raw))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) != len(records) {
		return nil, sql.ErrNoRows
	}
	return out, nil
}
