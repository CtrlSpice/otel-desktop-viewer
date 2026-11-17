package searchindex

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// FieldValuesSQL selects allowlisted columns only. Request field names never
// become SQL identifiers. Both transports share matching, ranking and limits.
func FieldValuesSQL(signal, field, term string, limit int64) (string, []any, error) {
	var table, column, order string
	switch signal + "/" + field {
	case "traces/name":
		table, column, order = "spans", "name", "count(*) desc, name"
	case "logs/eventName":
		table, column, order = "logs", "event_name", "count(*) desc, event_name"
	case "metrics/name":
		table, column, order = "metrics", "name", "name"
	case "metrics/unit":
		table, column, order = "metrics", "unit", "unit"
	default:
		return "", nil, fmt.Errorf("field %q has no value completion for %q", field, signal)
	}
	term = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(term)
	present := column + " <> ''"
	if signal == "metrics" && field == "name" {
		present = "true"
	}
	query := "select " + column + ` as value, row_number() over(order by ` + order + `) as ordinal
from ` + table + ` where ` + present + ` and ` + column + ` ilike '%' || $1 || '%' escape '\'
group by ` + column + ` order by ` + order + ` limit $2`
	return query, []any{term, limit}, nil
}

func FieldValuesJSON(ctx context.Context, db *sql.DB, query string, args []any) (json.RawMessage, error) {
	var raw []byte
	err := db.QueryRowContext(ctx, `select coalesce(to_json(list(value order by ordinal)), to_json([]))::varchar from (`+query+`) completions`, args...).Scan(&raw)
	return json.RawMessage(raw), err
}
