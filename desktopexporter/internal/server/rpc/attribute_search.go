package rpc

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/attributes"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/searchindex"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"golang.org/x/exp/jsonrpc2"
)

// searchAttributeMatches finds keys/owners first and values only after selection.
func (h *Handler) searchAttributeMatches(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	options, err := parseAttributeMatchParams(req.Params)
	if err != nil {
		return nil, err
	}
	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		return attributes.SearchScoped(ctx, db, *options)
	})
}

func parseAttributeMatchParams(raw json.RawMessage) (*searchindex.DiscoveryOptions, error) {
	params, err := decodePositionalParams(raw, 1, 5)
	if err != nil {
		return nil, err
	}
	options := &searchindex.DiscoveryOptions{}
	term, ok := params[0].(string)
	if !ok {
		return nil, jsonrpc2.ErrInvalidParams
	}
	options.Term = term
	for i, target := range map[int]*string{1: &options.Signal, 2: &options.Owner, 4: &options.Kind} {
		if i < len(params) && params[i] != nil {
			v, ok := params[i].(string)
			if !ok {
				return nil, jsonrpc2.ErrInvalidParams
			}
			*target = v
		}
	}
	if len(params) > 3 && params[3] != nil {
		path, ok := params[3].([]any)
		if !ok {
			return nil, jsonrpc2.ErrInvalidParams
		}
		options.Path = path
	}
	if _, err := searchindex.NormalizeSignal(options.Signal); err != nil {
		return nil, jsonrpc2.ErrInvalidParams
	}
	if options.Owner != "" {
		switch options.Owner {
		case "span", "event", "link", "log", "metric", "datapoint", "exemplar", "resource", "scope":
		default:
			return nil, jsonrpc2.ErrInvalidParams
		}
	}
	if options.Kind != "" {
		switch options.Kind {
		case "string", "int64", "double", "bool", "bytes", "empty", "map", "array":
		default:
			return nil, jsonrpc2.ErrInvalidParams
		}
	}
	if len(options.Path) > 0 {
		if options.Owner == "" {
			return nil, jsonrpc2.ErrInvalidParams
		}
		root, ok := options.Path[0].(string)
		if !ok || (root != "attributes" && root != "body" && root != "metadata") {
			return nil, jsonrpc2.ErrInvalidParams
		}
		for _, segment := range options.Path {
			switch v := segment.(type) {
			case string:
			case json.Number:
				index, err := v.Int64()
				if err != nil || index < 0 || index >= math.MaxUint32 {
					return nil, jsonrpc2.ErrInvalidParams
				}
			default:
				return nil, jsonrpc2.ErrInvalidParams
			}
		}
	}
	return options, nil
}

func (h *Handler) getFieldValueCompletions(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	params, err := parseFieldValuesParams(req.Params)
	if err != nil {
		return nil, err
	}

	// A map rather than a switch: the named-params coverage test reads
	// Handle's dispatch cases out of this file by pattern, and a case-shaped
	// signal dispatch would show up in it as three phantom methods.
	bySignal := map[string]func(context.Context, *sql.DB, string, string, int64) (json.RawMessage, error){
		"traces":  spans.GetFieldValueCompletions,
		"logs":    logs.GetFieldValueCompletions,
		"metrics": metrics.GetFieldValueCompletions,
	}
	get, ok := bySignal[params.signal]
	if !ok {
		return nil, fmt.Errorf("%w: unknown signal %q", jsonrpc2.ErrInvalidParams, params.signal)
	}

	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		return get(ctx, db, params.field, params.term, params.limit)
	})
}

// getFieldValueCompletions returns distinct values of one completable column, for
// value completion in the search box. Which fields complete is decided by
// each signal package's allowlist, so an unknown field is an invalid-params
// answer rather than an empty list -- a typo should look like one. The limit
// is clamped rather than trusted: the caller wants a dropdown, not a dump.
type fieldValuesParams struct {
	signal string
	field  string
	term   string
	limit  int64
}

func parseFieldValuesParams(raw json.RawMessage) (fieldValuesParams, error) {
	params, err := decodePositionalParams(raw, 4, 4)
	if err != nil {
		return fieldValuesParams{}, err
	}
	values := make([]string, 3)
	for index := range values {
		value, ok := params[index].(string)
		if !ok {
			return fieldValuesParams{}, jsonrpc2.ErrInvalidParams
		}
		values[index] = value
	}
	// Same decoder targetBuckets uses: accepts a JSON number or a string.
	limit, err := parseTimestampParam(params[3], "limit")
	if err != nil {
		return fieldValuesParams{}, err
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 500 {
		limit = 500
	}
	return fieldValuesParams{signal: values[0], field: values[1], term: values[2], limit: limit}, nil
}
