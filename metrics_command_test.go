package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/otelcol"
)

func TestMetricsCommandPreservesSummaryFieldsFiltersAndResolvesTimeOnce(t *testing.T) {
	const response = `[{"id":"018f0000-0000-7000-8000-000000000002","name":"http.server.duration","description":"request duration","unit":"ms","metricType":"Sum","aggregationTemporalityCode":2,"aggregationTemporality":"Cumulative","isMonotonic":true,"serviceName":"checkout","seriesCount":3,"seriesCardinality":7,"dataPointCount":9007199254740993,"lastValue":12.5,"lastSeen":"1790928000123456789"}]`
	requests := make(chan queryRPCRequest, 2)
	viewer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var rpcRequest queryRPCRequest
		require.NoError(t, json.NewDecoder(request.Body).Decode(&rpcRequest))
		requests <- rpcRequest
		_, err := writer.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":` + response + `}`))
		require.NoError(t, err)
	}))
	defer viewer.Close()

	fixedNow := time.Date(2026, 10, 2, 9, 0, 0, 111222333, time.UTC)
	clockCalls := 0
	cmd := newMetricsCommand(http.DefaultClient, func() time.Time {
		clockCalls++
		return fixedNow
	})
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"--endpoint", viewer.URL, "--service", "checkout", "--limit", "25"})
	require.NoError(t, cmd.Execute())
	assert.Equal(t, 1, clockCalls)
	for _, field := range metricSummaryFields {
		assert.Contains(t, output.String(), field)
	}
	assert.Contains(t, output.String(), "9007199254740993")

	request := <-requests
	assert.Equal(t, "searchMetricSummaries", request.Method)
	assert.Equal(t, float64(26), request.Params["limit"])
	assert.Equal(t, strconv.FormatInt(fixedNow.Add(-time.Hour).UnixNano(), 10), request.Params["startTime"])
	assert.Equal(t, strconv.FormatInt(fixedNow.UnixNano(), 10), request.Params["endTime"])
	condition := request.Params["query"].(map[string]any)["query"].(map[string]any)
	assert.Equal(t, "checkout", condition["value"])
	field := condition["field"].(map[string]any)
	assert.Equal(t, "service.name", field["name"])
	assert.Equal(t, "attribute", field["searchScope"])
	assert.Equal(t, "resource", field["attributeScope"])

	output.Reset()
	cmd = newMetricsCommand(http.DefaultClient, func() time.Time { return fixedNow })
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"--endpoint", viewer.URL, "--json"})
	require.NoError(t, cmd.Execute())
	assert.Equal(t, response+"\n", output.String())
	<-requests
}

func TestMetricsCommandLimitsEmptyErrorsCancellationAndHelp(t *testing.T) {
	summary := `{"id":"1","name":"requests","description":"","unit":"1","metricType":"Gauge","aggregationTemporalityCode":null,"aggregationTemporality":null,"isMonotonic":null,"serviceName":"svc","seriesCount":1,"seriesCardinality":1,"dataPointCount":1,"lastValue":1,"lastSeen":"1"}`
	t.Run("truncation and empty", func(t *testing.T) {
		responses := []string{"[" + summary + "," + summary + "]", `[]`}
		viewer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			response := responses[0]
			responses = responses[1:]
			_, _ = writer.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":` + response + `}`))
		}))
		defer viewer.Close()
		result, err := requestTelemetrySearch(context.Background(), viewer.Client(), viewer.URL, "searchMetricSummaries", telemetrySearchQuery{Limit: 1}, metricSummaryFields)
		require.NoError(t, err)
		assert.True(t, result.Truncated)
		assert.Len(t, result.Summaries, 1)
		result, err = requestTelemetrySearch(context.Background(), viewer.Client(), viewer.URL, "searchMetricSummaries", telemetrySearchQuery{Limit: 1}, metricSummaryFields)
		require.NoError(t, err)
		assert.Empty(t, result.Summaries)
	})

	t.Run("malformed arguments", func(t *testing.T) {
		cmd := newMetricsCommand(http.DefaultClient, time.Now)
		cmd.SetArgs([]string{"--start", "later"})
		require.ErrorContains(t, cmd.Execute(), "use RFC3339")
	})

	t.Run("unreachable endpoint", func(t *testing.T) {
		client := &http.Client{Transport: telemetryRoundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, context.DeadlineExceeded
		})}
		_, err := requestTelemetrySearch(context.Background(), client, "http://viewer.test", "searchMetricSummaries", telemetrySearchQuery{Limit: 25}, metricSummaryFields)
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("cancelled request", func(t *testing.T) {
		client := &http.Client{Transport: telemetryRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			<-request.Context().Done()
			return nil, request.Context().Err()
		})}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := requestTelemetrySearch(ctx, client, "http://viewer.test", "searchMetricSummaries", telemetrySearchQuery{Limit: 25}, metricSummaryFields)
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("offline help", func(t *testing.T) {
		root := newCommand(otelcol.CollectorSettings{BuildInfo: component.BuildInfo{Command: "otel-desktop-viewer"}})
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetErr(&output)
		root.SetArgs([]string{"metrics", "--help", "--endpoint", "http://127.0.0.1:1"})
		require.NoError(t, root.Execute())
		for _, text := range []string{"📈", "--service", "--since", "default 1h0m0s", "--start", "--end", "--limit", "default 25", "--endpoint", "--json", "checkout"} {
			assert.Contains(t, output.String(), text)
		}
	})
}
