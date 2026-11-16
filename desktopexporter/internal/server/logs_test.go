package server

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/jsonrpc2"
)

func TestSearchLogSummaries(t *testing.T) {
	t.Run("Empty", func(t *testing.T) {
		handler := setupHandler(t)

		req := createRequest("searchLogSummaries", []string{"0", strconv.FormatInt(1<<63-1, 10)})
		result, err := handler.Handle(context.Background(), req)

		assert.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		assert.True(t, ok, "Expected json.RawMessage, got %T", result)
		var entries []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &entries))
		assert.Len(t, entries, 0)
	})

	t.Run("With Data", func(t *testing.T) {
		handler := setupHandlerWithData(t)

		req := createRequest("searchLogSummaries", []string{"0", strconv.FormatInt(1<<63-1, 10)})
		result, err := handler.Handle(context.Background(), req)

		assert.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		assert.True(t, ok, "Expected json.RawMessage, got %T", result)
		var entries []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &entries))
		require.Len(t, entries, 1, "searchLogSummaries should return the ingested log")
		// The summary carries bodyPreview; getLog carries the complete body.
		assert.Equal(t, "test log message", entries[0]["bodyPreview"])

		logRef, ok := entries[0]["logRef"].(string)
		require.True(t, ok, "summary should carry an id for detail fetch")
		getReq := createRequest("getLog", []string{logRef})
		getResult, getErr := handler.Handle(context.Background(), getReq)
		assert.NoError(t, getErr)
		getRaw, ok := getResult.(json.RawMessage)
		assert.True(t, ok)
		var full map[string]any
		assert.NoError(t, json.Unmarshal(getRaw, &full))
		assert.Equal(t, map[string]any{"kind": "string", "value": "test log message"}, full["body"])
	})

	t.Run("Garbage spanID in query tree", func(t *testing.T) {
		handler := setupHandlerWithData(t)

		query := map[string]any{
			"id":   "q-garbage",
			"type": "condition",
			"query": map[string]any{
				"field":         map[string]any{"name": "spanID", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "zz-definitely-not-hex",
			},
		}
		req := createRequest("searchLogSummaries", []any{"0", strconv.FormatInt(1<<63-1, 10), query})
		result, err := handler.Handle(context.Background(), req)

		assert.NoError(t, err, "garbage span ID in search must not surface -32603")
		raw, ok := result.(json.RawMessage)
		require.True(t, ok)
		var entries []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &entries))
		assert.Empty(t, entries)
	})

	t.Run("Limit", func(t *testing.T) {
		handler := setupHandlerWithData(t)
		maxTime := strconv.FormatInt(1<<63-1, 10)

		result, err := handler.Handle(context.Background(), createRequest("searchLogSummaries", []any{"0", maxTime, nil, 1}))
		require.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		require.True(t, ok)
		var entries []map[string]any
		require.NoError(t, json.Unmarshal(raw, &entries))
		require.Len(t, entries, 1)

		result, err = handler.Handle(context.Background(), createRequest("searchLogSummaries", []any{"0", maxTime, nil, 0}))
		assert.Nil(t, result)
		assert.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)
	})
}

func TestGetTraceLogSummaries(t *testing.T) {
	t.Run("Valid", func(t *testing.T) {
		handler := setupHandlerWithData(t)

		result, err := handler.Handle(context.Background(), createRequest("getTraceLogSummaries", map[string]any{
			"traceID": testTraceIDHex,
		}))
		require.NoError(t, err)
		var entries []map[string]any
		require.NoError(t, json.Unmarshal(result.(json.RawMessage), &entries))
		require.Len(t, entries, 1)
		require.Equal(t, "0000000000000001", entries[0]["spanID"])
		require.Equal(t, "request.failed", entries[0]["eventName"])
		require.Equal(t, "test log message", entries[0]["bodyPreview"])
		require.NotContains(t, entries[0], "body")
		require.NotContains(t, entries[0], "attributes")
	})

	t.Run("Empty", func(t *testing.T) {
		handler := setupHandler(t)

		result, err := handler.Handle(context.Background(), createRequest("getTraceLogSummaries", []string{
			"00000000000000000000000000000002",
		}))
		require.NoError(t, err)
		require.JSONEq(t, `[]`, string(result.(json.RawMessage)))
	})

	t.Run("Malformed trace ID", func(t *testing.T) {
		handler := setupHandler(t)

		result, err := handler.Handle(context.Background(), createRequest("getTraceLogSummaries", []string{"not-a-trace-id"}))
		require.Nil(t, result)
		require.Equal(t, ErrInvalidTraceID, err)
	})

	t.Run("Empty trace ID", func(t *testing.T) {
		handler := setupHandler(t)

		result, err := handler.Handle(context.Background(), createRequest("getTraceLogSummaries", []string{""}))
		require.Nil(t, result)
		require.Equal(t, ErrInvalidTraceID, err)
	})

	t.Run("Missing trace ID", func(t *testing.T) {
		handler := setupHandler(t)

		result, err := handler.Handle(context.Background(), createRequest("getTraceLogSummaries", []string{}))
		require.Nil(t, result)
		require.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)
	})
}

// TestSearchLogSummariesInvalidParams ensures searchLogSummaries with wrong param count returns ErrInvalidParams.
func TestSearchLogSummariesInvalidParams(t *testing.T) {
	handler := setupHandler(t)

	req := createRequest("searchLogSummaries", []string{"0"}) // only one param
	result, err := handler.Handle(context.Background(), req)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, jsonrpc2.ErrInvalidParams, err)
}

func TestClearLogs(t *testing.T) {
	handler := setupHandlerWithData(t)
	ctx := context.Background()

	result, err := handler.Handle(ctx, createRequest("clearLogs", nil))
	assert.NoError(t, err)
	assert.Equal(t, "Logs cleared successfully", result)

	searchResult, err := handler.Handle(ctx, createRequest("searchLogSummaries", []string{"0", strconv.FormatInt(1<<63-1, 10)}))
	assert.NoError(t, err)
	raw, ok := searchResult.(json.RawMessage)
	require.True(t, ok)
	var entries []map[string]any
	assert.NoError(t, json.Unmarshal(raw, &entries))
	assert.Len(t, entries, 0)
}

func TestGetLogNotFound(t *testing.T) {
	handler := setupHandlerWithData(t)

	req := createRequest("getLog", []string{"00000000-0000-0000-0000-0000000000aa"})
	result, err := handler.Handle(context.Background(), req)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, ErrLogsNotFound, err)
}

func TestDeleteLogsByRefs(t *testing.T) {
	handler := setupHandlerWithData(t)
	ctx := context.Background()

	searchResult, err := handler.Handle(ctx, createRequest("searchLogSummaries", []string{"0", strconv.FormatInt(1<<63-1, 10)}))
	require.NoError(t, err)
	raw, ok := searchResult.(json.RawMessage)
	require.True(t, ok)
	var entries []map[string]any
	require.NoError(t, json.Unmarshal(raw, &entries))
	require.Len(t, entries, 1)
	logRef, ok := entries[0]["logRef"].(string)
	require.True(t, ok)

	result, err := handler.Handle(ctx, createRequest("deleteLogsByRefs", []string{logRef}))
	assert.NoError(t, err)
	response, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 1, response["count"])

	getResult, err := handler.Handle(ctx, createRequest("getLog", []string{logRef}))
	assert.Nil(t, getResult)
	assert.Equal(t, ErrLogsNotFound, err, "deleted log should be gone")
}

func TestGetLogAttributeDefinitions(t *testing.T) {
	t.Run("With Data", func(t *testing.T) {
		handler := setupHandlerWithData(t)

		result, err := handler.Handle(context.Background(), createRequest("getLogAttributeDefinitions", []any{}))
		assert.NoError(t, err)
		assertAttributeDiscovery(t, result)
	})

	t.Run("Invalid Parameters", func(t *testing.T) {
		handler := setupHandler(t)

		result, err := handler.Handle(context.Background(), createRequest("getLogAttributeDefinitions", []string{"123"}))
		assert.Nil(t, result)
		assert.Equal(t, jsonrpc2.ErrInvalidParams, err)
	})
}
