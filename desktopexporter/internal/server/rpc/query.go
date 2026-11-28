package rpc

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	storequery "github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/query"
	"golang.org/x/exp/jsonrpc2"
)

func (h *Handler) query(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	params, err := parseQueryParams(req.Params)
	if err != nil {
		return nil, err
	}

	result, err := storeRead(h.store, func(db *sql.DB) (json.RawMessage, error) {
		conn, err := db.Conn(ctx)
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		return storequery.Execute(ctx, conn, params.statement, params.limit)
	})
	if err == nil {
		return result, nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, h.handleStoreError(ctx, err)
	}
	if errors.Is(err, storequery.ErrReadOnly) {
		return nil, fmt.Errorf("query rejected: %v: %w", err, jsonrpc2.ErrInvalidParams)
	}
	return nil, fmt.Errorf("query failed: %v: %w", err, ErrInvalidQuery)
}

type queryParams struct {
	statement string
	limit     uint64
}

func parseQueryParams(raw json.RawMessage) (queryParams, error) {
	params, err := decodePositionalParams(raw, 1, 2)
	if err != nil {
		return queryParams{}, err
	}
	statement, ok := params[0].(string)
	if !ok {
		return queryParams{}, fmt.Errorf("sql must be a string: %w", jsonrpc2.ErrInvalidParams)
	}
	limit := storequery.DefaultLimit
	if len(params) == 2 {
		number, ok := params[1].(json.Number)
		if !ok {
			return queryParams{}, fmt.Errorf("limit must be a non-negative whole number: %w", jsonrpc2.ErrInvalidParams)
		}
		parsed, err := strconv.ParseUint(number.String(), 10, 64)
		if err != nil {
			return queryParams{}, fmt.Errorf("limit must be a non-negative whole number: %w", jsonrpc2.ErrInvalidParams)
		}
		limit = parsed
	}
	return queryParams{statement: statement, limit: limit}, nil
}
