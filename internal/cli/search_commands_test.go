package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func TestResolveTelemetrySearchWindowsAndErrors(t *testing.T) {
	now := time.Date(2026, 10, 2, 9, 30, 0, 123456789, time.FixedZone("offset", -7*60*60))
	query, err := resolveTelemetrySearch(telemetrySearchOptions{Since: time.Hour, Limit: 25}, false, now)
	require.NoError(t, err)
	assert.Equal(t, strconv.FormatInt(now.Add(-time.Hour).UnixNano(), 10), *query.StartTime)
	assert.Equal(t, strconv.FormatInt(now.UnixNano(), 10), *query.EndTime)

	query, err = resolveTelemetrySearch(telemetrySearchOptions{
		Since: time.Hour, Start: "2026-10-02T08:00:00.000000001Z",
		End: "2026-10-02T09:00:00.999999999+01:00", Limit: 7,
	}, false, now)
	require.NoError(t, err)
	assert.Equal(t, "1790928000000000001", *query.StartTime)
	assert.Equal(t, "1790928000999999999", *query.EndTime)

	for _, tc := range []struct {
		name         string
		options      telemetrySearchOptions
		sinceChanged bool
		message      string
	}{
		{"since conflict", telemetrySearchOptions{Since: time.Hour, Start: "2026-10-02T08:00:00Z", Limit: 25}, true, "cannot be combined"},
		{"zero since", telemetrySearchOptions{Limit: 25}, false, "greater than zero"},
		{"zero limit", telemetrySearchOptions{Since: time.Hour}, false, "greater than zero"},
		{"bad start", telemetrySearchOptions{Since: time.Hour, Start: "yesterday", Limit: 25}, false, "use RFC3339"},
		{"reverse range", telemetrySearchOptions{Since: time.Hour, Start: "2026-10-02T09:00:00Z", End: "2026-10-02T08:00:00Z", Limit: 25}, false, "must not be after"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolveTelemetrySearch(tc.options, tc.sinceChanged, now)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.message)
		})
	}
}

func TestSearchCommandsSummaryContract(t *testing.T) {
	const traceSummary = `{"traceID":"0123456789abcdef0123456789abcdef","hasRootSpan":true,"rootSpan":{"serviceName":"checkout","name":"POST /checkout"},"startTime":"1790928000123456789","durationNs":"9007199254740993","spanCount":4,"errorCount":1}`
	const matchedSpans = `[{"traceID":"0123456789abcdef0123456789abcdef","spanID":"00000000000000a1"},{"traceID":"0123456789abcdef0123456789abcdef","spanID":"00000000000000a2"}]`
	const traceTableOutput = "traceID                           hasRootSpan  rootSpan                                            startTime            durationNs        spanCount  errorCount  matchedSpans (row traceID omitted)                           \n" +
		"--------------------------------  -----------  --------------------------------------------------  -------------------  ----------------  ---------  ----------  -------------------------------------------------------------\n" +
		"0123456789abcdef0123456789abcdef  true         {\"name\":\"POST /checkout\",\"serviceName\":\"checkout\"}  1790928000123456789  9007199254740993  4          1           [{\"spanID\":\"00000000000000a1\"},{\"spanID\":\"00000000000000a2\"}]\n"
	const logSummary = `{"logRef":"018f0000-0000-7000-8000-000000000001","timestamp":"1790928000123456789","severityText":"ERROR","severityNumber":17,"serviceName":"checkout","bodyPreview":"card declined"}`
	const metricSummary = `{"metricRef":"018f0000-0000-7000-8000-000000000002","name":"http.server.duration","unit":"ms","metricType":"Sum","aggregationTemporalityCode":null,"aggregationTemporality":null,"isMonotonic":true,"serviceName":"checkout","seriesCount":3,"seriesCardinality":7,"dataPointCount":9007199254740993,"lastValue":12.500,"lastSeen":"1790928000123456789"}`

	cases := []struct {
		command          string
		method           string
		fields           []string
		fixedNow         time.Time
		serviceField     map[string]any
		serverResult     func(filtered bool) string
		wantJSONFiltered string
		wantJSONAll      string
		wantExactTable   string
		wantTableSubstrs []string
		forbidSubstrs    []string
		twoRows          string
		twoRowsFiltered  string
		wantTrimmedJSON  string
		badArgs          []string
		badErr           string
		helpSubstrs      []string
	}{
		{
			command:      "traces",
			method:       "searchTraceSummaries",
			fields:       traceSummaryFields,
			fixedNow:     time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC),
			serviceField: map[string]any{"name": "serviceName", "searchScope": "field", "type": "string"},
			serverResult: func(filtered bool) string {
				const serviceBreakdown = `,"serviceBreakdown":[{"key":"service:checkout","label":"checkout","count":4,"other":false}]`
				body := `[` + traceSummary[:len(traceSummary)-1] + serviceBreakdown
				if filtered {
					body += `,"matchedSpans":` + matchedSpans
				}
				return body + `}]`
			},
			wantJSONFiltered: `[` + traceSummary[:len(traceSummary)-1] + `,"matchedSpans":` + matchedSpans + `}]`,
			wantJSONAll:      `[` + traceSummary + `]`,
			wantExactTable:   traceTableOutput,
			twoRows: `[{"traceID":"1","hasRootSpan":false,"rootSpan":null,"startTime":"1","durationNs":null,"spanCount":1,"errorCount":0},` +
				`{"traceID":"2","hasRootSpan":false,"rootSpan":null,"startTime":"2","durationNs":null,"spanCount":1,"errorCount":0}]`,
			twoRowsFiltered: `[{"traceID":"1","hasRootSpan":false,"rootSpan":null,"startTime":"1","durationNs":null,"spanCount":1,"errorCount":0,"matchedSpans":[{"traceID":"1","spanID":"0000000000000001"}]},` +
				`{"traceID":"2","hasRootSpan":false,"rootSpan":null,"startTime":"2","durationNs":null,"spanCount":1,"errorCount":0,"matchedSpans":[{"traceID":"2","spanID":"0000000000000002"}]}]`,
			wantTrimmedJSON: `[{"traceID":"1","hasRootSpan":false,"rootSpan":null,"startTime":"1","durationNs":null,"spanCount":1,"errorCount":0,"matchedSpans":[{"traceID":"1","spanID":"0000000000000001"}]}]`,
			badArgs:         []string{"--limit", "0"},
			badErr:          "--limit must be greater than zero",
			helpSubstrs:     []string{"🧵", "--service", "--since", "default 1h0m0s", "--start", "--end", "--limit", "default 25", "--endpoint", "--json", "checkout"},
		},
		{
			command:          "logs",
			method:           "searchLogSummaries",
			fields:           logSummaryFields,
			fixedNow:         time.Date(2026, 10, 2, 9, 0, 0, 987654321, time.UTC),
			serviceField:     map[string]any{"name": "serviceName", "searchScope": "field", "type": "string"},
			serverResult:     func(bool) string { return `[` + logSummary + `]` },
			wantJSONFiltered: `[` + logSummary + `]`,
			wantJSONAll:      `[` + logSummary + `]`,
			wantTableSubstrs: append(append([]string(nil), logSummaryFields...), "1790928000123456789"),
			twoRows: `[{"logRef":"1","timestamp":"1","severityText":"INFO","severityNumber":9,"serviceName":"svc","bodyPreview":"one"},` +
				`{"logRef":"2","timestamp":"2","severityText":"INFO","severityNumber":9,"serviceName":"svc","bodyPreview":"two"}]`,
			badArgs:     []string{"--since", "0s"},
			badErr:      "--since must be greater than zero",
			helpSubstrs: []string{"🪵", "--service", "--since", "default 1h0m0s", "--start", "--end", "--limit", "default 25", "--endpoint", "--json", "checkout"},
		},
		{
			command:          "metrics",
			method:           "searchMetricSummaries",
			fields:           metricSummaryFields,
			fixedNow:         time.Date(2026, 10, 2, 9, 0, 0, 111222333, time.UTC),
			serviceField:     map[string]any{"name": "service.name", "searchScope": "attribute", "attributeScope": "resource", "type": "string"},
			serverResult:     func(bool) string { return `[` + metricSummary + `]` },
			wantJSONFiltered: `[` + metricSummary + `]`,
			wantJSONAll:      `[` + metricSummary + `]`,
			wantTableSubstrs: append(append([]string(nil), metricSummaryFields...), "018f0000-0000-7000-8000-000000000002", "9007199254740993"),
			forbidSubstrs:    []string{"id"},
			twoRows: `[{"metricRef":"1","name":"requests","unit":"1","metricType":"Gauge","aggregationTemporalityCode":null,"aggregationTemporality":null,"isMonotonic":null,"serviceName":"svc","seriesCount":1,"seriesCardinality":1,"dataPointCount":1,"lastValue":1,"lastSeen":"1"},` +
				`{"metricRef":"2","name":"requests","unit":"1","metricType":"Gauge","aggregationTemporalityCode":null,"aggregationTemporality":null,"isMonotonic":null,"serviceName":"svc","seriesCount":1,"seriesCardinality":1,"dataPointCount":1,"lastValue":2,"lastSeen":"2"}]`,
			badArgs:     []string{"--start", "later"},
			badErr:      "use RFC3339",
			helpSubstrs: []string{"📈", "--service", "--since", "default 1h0m0s", "--start", "--end", "--limit", "default 25", "--endpoint", "--json", "checkout", "metricRef"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			t.Run("summary fields, service filter, and single clock read", func(t *testing.T) {
				requests := make(chan queryRPCRequest, 3)
				viewer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var req queryRPCRequest
					require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
					requests <- req
					_, err := w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":` + tc.serverResult(req.Params["query"] != nil) + `}`))
					require.NoError(t, err)
				}))
				defer viewer.Close()

				clockCalls := 0
				out, _, err := execCLI(t, cliOptions{
					client: viewer.Client(),
					now: func() time.Time {
						clockCalls++
						return tc.fixedNow
					},
					mergeErr: true,
				}, tc.command, "--endpoint", viewer.URL, "--service", "checkout", "--limit", "25")
				require.NoError(t, err)
				assert.Equal(t, 1, clockCalls)
				if tc.wantExactTable != "" {
					assert.Equal(t, tc.wantExactTable, out)
				}
				for _, text := range tc.wantTableSubstrs {
					assert.Contains(t, out, text)
				}
				for _, forbidden := range tc.forbidSubstrs {
					assert.NotContains(t, out, forbidden)
				}

				req := <-requests
				assert.Equal(t, tc.method, req.Method)
				assert.Equal(t, float64(26), req.Params["limit"])
				assert.Equal(t, strconv.FormatInt(tc.fixedNow.Add(-time.Hour).UnixNano(), 10), req.Params["startTime"])
				assert.Equal(t, strconv.FormatInt(tc.fixedNow.UnixNano(), 10), req.Params["endTime"])
				condition := req.Params["query"].(map[string]any)["query"].(map[string]any)
				assert.Equal(t, "checkout", condition["value"])
				assert.Equal(t, tc.serviceField, condition["field"])

				out, _, err = execCLI(t, cliOptions{client: viewer.Client(), now: fixedClock(tc.fixedNow), mergeErr: true},
					tc.command, "--endpoint", viewer.URL, "--service", "checkout", "--json")
				require.NoError(t, err)
				assert.Equal(t, tc.wantJSONFiltered+"\n", out)
				<-requests

				out, _, err = execCLI(t, cliOptions{client: viewer.Client(), now: fixedClock(tc.fixedNow), mergeErr: true},
					tc.command, "--endpoint", viewer.URL, "--json")
				require.NoError(t, err)
				assert.Equal(t, tc.wantJSONAll+"\n", out)
				for _, forbidden := range tc.forbidSubstrs {
					assert.NotContains(t, out, `"`+forbidden+`"`)
				}
				<-requests
			})

			t.Run("truncation and empty", func(t *testing.T) {
				two := staticRPCServer(t, `{"jsonrpc":"2.0","id":1,"result":`+tc.twoRows+`}`)
				result, err := requestTelemetrySearch(context.Background(), two.Client(), two.URL, tc.method, telemetrySearchQuery{Limit: 1}, tc.fields)
				require.NoError(t, err)
				assert.True(t, result.Truncated)
				assert.Len(t, result.Summaries, 1)

				out, _, err := execCLI(t, cliOptions{client: two.Client()}, tc.command, "--endpoint", two.URL, "--limit", "1")
				require.NoError(t, err)
				assert.Contains(t, out, "[1 rows shown; more rows available; use --limit to return more]")
				if tc.command == "traces" {
					assert.NotContains(t, out, "  2 ")
				}

				if tc.twoRowsFiltered != "" {
					filtered := staticRPCServer(t, `{"jsonrpc":"2.0","id":1,"result":`+tc.twoRowsFiltered+`}`)
					out, _, err = execCLI(t, cliOptions{client: filtered.Client()}, tc.command, "--endpoint", filtered.URL, "--service", "checkout", "--limit", "1", "--json")
					require.NoError(t, err)
					assert.Equal(t, tc.wantTrimmedJSON+"\n", out)
				}

				empty := staticRPCServer(t, `{"jsonrpc":"2.0","id":1,"result":[]}`)
				result, err = requestTelemetrySearch(context.Background(), empty.Client(), empty.URL, tc.method, telemetrySearchQuery{Limit: 25}, tc.fields)
				require.NoError(t, err)
				assert.Empty(t, result.Rows)
				assert.NotNil(t, result.Summaries)
				assert.Empty(t, result.Summaries)
			})

			t.Run("malformed arguments offline", func(t *testing.T) {
				args := append([]string{tc.command}, tc.badArgs...)
				_, _, err := execCLI(t, cliOptions{client: offlineHTTPClient(t)}, args...)
				require.ErrorContains(t, err, tc.badErr)
			})

			t.Run("offline help", func(t *testing.T) {
				out, _, err := execCLI(t, cliOptions{client: offlineHTTPClient(t), mergeErr: true},
					tc.command, "--help", "--endpoint", "http://127.0.0.1:1")
				require.NoError(t, err)
				for _, text := range tc.helpSubstrs {
					assert.Contains(t, out, text)
				}
			})
		})
	}
}

func TestMatchedSpansForTableKeepsForeignIDsAndRepeatedMatches(t *testing.T) {
	own := map[string]any{"traceID": "parent", "spanID": "a", "count": json.Number("9007199254740993")}
	foreign := map[string]any{"traceID": "other", "spanID": "a"}
	unknown := map[string]any{"traceID": nil, "spanID": "b"}
	matches := []any{own, foreign, unknown, own}
	display := map[string]any{"spanID": "a", "count": json.Number("9007199254740993")}
	assert.Equal(t, []any{display, foreign, unknown, display}, matchedSpansForTable("parent", matches))
	assert.Equal(t, "parent", own["traceID"])
	assert.Equal(t, []any{}, matchedSpansForTable("parent", []any{}))
	assert.Nil(t, matchedSpansForTable("parent", nil))
}

func TestMetricsCommandThroughProductionServer(t *testing.T) {
	t.Parallel()
	endpoint, _, _, metricsExporter := startAttributeIntegration(t)
	data := pmetric.NewMetrics()
	resource := data.ResourceMetrics().AppendEmpty()
	resource.Resource().Attributes().PutStr("service.name", "checkout")
	metrics := resource.ScopeMetrics().AppendEmpty().Metrics()
	for i, description := range []string{"received request duration", ""} {
		metric := metrics.AppendEmpty()
		metric.SetName("duration" + strconv.Itoa(i))
		metric.SetDescription(description)
		metric.SetUnit("ms")
		point := metric.SetEmptyGauge().DataPoints().AppendEmpty()
		point.SetTimestamp(1)
		point.SetIntValue(42)
	}
	require.NoError(t, metricsExporter.ConsumeMetrics(t.Context(), data))

	args := []string{"metrics", "--endpoint", endpoint, "--start", "1970-01-01T00:00:00Z"}
	table, err := runCLI(t, args...)
	require.NoError(t, err)
	assert.NotContains(t, table, "description")

	output, err := runCLI(t, append(args, "--json")...)
	require.NoError(t, err)
	var summaries []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(output), &summaries))
	require.Len(t, summaries, 2)
	var descriptions []string
	for _, summary := range summaries {
		for _, field := range metricSummaryFields {
			require.Contains(t, summary, field)
		}
		assert.NotContains(t, summary, "description")
		assert.JSONEq(t, `"checkout"`, string(summary["serviceName"]))
		assert.JSONEq(t, `42`, string(summary["lastValue"]))
		var ref string
		require.NoError(t, json.Unmarshal(summary["metricRef"], &ref))
		detail, err := runCLI(t, "metric", ref, "--endpoint", endpoint, "--json")
		require.NoError(t, err)
		var descriptor map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(detail), &descriptor))
		require.Contains(t, descriptor, "description")
		var description string
		require.NoError(t, json.Unmarshal(descriptor["description"], &description))
		descriptions = append(descriptions, description)
	}
	assert.ElementsMatch(t, []string{"received request duration", ""}, descriptions)
}
