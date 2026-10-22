package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/CtrlSpice/otel-desktop-viewer/internal/discovery"
	"golang.org/x/exp/jsonrpc2"
)

type discoveryOptions struct {
	Kind        string  `json:"kind"`
	Signal      string  `json:"signal"`
	OwnerType   string  `json:"ownerType"`
	ServiceName *string `json:"serviceName"`
	Namespace   *string `json:"namespace"`
	Key         *string `json:"key"`
	StartTime   *string `json:"startTime"`
	EndTime     *string `json:"endTime"`
	Limit       *uint64 `json:"limit"`
	Offset      uint64  `json:"offset"`
	Term        string  `json:"term"`
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
	query := discovery.Query{StartTime: options.StartTime, EndTime: options.EndTime, ServiceName: options.ServiceName, Namespace: options.Namespace}
	var statement, search, order string
	switch options.Kind {
	case "services":
		statement = discovery.Services(query)
		search = "concat(json_extract_string(entry, '$.serviceNamespace'), ' ', json_extract_string(entry, '$.serviceName'))"
		order = "json_extract_string(entry, '$.serviceNamespace'), json_extract_string(entry, '$.serviceName')"
	case "keys", "values":
		records, err := discovery.Records(options.Signal, options.OwnerType)
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
		}
	default:
		return nil, jsonrpc2.ErrInvalidParams
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
