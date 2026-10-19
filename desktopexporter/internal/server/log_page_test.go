package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/exp/jsonrpc2"
)

func TestSearchLogPageParams(t *testing.T) {
	handler := setupHandlerWithData(t)
	for _, params := range []any{
		[]any{nil, nil},
		map[string]any{"startTime": nil, "endTime": nil, "pageSize": 1, "offset": 0},
		map[string]any{"startTime": nil, "endTime": nil, "columns": []any{map[string]any{"source": "log", "kind": "field", "key": "flags"}}},
	} {
		result, err := handler.Handle(context.Background(), createRequest("searchLogPage", params))
		require.NoError(t, err)
		var page struct {
			Logs  []json.RawMessage `json:"logs"`
			Total int               `json:"total"`
		}
		require.NoError(t, json.Unmarshal(result.(json.RawMessage), &page))
		require.Equal(t, 1, page.Total)
		require.Len(t, page.Logs, 1)
	}
	for _, params := range []any{
		[]any{nil},
		[]any{nil, nil, nil, nil, nil, 0},
		[]any{nil, nil, nil, nil, nil, 251},
		[]any{nil, nil, nil, nil, nil, 1.5},
		[]any{nil, nil, nil, nil, nil, 100, -1},
		[]any{nil, nil, nil, nil, nil, 100, "9223372036854775808"},
		map[string]any{"startTime": nil, "endTime": nil, "page": 2},
		[]any{nil, nil, nil, nil, nil, 100, 0, "flags"},
		[]any{nil, nil, nil, nil, nil, 100, 0, []any{nil}},
		[]any{nil, nil, nil, nil, nil, 100, 0, []any{map[string]any{"source": "log", "kind": "field", "key": "id"}}},
		[]any{nil, nil, nil, nil, nil, 100, 0, []any{map[string]any{"source": "span", "kind": "attribute", "key": "flags"}}},
		[]any{nil, nil, nil, nil, nil, 100, 0, []any{map[string]any{"source": "log", "kind": "attribute", "key": 1}}},
	} {
		_, err := handler.Handle(context.Background(), createRequest("searchLogPage", params))
		require.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)
	}
}
