package server

import (
	"bytes"
	"container/list"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode/utf16"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/arrowread"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
	"github.com/CtrlSpice/otel-desktop-viewer/internal/discovery"
	"github.com/google/uuid"
	"golang.org/x/exp/jsonrpc2"
)

type homeCounts struct {
	Traces  uint64 `json:"traces"`
	Metrics uint64 `json:"metrics"`
	Logs    uint64 `json:"logs"`
}
type homeKindCounts struct {
	Kind   string     `json:"kind"`
	Count  uint64     `json:"count"`
	Counts homeCounts `json:"counts"`
}
type homeValue struct {
	Identity          string               `json:"identity"` // Opaque exact reference, not display text or a hash.
	ValueRef          string               `json:"valueRef"` // Exact received representative for this scope/page.
	Kind              string               `json:"kind"`
	Preview           string               `json:"preview"`
	ColorHash         uint32               `json:"colorHash"`
	Counts            homeCounts           `json:"counts"`
	Count             uint64               `json:"count"`
	KindCount         uint64               `json:"kindCount"`
	Denominator       uint64               `json:"denominator"`
	RelativeFrequency float64              `json:"relativeFrequency"`
	FoundOn           []discovery.Location `json:"foundOn"`
}
type homeValuePage struct {
	Values       []homeValue      `json:"values"`
	Kinds        []homeKindCounts `json:"kinds"`
	Denominators homeCounts       `json:"denominators"`
	Denominator  uint64           `json:"denominator"`
	Total        uint64           `json:"total"`
	Truncated    bool             `json:"truncated"`
}

func homeDiscoveryOptions(raw json.RawMessage) (discoveryOptions, bool, error) {
	var params []json.RawMessage
	var options discoveryOptions
	if json.Unmarshal(raw, &params) != nil || len(params) != 1 {
		return options, false, nil
	}
	if json.Unmarshal(params[0], &options) != nil {
		return options, false, nil
	}
	if options.Kind != "home-values" && options.Kind != "home-value" {
		return options, false, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(params[0]))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&options); err != nil {
		return options, true, jsonrpc2.ErrInvalidParams
	}
	return options, true, nil
}

// References retain exact native bytes independently of the representative's
// source record. Retention/deletion cannot silently change Copy. This bounded,
// handler-owned LRU is also scoped to the current database handle. Evicted refs
// fail explicitly; callers must refetch discovery. Tokens are never reused,
// including after eviction or database replacement. Equality is exact, not hash
// equality. No table, persistence, or global store contract is introduced.
type homeReferenceKey struct{ identity, metadata, value string }
type homeReference struct {
	key             homeReferenceKey
	ref             string
	metadata, value []byte
}
type homeValueReferences struct {
	mu         sync.Mutex
	db         *sql.DB
	epoch      string
	next       uint64
	bytes      int
	order      list.List
	byRef      map[string]*list.Element
	byValue    map[homeReferenceKey]*list.Element
	byIdentity map[string]*list.Element
}

const homeReferenceBytes = 64 << 20
const homeReferenceEntries = 1024

func (c *homeValueReferences) database(db *sql.DB) {
	if c.db == db && c.byRef != nil {
		return
	}
	c.db, c.epoch, c.next, c.bytes = db, uuid.NewString(), 0, 0
	c.order.Init()
	c.byRef = make(map[string]*list.Element)
	c.byValue = make(map[homeReferenceKey]*list.Element)
	c.byIdentity = make(map[string]*list.Element)
}
func (c *homeValueReferences) retain(db *sql.DB, identity string, metadata, value []byte) (string, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.database(db)
	key := homeReferenceKey{identity, string(metadata), string(value)}
	if previous := c.byValue[key]; previous != nil {
		c.order.MoveToFront(previous)
		if c.byIdentity[identity] == nil {
			c.byIdentity[identity] = previous
		}
		canonical := c.byIdentity[identity]
		c.order.MoveToFront(canonical)
		return canonical.Value.(*homeReference).ref, previous.Value.(*homeReference).ref, nil
	}
	size := len(identity) + len(key.metadata) + len(key.value) + len(metadata) + len(value)
	if size > homeReferenceBytes {
		return "", "", fmt.Errorf("Home value exceeds exact-reference capacity")
	}
	for c.bytes+size > homeReferenceBytes || c.order.Len() >= homeReferenceEntries {
		last := c.order.Back()
		entry := last.Value.(*homeReference)
		delete(c.byRef, entry.ref)
		delete(c.byValue, entry.key)
		if c.byIdentity[entry.key.identity] == last {
			delete(c.byIdentity, entry.key.identity)
		}
		c.bytes -= len(entry.key.identity) + len(entry.key.metadata) + len(entry.key.value) + len(entry.metadata) + len(entry.value)
		c.order.Remove(last)
	}
	c.next++
	ref := "home:" + c.epoch + ":" + strconv.FormatUint(c.next, 10)
	entry := &homeReference{key: key, ref: ref, metadata: bytes.Clone(metadata), value: bytes.Clone(value)}
	element := c.order.PushFront(entry)
	c.byRef[ref], c.byValue[key] = element, element
	if c.byIdentity[identity] == nil {
		c.byIdentity[identity] = element
	}
	canonical := c.byIdentity[identity]
	c.order.MoveToFront(canonical)
	c.bytes += size
	return canonical.Value.(*homeReference).ref, ref, nil
}
func (c *homeValueReferences) resolve(db *sql.DB, ref string) (*homeReference, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.database(db)
	if element := c.byRef[ref]; element != nil {
		c.order.MoveToFront(element)
		return element.Value.(*homeReference), nil
	}
	return nil, ErrHomeValueUnavailable
}

func homeColorHash(owner, key, identity string) uint32 {
	s := "[" + queries.HomeCopyQuote(owner) + "," + queries.HomeCopyQuote(key) + "," + queries.HomeCopyQuote(identity) + "]"
	hash := uint32(2166136261)
	for _, unit := range utf16.Encode([]rune(s)) {
		hash = (hash ^ uint32(unit)) * 16777619
	}
	hash ^= hash >> 16
	hash *= 0x85ebca6b
	hash ^= hash >> 13
	hash *= 0xc2b2ae35
	return hash ^ (hash >> 16)
}

func homeValuesPlan(options discoveryOptions) (*discoveryPlan, error) {
	limit := uint64(25)
	if options.Limit != nil {
		limit = *options.Limit
	}
	if options.Key == nil || limit > 100 || options.Offset > 1_000_000_000 ||
		(options.SortBy != "" && options.SortBy != "name" && options.SortBy != "frequency") ||
		(options.SortDirection != "" && options.SortDirection != "asc" && options.SortDirection != "desc") {
		return nil, jsonrpc2.ErrInvalidParams
	}
	for _, bound := range []*string{options.StartTime, options.EndTime} {
		if bound != nil {
			if _, err := strconv.ParseUint(*bound, 10, 64); err != nil {
				return nil, jsonrpc2.ErrInvalidParams
			}
		}
	}
	if options.Queries == nil {
		options.Queries = &discoveryQueries{}
	}
	prefix, args, err := discoverySearchSources(options)
	if err != nil {
		return nil, fmt.Errorf("%v: %w", err, jsonrpc2.ErrInvalidParams)
	}
	query := discovery.Query{StartTime: options.StartTime, EndTime: options.EndTime, ServiceName: options.ServiceName,
		Namespace: options.Namespace, ServiceIdentity: options.ServiceIdentity,
		Sources: discovery.Sources{Spans: "home_spans", Logs: "home_logs", Metrics: "home_metrics"}}
	if options.StartTime == nil && options.EndTime == nil && (len(bytes.TrimSpace(options.Queries.Metrics)) == 0 || bytes.Equal(bytes.TrimSpace(options.Queries.Metrics), []byte("null"))) {
		// Without identity-level search/time eligibility, received records already
		// are the complete selected set. Do not scan the latest-descriptor view.
		query.Sources.Metrics = ""
	}
	var sources []string
	supported := false
	for _, signal := range []string{"logs", "metrics", "traces"} {
		records, err := discovery.RecordsFrom(signal, options.OwnerType, query)
		if err != nil {
			continue
		} // Owners are signal-specific.
		supported = true
		var submitted json.RawMessage
		switch signal {
		case "traces":
			submitted = options.Queries.Traces
		case "logs":
			submitted = options.Queries.Logs
		case "metrics":
			submitted = options.Queries.Metrics
		}
		if bytes.Equal(bytes.TrimSpace(submitted), []byte("false")) {
			continue
		}
		recordID := "struct_pack(trace_id := NULL::UUID, span_id := NULL::UBIGINT, id := s.record_id)"
		if signal == "traces" {
			recordID = "struct_pack(trace_id := s.record_id.trace_id, span_id := s.record_id.span_id, id := NULL::UUID)"
		}
		sources = append(sources, `SELECT `+discovery.Literal(signal)+` AS signal, `+recordID+` AS record_id,
            variant_to_parquet_variant(s.attributes[`+discovery.Literal(*options.Key)+`])::struct(metadata blob, value blob) AS parts
            FROM (`+records+`) s WHERE `+discovery.Predicate(query)+` AND variant_exists(s.attributes, `+discovery.Literal(*options.Key)+`)`)
	}
	if len(sources) == 0 {
		if !supported {
			return nil, jsonrpc2.ErrInvalidParams
		}
		return &discoveryPlan{statement: `SELECT NULL::VARCHAR, NULL::VARCHAR, NULL::BIGINT, NULL::VARCHAR,
            NULL::BLOB, NULL::BLOB, NULL::VARCHAR, 0::UBIGINT, 0::UBIGINT, 0::UBIGINT,
            '{"traces":0,"metrics":0,"logs":0}'::VARCHAR, '[]'::VARCHAR`}, nil
	}
	counts := `json_object('traces', coalesce(sum(n) FILTER (WHERE signal='traces'),0)::UBIGINT,
        'metrics', coalesce(sum(n) FILTER (WHERE signal='metrics'),0)::UBIGINT,
        'logs', coalesce(sum(n) FILTER (WHERE signal='logs'),0)::UBIGINT)`
	direction := "DESC"
	if options.SortDirection == "asc" || (options.SortDirection == "" && options.SortBy == "name") {
		direction = "ASC"
	}
	order := "count " + direction
	locale := options.Locale
	if locale == "" {
		locale = "en_US"
	}
	// ICU locale is a SQL constant, not SQL source; unknown locales use ICU's
	// normal fallback. The full Copy spelling supplies collation, not the preview.
	locale = strings.ReplaceAll(locale, "-", "_")
	if options.SortBy == "name" {
		order = "icu_sort_key(otel_home_value_text(parts.metadata, parts.value), " + discovery.Literal(locale) + ") " + direction
	}
	// Preserve the old stable merge's final ties: its first containing signal,
	// then that signal's descending frequency / binary identity insertion order.
	order += ", icu_sort_key(identity, " + discovery.Literal(locale) + "), first_signal, first_signal_count DESC, identity"
	filter := "true"
	if options.ValueKind != "" {
		filter += " AND kind=" + discovery.Literal(options.ValueKind)
	}
	if options.Term != "" {
		filter += " AND contains(lower(identity), lower(" + discovery.Literal(options.Term) + "))"
	}
	statement := prefix + `, received AS MATERIALIZED (` + strings.Join(sources, " UNION ALL ") + `),
    native_values AS (SELECT DISTINCT parts FROM received),
    identified AS MATERIALIZED (
        SELECT parts, otel_value_json(variant_bytes_to_variant(parts.metadata || parts.value))::VARCHAR AS identity,
            otel_value_kind(variant_bytes_to_variant(parts.metadata || parts.value)) AS kind FROM native_values
    ), owned AS (
        SELECT r.*, i.identity, i.kind FROM received r JOIN identified i ON r.parts IS NOT DISTINCT FROM i.parts
    ), denominators AS (
        SELECT signal, count(DISTINCT record_id) AS n FROM received GROUP BY signal
    ), kind_records AS (
        SELECT kind, signal, count(DISTINCT record_id) AS n FROM owned GROUP BY kind, signal
    ), kinds AS (
        SELECT kind, sum(n)::UBIGINT AS count, ` + counts + ` AS counts FROM kind_records GROUP BY kind
    ), signal_values AS (
        SELECT identity, kind, signal, count(DISTINCT record_id) AS n,
            first(parts ORDER BY record_id, parts) AS parts FROM owned GROUP BY identity, kind, signal
    ), value_counts AS (
        SELECT identity, kind, sum(n)::UBIGINT AS count, ` + counts + ` AS counts,
            first(parts ORDER BY CASE signal WHEN 'logs' THEN 0 WHEN 'metrics' THEN 1 ELSE 2 END) AS parts,
            min(CASE signal WHEN 'traces' THEN 0 WHEN 'metrics' THEN 1 ELSE 2 END) AS first_signal,
            first(n ORDER BY CASE signal WHEN 'traces' THEN 0 WHEN 'metrics' THEN 1 ELSE 2 END) AS first_signal_count
        FROM signal_values GROUP BY identity, kind
    ), filtered AS (SELECT * FROM value_counts WHERE ` + filter + `),
    page AS (SELECT *, row_number() over(ORDER BY ` + order + `) AS ordinal FROM filtered
        ORDER BY ` + order + ` OFFSET ` + strconv.FormatUint(options.Offset, 10) + ` LIMIT ` + strconv.FormatUint(limit, 10) + `),
    summary AS (SELECT (SELECT count(*) FROM filtered) AS total,
        coalesce((SELECT sum(n) FROM denominators),0)::UBIGINT AS denominator,
        (SELECT ` + counts + ` FROM denominators) AS denominators,
        coalesce((SELECT to_json(list(struct_pack(kind := kind, count := count, counts := counts) ORDER BY kind)) FROM kinds), '[]'::JSON) AS kinds)
    SELECT p.identity, p.kind, p.count, p.counts::VARCHAR, p.parts.metadata, p.parts.value,
        otel_native_column_preview(p.parts.metadata, p.parts.value) AS preview,
        coalesce(k.count,0), s.total, s.denominator, s.denominators::VARCHAR, s.kinds::VARCHAR
    FROM summary s LEFT JOIN page p ON true LEFT JOIN kinds k ON k.kind=p.kind ORDER BY p.ordinal`
	return &discoveryPlan{statement: statement, args: args}, nil
}

func (h *JSONRPCHandler) discoverHomeValues(ctx context.Context, options discoveryOptions) (any, error) {
	if options.Kind == "home-value" {
		return handlerRead(ctx, h, func(db *sql.DB) (map[string]string, error) {
			entry, err := h.homeValues.resolve(db, options.ValueRef)
			if err != nil {
				return nil, err
			}
			return map[string]string{"identity": entry.key.identity}, nil
		})
	}
	plan, err := homeValuesPlan(options)
	if err != nil {
		return nil, err
	}
	return handlerRead(ctx, h, func(db *sql.DB) (*homeValuePage, error) {
		rows, err := db.QueryContext(ctx, plan.statement, plan.args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		page := &homeValuePage{Values: []homeValue{}, Kinds: []homeKindCounts{}}
		for rows.Next() {
			var identity, kind, preview, counts sql.NullString
			var count sql.NullInt64
			var metadata, value []byte
			var denominators, kinds string
			var kindCount uint64
			if err := rows.Scan(&identity, &kind, &count, &counts, &metadata, &value, &preview, &kindCount, &page.Total, &page.Denominator, &denominators, &kinds); err != nil {
				return nil, err
			}
			if err := json.Unmarshal([]byte(denominators), &page.Denominators); err != nil {
				return nil, err
			}
			if err := json.Unmarshal([]byte(kinds), &page.Kinds); err != nil {
				return nil, err
			}
			if !identity.Valid {
				continue
			}
			ref, valueRef, err := h.homeValues.retain(db, identity.String, metadata, value)
			if err != nil {
				return nil, err
			}
			entry := homeValue{Identity: ref, ValueRef: valueRef, Kind: kind.String, Preview: preview.String, Count: uint64(count.Int64),
				KindCount: kindCount, Denominator: page.Denominator, ColorHash: homeColorHash(options.OwnerType, *options.Key, identity.String), FoundOn: []discovery.Location{}}
			if page.Denominator > 0 {
				entry.RelativeFrequency = float64(entry.Count) / float64(page.Denominator)
			}
			if err := json.Unmarshal([]byte(counts.String), &entry.Counts); err != nil {
				return nil, err
			}
			for _, source := range []struct {
				signal string
				count  uint64
			}{{"traces", entry.Counts.Traces}, {"metrics", entry.Counts.Metrics}, {"logs", entry.Counts.Logs}} {
				if source.count > 0 {
					entry.FoundOn = append(entry.FoundOn, discovery.Location{Signal: source.signal, OwnerType: options.OwnerType})
				}
			}
			page.Values = append(page.Values, entry)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		page.Truncated = options.Offset+uint64(len(page.Values)) < page.Total
		return page, nil
	})
}

func (h *JSONRPCHandler) homeValueArrow(ctx context.Context, options discoveryOptions) (*arrowread.Result, error) {
	if options.Kind != "home-value" || options.ValueRef == "" {
		return nil, jsonrpc2.ErrInvalidParams
	}
	return h.arrowRead(ctx, func(db *sql.DB) (*arrowread.Result, error) {
		entry, err := h.homeValues.resolve(db, options.ValueRef)
		if err != nil {
			return nil, err
		}
		return arrowread.Read(ctx, db, `SELECT variant_bytes_to_variant($1::BLOB || $2::BLOB) AS value, $3::VARCHAR AS identity`, entry.metadata, entry.value, entry.key.identity)
	})
}
