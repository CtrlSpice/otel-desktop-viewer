package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/timerange"
	"github.com/CtrlSpice/otel-desktop-viewer/internal/discovery"
	"golang.org/x/exp/jsonrpc2"
)

type discoveryOptions struct {
	Kind            string            `json:"kind"`
	Signal          string            `json:"signal"`
	OwnerType       string            `json:"ownerType"`
	ServiceName     *string           `json:"serviceName"`
	Namespace       *string           `json:"namespace"`
	ServiceIdentity *string           `json:"serviceIdentity"`
	TargetIdentity  *string           `json:"targetIdentity"`
	Key             *string           `json:"key"`
	StartTime       *string           `json:"startTime"`
	EndTime         *string           `json:"endTime"`
	Limit           *uint64           `json:"limit"`
	Offset          uint64            `json:"offset"`
	Term            string            `json:"term"`
	SortBy          string            `json:"sortBy"`
	SortDirection   string            `json:"sortDirection"`
	Queries         *discoveryQueries `json:"queries"`
	ValueKind       string            `json:"valueKind"`
	ValueRef        string            `json:"valueRef"`
	Locale          string            `json:"locale"`
}

// A false predicate excludes a signal whose fields cannot satisfy the Home query.
// A null predicate selects all records in the time window.
type discoveryQueries struct {
	Traces  json.RawMessage `json:"traces"`
	Logs    json.RawMessage `json:"logs"`
	Metrics json.RawMessage `json:"metrics"`
}

func (h *JSONRPCHandler) discoverTelemetry(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	if options, home, err := homeDiscoveryOptions(req.Params); err != nil {
		return nil, err
	} else if home {
		return h.discoverHomeValues(ctx, options)
	}
	plan, err := discoveryPlanFor(req.Params, false)
	if err != nil {
		return nil, err
	}
	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		var result []byte
		err := db.QueryRowContext(ctx, plan.statement, plan.args...).Scan(&result)
		return result, err
	})
}

type discoveryPlan struct {
	statement string
	args      []any
}

func (h *JSONRPCHandler) getServiceLabels(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	ref, err := parseSingleIDParam(req.Params, jsonrpc2.ErrInvalidParams, func(value string) (string, error) {
		decoded, err := hex.DecodeString(value)
		if err != nil || len(decoded) != 32 {
			return "", jsonrpc2.ErrInvalidParams
		}
		return strings.ToLower(value), nil
	})
	if err != nil {
		return nil, err
	}
	return handlerRead(ctx, h, func(db *sql.DB) (map[string]string, error) {
		var name, namespace string
		err := db.QueryRowContext(ctx, `select name,namespace from search_services where ref=$1`, ref).Scan(&name, &namespace)
		if err == sql.ErrNoRows {
			return nil, ErrHomeValueUnavailable
		}
		return map[string]string{"serviceName": name, "serviceNamespace": namespace}, err
	})
}

func (h *JSONRPCHandler) getServiceCount(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	if err := validateNoParams(req.Params); err != nil {
		return nil, err
	}
	return handlerRead(ctx, h, func(db *sql.DB) (int64, error) {
		var count int64
		err := db.QueryRowContext(ctx, discovery.ServiceCount()).Scan(&count)
		return count, err
	})
}

func discoveryPlanFor(raw json.RawMessage, native bool) (*discoveryPlan, error) {
	var params []json.RawMessage
	if err := json.Unmarshal(raw, &params); err != nil || len(params) != 1 || bytes.Equal(bytes.TrimSpace(params[0]), []byte("null")) {
		return nil, jsonrpc2.ErrInvalidParams
	}
	var options discoveryOptions
	decoder := json.NewDecoder(bytes.NewReader(params[0]))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&options); err != nil {
		return nil, jsonrpc2.ErrInvalidParams
	}
	if (options.SortBy != "" && options.SortBy != "name" && options.SortBy != "frequency") ||
		(options.SortDirection != "" && options.SortDirection != "asc" && options.SortDirection != "desc") ||
		(options.Kind != "values" && (options.SortBy != "" || options.SortDirection != "")) {
		return nil, jsonrpc2.ErrInvalidParams
	}
	for _, bound := range []*string{options.StartTime, options.EndTime} {
		if bound != nil {
			if _, err := strconv.ParseUint(*bound, 10, 64); err != nil {
				return nil, jsonrpc2.ErrInvalidParams
			}
		}
	}
	limit := uint64(100)
	if options.Limit != nil {
		limit = *options.Limit
	}
	if limit < 1 || limit > 500 || options.Offset > 1_000_000_000 {
		return nil, jsonrpc2.ErrInvalidParams
	}
	query := discovery.Query{StartTime: options.StartTime, EndTime: options.EndTime, ServiceName: options.ServiceName, Namespace: options.Namespace, ServiceIdentity: options.ServiceIdentity, Native: native}
	if options.Kind == "connections" || options.Kind == "connection-spans" {
		if options.ServiceIdentity == nil || (options.Kind == "connection-spans" && options.TargetIdentity == nil) {
			return nil, jsonrpc2.ErrInvalidParams
		}
		if options.Queries == nil {
			options.Queries = &discoveryQueries{}
		}
	}
	var prefix string
	var args []any
	if options.Queries != nil {
		var err error
		prefix, args, err = discoverySearchSources(options)
		if err != nil {
			return nil, fmt.Errorf("%v: %w", err, jsonrpc2.ErrInvalidParams)
		}
		query.Sources = discovery.Sources{Spans: "home_spans", Logs: "home_logs", Metrics: "home_metrics"}
		if options.Kind == "services" && (len(bytes.TrimSpace(options.Queries.Metrics)) == 0 || bytes.Equal(bytes.TrimSpace(options.Queries.Metrics), []byte("null"))) {
			// Service counts already select the owning points by time. Without
			// a metric search expression there is no additional identity-level
			// eligibility to reconstruct through the latest-descriptor view.
			query.Sources.Metrics = ""
		}
	}
	var statement, search, order string
	builder := discovery.Builder{Offset: len(args)}
	switch options.Kind {
	case "connections":
		statement = builder.ServiceConnections(query)
		search = "entry::VARCHAR"
		order = "json_extract_string(entry, '$.source.serviceIdentity'), json_extract_string(entry, '$.target.serviceIdentity')"
	case "connection-spans":
		statement = builder.ServiceConnectionSpans(query, *options.TargetIdentity)
		search = "json_extract_string(entry, '$.name')"
		order = "json_extract_string(entry, '$.startTime')::UBIGINT, json_extract_string(entry, '$.traceID'), json_extract_string(entry, '$.spanID')"
	case "services":
		statement = builder.Services(query)
		search = "concat(json_extract_string(entry, '$.serviceNamespace'), ' ', json_extract_string(entry, '$.serviceName'))"
		order = "json_extract_string(entry, '$.serviceNamespace'), json_extract_string(entry, '$.serviceName'), json_extract_string(entry, '$.serviceIdentity')"
	case "keys", "values":
		records, err := builder.RecordsFrom(options.Signal, options.OwnerType, query)
		if err != nil {
			return nil, fmt.Errorf("%v: %w", err, jsonrpc2.ErrInvalidParams)
		}
		location := discovery.Location{Signal: options.Signal, OwnerType: options.OwnerType}
		if options.Kind == "keys" {
			statement = builder.Keys(query, location, records)
			search = "json_extract_string(entry, '$.key')"
			order = "json_extract_string(entry, '$.key'), json_extract_string(entry, '$.kind')"
		} else {
			if options.Key == nil {
				return nil, jsonrpc2.ErrInvalidParams
			}
			statement = builder.Values(query, location, records, *options.Key)
			search = "json_extract(entry, '$.value')::VARCHAR"
			order = "json_extract(entry, '$.count')::UBIGINT DESC, json_extract(entry, '$.value')::VARCHAR"
			if options.SortBy != "" || options.SortDirection != "" {
				direction := "DESC"
				if options.SortDirection == "asc" || (options.SortDirection == "" && options.SortBy == "name") {
					direction = "ASC"
				}
				field := "json_extract(entry, '$.count')::UBIGINT"
				if options.SortBy == "name" {
					field = "coalesce(json_extract_string(entry, '$.value.value'), json_extract(entry, '$.value')::VARCHAR)"
				}
				order = field + " " + direction + ", json_extract(entry, '$.value')::VARCHAR"
			}
		}
	default:
		return nil, jsonrpc2.ErrInvalidParams
	}
	if native {
		switch options.Kind {
		case "services":
			search = "(select concat(s.namespace, ' ', s.name) from search_services s where s.ref=entry.serviceIdentity)"
			order = "(select s.namespace from search_services s where s.ref=entry.serviceIdentity),(select s.name from search_services s where s.ref=entry.serviceIdentity),entry.serviceIdentity"
		case "connections":
			// Text filtering retains the established searchable representation,
			// including quoted nanosecond timestamps; output stays native.
			search = `to_json(struct_update(entry,
                source := struct_update(entry.source, lastSeen := entry.source.lastSeen::VARCHAR,
                    serviceName := (select name from search_services where ref=entry.source.serviceIdentity),
                    serviceNamespace := (select namespace from search_services where ref=entry.source.serviceIdentity)),
                target := struct_update(entry.target, lastSeen := entry.target.lastSeen::VARCHAR,
                    serviceName := (select name from search_services where ref=entry.target.serviceIdentity),
                    serviceNamespace := (select namespace from search_services where ref=entry.target.serviceIdentity))))::VARCHAR`
			order = "entry.source.serviceIdentity,entry.target.serviceIdentity"
		case "connection-spans":
			search = "entry.name"
			order = "entry.startTime,entry.traceID,entry.spanID"
		case "keys":
			search = "entry.key"
			order = "entry.key,entry.kind"
		case "values":
			search = "entry.identity"
			order = "entry.count DESC,entry.identity"
			if options.SortBy != "" || options.SortDirection != "" {
				direction := "DESC"
				if options.SortDirection == "asc" || (options.SortDirection == "" && options.SortBy == "name") {
					direction = "ASC"
				}
				field := "entry.count"
				if options.SortBy == "name" {
					field = "coalesce(json_extract_string(entry.identity, '$.value'), entry.identity)"
				}
				order = field + " " + direction + ",entry.identity"
			}
		}
	}
	if prefix == "" {
		prefix = "WITH "
	} else {
		prefix += ", "
	}
	filtered := "SELECT entry FROM discovered"
	if options.Term != "" {
		filtered += " WHERE contains(lower(" + search + "), lower(" + builder.Bind(options.Term) + "))"
	}
	statement = prefix + "discovered AS (" + statement + "), filtered AS (" + filtered + "), ordered AS (SELECT entry, row_number() over(ORDER BY " + order + ") AS ordinal FROM filtered ORDER BY " + order + " OFFSET " + strconv.FormatUint(options.Offset, 10) + " LIMIT " + strconv.FormatUint(limit+1, 10) + "), page AS (SELECT * FROM ordered ORDER BY ordinal LIMIT " + strconv.FormatUint(limit, 10) + ") "
	totals := options.Queries != nil && options.Kind == "services"
	if native {
		statement += "SELECT coalesce((SELECT list(entry ORDER BY ordinal) FROM page), []) AS items, (SELECT count(*) FROM ordered)>" + strconv.FormatUint(limit, 10) + " AS truncated"
		if totals {
			statement += ", struct_pack(home := (SELECT count(*) FROM filtered), traces := (SELECT count(DISTINCT trace_id) FROM home_spans), logs := (SELECT count(*) FROM home_logs), metrics := (SELECT count(*) FROM home_metrics)) AS totals"
		}
	} else {
		statement += "SELECT json_object('rows', coalesce((SELECT list(json_array(entry) ORDER BY ordinal) FROM page), []), 'truncated', (SELECT count(*) FROM ordered)>" + strconv.FormatUint(limit, 10)
		if totals {
			statement += ", 'totals', json_object('home', (SELECT count(*) FROM filtered), 'traces', (SELECT count(DISTINCT trace_id) FROM home_spans), 'logs', (SELECT count(*) FROM home_logs), 'metrics', (SELECT count(*) FROM home_metrics))"
		}
		statement += ", 'columns', json_array(json_object('name', 'entry', 'type', 'JSON')))::VARCHAR"
	}
	return &discoveryPlan{statement: statement, args: append(args, builder.Args()...)}, nil
}

func discoverySearchSources(options discoveryOptions) (string, []any, error) {
	var window timerange.TimeRange
	for index, bound := range []*string{options.StartTime, options.EndTime} {
		if bound == nil {
			continue
		}
		value, err := strconv.ParseUint(*bound, 10, 64)
		if err != nil {
			return "", nil, err
		}
		if index == 0 {
			window.Start = &value
		} else {
			window.End = &value
		}
	}
	signals := []struct {
		name  string
		table string
		query json.RawMessage
		build func(*search.QueryNode, timerange.TimeRange) (string, []any, error)
	}{
		{"home_spans", "spans", options.Queries.Traces, spans.MatchingRowsSQL},
		{"home_logs", "logs", options.Queries.Logs, logs.MatchingRowsSQL},
		{"home_metrics", "metrics", options.Queries.Metrics, metrics.MatchingRowsSQL},
	}
	ctes := make([]string, 0, len(signals))
	var args []any
	for _, signal := range signals {
		if bytes.Equal(bytes.TrimSpace(signal.query), []byte("false")) {
			ctes = append(ctes, signal.name+" AS (SELECT * FROM "+signal.table+" WHERE false)")
			continue
		}
		var tree *search.QueryNode
		if len(signal.query) > 0 && !bytes.Equal(bytes.TrimSpace(signal.query), []byte("null")) {
			if err := json.Unmarshal(signal.query, &tree); err != nil {
				return "", nil, err
			}
		}
		statement, params, err := signal.build(tree, window)
		if err != nil {
			return "", nil, err
		}
		ctes = append(ctes, signal.name+" AS ("+search.RebaseParameters(statement, len(args))+")")
		args = append(args, params...)
	}
	return "WITH " + strings.Join(ctes, ", "), args, nil
}
