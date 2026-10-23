package server

import (
	"bytes"
	"context"
	"database/sql"
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
	Key             *string           `json:"key"`
	StartTime       *string           `json:"startTime"`
	EndTime         *string           `json:"endTime"`
	Limit           *uint64           `json:"limit"`
	Offset          uint64            `json:"offset"`
	Term            string            `json:"term"`
	SortBy          string            `json:"sortBy"`
	SortDirection   string            `json:"sortDirection"`
	Queries         *discoveryQueries `json:"queries"`
}

// A false predicate excludes a signal whose fields cannot satisfy the Home query.
// A null predicate selects all records in the time window.
type discoveryQueries struct {
	Traces  json.RawMessage `json:"traces"`
	Logs    json.RawMessage `json:"logs"`
	Metrics json.RawMessage `json:"metrics"`
}

func (h *JSONRPCHandler) discoverTelemetry(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []json.RawMessage
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params) != 1 || bytes.Equal(bytes.TrimSpace(params[0]), []byte("null")) {
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
	query := discovery.Query{StartTime: options.StartTime, EndTime: options.EndTime, ServiceName: options.ServiceName, Namespace: options.Namespace, ServiceIdentity: options.ServiceIdentity}
	var prefix string
	var args []any
	if options.Queries != nil {
		var err error
		prefix, args, err = discoverySearchSources(options)
		if err != nil {
			return nil, fmt.Errorf("%v: %w", err, jsonrpc2.ErrInvalidParams)
		}
		query.Sources = discovery.Sources{Spans: "home_spans", Logs: "home_logs", Metrics: "home_metrics"}
	}
	var statement, search, order string
	switch options.Kind {
	case "services":
		statement = discovery.Services(query)
		search = "concat(json_extract_string(entry, '$.serviceNamespace'), ' ', json_extract_string(entry, '$.serviceName'))"
		order = "json_extract_string(entry, '$.serviceNamespace'), json_extract_string(entry, '$.serviceName'), json_extract_string(entry, '$.serviceIdentity')"
	case "keys", "values":
		records, err := discovery.RecordsFrom(options.Signal, options.OwnerType, query.Sources)
		if err != nil {
			return nil, fmt.Errorf("%v: %w", err, jsonrpc2.ErrInvalidParams)
		}
		location := discovery.Location{Signal: options.Signal, OwnerType: options.OwnerType}
		if options.Kind == "keys" {
			statement = discovery.Keys(query, location, records)
			search = "json_extract_string(entry, '$.key')"
			order = "json_extract_string(entry, '$.key'), json_extract_string(entry, '$.kind')"
		} else {
			if options.Key == nil {
				return nil, jsonrpc2.ErrInvalidParams
			}
			statement = discovery.Values(query, location, records, *options.Key)
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
	if options.Queries != nil {
		filtered := "SELECT entry FROM discovered WHERE contains(lower(" + search + "), lower(" + discovery.Literal(options.Term) + "))"
		statement = prefix + ", discovered AS (" + statement + "), filtered AS (" + filtered + "), ordered AS (SELECT entry FROM filtered ORDER BY " + order + " OFFSET " + strconv.FormatUint(options.Offset, 10) + " LIMIT " + strconv.FormatUint(limit+1, 10) + "), page AS (SELECT entry FROM ordered LIMIT " + strconv.FormatUint(limit, 10) + ") SELECT json_object('rows', coalesce((SELECT list(json_array(entry)) FROM page), []), 'truncated', (SELECT count(*) FROM ordered) > " + strconv.FormatUint(limit, 10)
		if options.Kind == "services" {
			statement += ", 'totals', json_object('home', (SELECT count(*) FROM filtered), 'traces', (SELECT count(DISTINCT trace_id) FROM home_spans), 'logs', (SELECT count(*) FROM home_logs), 'metrics', (SELECT count(*) FROM home_metrics))"
		}
		statement += ")::VARCHAR"
		return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
			var result []byte
			err := db.QueryRowContext(ctx, statement, args...).Scan(&result)
			return result, err
		})
	}
	statement = "SELECT entry FROM (" + statement + ") discovered WHERE contains(lower(" + search + "), lower(" + discovery.Literal(options.Term) + ")) ORDER BY " + order + " OFFSET " + strconv.FormatUint(options.Offset, 10)
	queryParams, err := json.Marshal(map[string]any{"sql": statement, "limit": limit})
	if err != nil {
		return nil, err
	}
	normalized, err := normalizeParams("query", queryParams)
	if err != nil {
		return nil, err
	}
	return h.query(ctx, &jsonrpc2.Request{Params: normalized})
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
		ctes = append(ctes, signal.name+" AS ("+statement+")")
		args = append(args, params...)
	}
	return "WITH " + strings.Join(ctes, ", "), args, nil
}
