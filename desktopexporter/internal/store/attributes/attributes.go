// Package attributes answers questions about the attribute dictionary itself,
// rather than about the signals that reference it.
//
// The shared dictionary supports value-first discovery across traces, logs,
// and metrics without scanning each signal separately.
package attributes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var ErrAttributesStoreInternal = errors.New("attributes store internal error")

// maxSampleValues bounds the sample list per match. Enough to recognise what a
// key holds, few enough that a broad term does not return the dictionary.
const maxSampleValues = 3

// searchQuery derives scope from each owner array, then groups matching values.
//
// Matching is case-insensitive across values and keys because callers may have
// either one.
//
// matchCount counts distinct dictionary rows, not owners. It answers "how many
// different values of this key match", which is what tells you whether you have
// found one specific thing or a whole family. Owner counts would mean unnesting
// every array, which is exactly the cost this avoids.
//
// Results are ordered by match count, then name for stable ties.
const searchQuery = `
	with owner_ids(scope, id) as (
		select 'span', unnest(attribute_ids) from spans
		union all select 'event', unnest(attribute_ids) from events
		union all select 'link', unnest(attribute_ids) from links
		union all select 'log', unnest(attribute_ids) from logs
		union all select 'datapoint', unnest(attribute_ids) from metric_datapoints
		union all select 'exemplar', unnest(attribute_ids) from exemplars
		union all select 'metadata', unnest(metadata_ids) from metrics
		union all select 'resource', unnest(r.attribute_ids) from resources r
			where exists (select 1 from spans s where s.resource_id = r.id)
				or exists (select 1 from logs l where l.resource_id = r.id)
				or exists (select 1 from metrics m where m.resource_id = r.id)
		union all select 'scope', unnest(sc.attribute_ids) from scopes sc
			where exists (select 1 from spans s where s.scope_id = sc.id)
				or exists (select 1 from logs l where l.scope_id = sc.id)
				or exists (select 1 from metrics m where m.scope_id = sc.id)
	)

	select cast(coalesce(to_json(list(json_object(
		'name',           sub.key,
		'attributeScope', sub.scope,
		'type',           sub.type::varchar,
		'matchCount',     sub.match_count,
		'sampleValues',   sub.samples
	) order by sub.match_count desc, sub.key, sub.scope)), '[]') as varchar) as matches
	from (
		select a.key, o.scope, json_extract_string(a.value, '$.kind') as type,
			count(*) as match_count,
			list(a.value order by a.value)[1:` + sampleLimit + `] as samples
		from owner_ids o join attributes a on a.id = o.id
		where a.value::varchar ilike ? or a.key ilike ?
		group by a.key, o.scope, json_extract_string(a.value, '$.kind')
	) sub`

const sampleLimit = "3"

// Search returns the attribute keys whose values (or names) contain term,
// across every signal, with a few example values for each.
//
// An empty term returns an empty list rather than the whole dictionary: the
// caller is a search box, and a user who has typed nothing is not asking for
// everything.
func Search(ctx context.Context, db *sql.DB, term string) (json.RawMessage, error) {
	if strings.TrimSpace(term) == "" {
		return json.RawMessage("[]"), nil
	}

	// The term is matched as a substring. Escaping the LIKE metacharacters
	// keeps a literal % or _ in the search box from matching everything, which
	// is otherwise a confusing way to get the entire dictionary back.
	pattern := "%" + escapeLike(term) + "%"

	var raw []byte
	if err := db.QueryRowContext(ctx, searchQuery, pattern, pattern).Scan(&raw); err != nil {
		return nil, fmt.Errorf("Search: %w: %w", ErrAttributesStoreInternal, err)
	}
	if raw == nil {
		return json.RawMessage("[]"), nil
	}
	return json.RawMessage(raw), nil
}

// escapeLike neutralises the LIKE wildcards. DuckDB's default escape character
// is backslash, so the backslash itself has to go first or it would escape the
// escapes.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "%", `\%`)
	s = strings.ReplaceAll(s, "_", `\_`)
	return s
}
