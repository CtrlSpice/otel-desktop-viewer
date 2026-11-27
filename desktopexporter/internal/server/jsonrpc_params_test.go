package server

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/jsonrpc2"
)

// TestNamedParamsMatchPositional is the property that matters: for every
// method, a named request and the equivalent positional request must produce
// the same argument array. Anything else means the name table has drifted
// from the handler that reads the positions.
func TestNamedParamsMatchPositional(t *testing.T) {
	cases := []struct {
		method     string
		named      string
		positional string
	}{
		{"searchTraceSummaries",
			`{"startTime":"1","endTime":"2"}`, `["1","2"]`},
		{"getTraceView",
			`{"traceID":"abc"}`, `["abc"]`},
		{"getLog",
			`{"logRef":"L1"}`, `["L1"]`},
		{"getSpan",
			`{"limit":25,"traceID":"T1","spanID":"S1"}`, `["S1","T1",25]`},
		{"searchAttributeMatches",
			`{"term":"http"}`, `["http"]`},
		{"query",
			`{"limit":5,"sql":"select 1"}`, `["select 1",5]`},
		{"getMetric",
			`{"metricRef":"m"}`, `["m"]`},
		{"getMetricSeries",
			`{"endTime":null,"seriesRef":"s","metricRef":"m","startTime":"1"}`,
			`["m","s","1",null]`},
		// Order in the object must not matter.
		{"searchLogSummaries",
			`{"endTime":"2","startTime":"1"}`, `["1","2"]`},
		{"searchLogPage",
			`{"offset":100,"pageSize":50,"endTime":"2","startTime":"1"}`, `["1","2",null,null,null,50,100]`},
		// The wide one, fully populated, in a deliberately shuffled order.
		{"getMetricView",
			`{"tzName":"UTC","metricRef":"s","startTime":"1","endTime":"2",
			  "targetBuckets":10,"seriesRefs":["a"],"quantiles":[0.5],
			  "tzOffsetNs":0,"viewBuckets":5,
			  "sparklineBuckets":6,"selectedSeriesRefs":["b"],
			  "datapointSeriesRefs":["c"],"datapointSeriesLimit":7}`,
			`["s","1","2",10,["a"],[0.5],0,5,6,["b"],"UTC",["c"],7]`},
		{"getMetricAggregateView",
			`{"tzName":"UTC","metricRef":"s","startTime":"1","endTime":"2",
			  "targetBuckets":10,"seriesRefs":["a"],"quantiles":[0.5],
			  "tzOffsetNs":0,"viewBuckets":5,"selectedSeriesRefs":["b"]}`,
			`["s","1","2",10,["a"],[0.5],0,5,["b"],"UTC"]`},
	}

	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			got, err := normalizeParams(tc.method, json.RawMessage(tc.named))
			require.NoError(t, err)

			var a, b any
			require.NoError(t, json.Unmarshal(got, &a))
			require.NoError(t, json.Unmarshal([]byte(tc.positional), &b))
			require.Equal(t, b, a, "named form must equal the positional form")
		})
	}
}

func TestNamedParamsGapsBecomeNull(t *testing.T) {
	// targetBuckets is skipped, seriesRefs is not. A shorter array would drop
	// seriesRefs entirely; the handler gates optional params on
	// `len(params) >= n && params[n-1] != nil`, so the gap must be an
	// explicit null and the array must stay long enough to reach it.
	got, err := normalizeParams("getMetricView", json.RawMessage(
		`{"metricRef":"s","startTime":"1","endTime":"2","seriesRefs":["a"]}`))
	require.NoError(t, err)

	var out []any
	require.NoError(t, json.Unmarshal(got, &out))
	require.Len(t, out, 5, "must reach index 4, not stop at the gap")
	require.Nil(t, out[3], "the skipped parameter is null, not missing")
	require.Equal(t, []any{"a"}, out[4])
}

func TestNamedSearchLimitLeavesNullQueryGap(t *testing.T) {
	for _, method := range []string{"searchTraceSummaries", "searchLogSummaries", "searchMetricSummaries"} {
		t.Run(method, func(t *testing.T) {
			got, err := normalizeParams(method, json.RawMessage(
				`{"startTime":"1","endTime":"2","limit":5}`))
			require.NoError(t, err)
			require.JSONEq(t, `["1","2",null,5]`, string(got))
		})
	}
}

func TestNamedSearchSortLeavesEarlierGapsNull(t *testing.T) {
	for _, method := range []string{"searchTraceSummaries", "searchLogSummaries", "searchMetricSummaries"} {
		t.Run(method, func(t *testing.T) {
			got, err := normalizeParams(method, json.RawMessage(
				`{"startTime":"1","endTime":"2","sort":{"field":"name","direction":"asc"}}`))
			require.NoError(t, err)
			require.JSONEq(t, `["1","2",null,null,{"field":"name","direction":"asc"}]`, string(got))
		})
	}
}

func TestNamedParamsRejectUnknownNames(t *testing.T) {
	// Silence here would hand back results for a window the caller did not
	// ask for, which is worse than any error.
	_, err := normalizeParams("searchTraceSummaries", json.RawMessage(
		`{"startTime":"1","endTime":"2","statTime":"3"}`))
	require.Error(t, err)
	require.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)
	require.Contains(t, err.Error(), `"statTime"`, "names the typo")
	require.Contains(t, err.Error(), `"startTime"`, "lists what it does take")
}

func TestPositionalParamsPassThroughUntouched(t *testing.T) {
	// The frontend sends arrays and must be unaffected, byte for byte.
	for _, raw := range []string{`["1","2"]`, `[]`, `null`, ``} {
		got, err := normalizeParams("searchTraceSummaries", json.RawMessage(raw))
		require.NoError(t, err)
		require.Equal(t, json.RawMessage(raw), got)
	}
}

// TestEveryMethodHasParamNames catches a method added to the dispatcher
// without a name list, which would otherwise fail only when somebody first
// tried to call it by name.
func TestEveryMethodHasParamNames(t *testing.T) {
	// Methods with no positional slot to name. Either they take no parameters
	// at all, or they are variadic: parseIDParams reads the whole params array
	// as the id list, so there is no position that means one thing.
	unnamed := map[string]bool{
		"rpc.discover": true,
		"clearTraces":  true, "clearLogs": true, "clearMetrics": true,
		"getStats":                      true,
		"getServiceCount":               true,
		"getImportConfig":               true,
		"listDatabases":                 true,
		"getTraceAttributeDefinitions":  true,
		"getLogAttributeDefinitions":    true,
		"getMetricAttributeDefinitions": true,
		"deleteSpansByTraceID":          true,
		"deleteLogsByRefs":              true,
	}
	for _, m := range dispatchedMethods() {
		if unnamed[m] {
			continue
		}
		_, ok := methodParamNames[m]
		require.True(t, ok, "method %q has no entry in methodParamNames", m)
	}
}

// dispatchedMethods reads the method names out of Handle's switch in the
// source, so the coverage test above cannot be satisfied by a hand-copied
// list that quietly falls behind the dispatcher.
func dispatchedMethods() []string {
	src, err := os.ReadFile("jsonrpc_handler.go")
	if err != nil {
		panic(err)
	}
	var out []string
	for _, m := range regexp.MustCompile(`case "([a-zA-Z.]+)":`).FindAllSubmatch(src, -1) {
		out = append(out, string(m[1]))
	}
	return out
}

// TestVariadicMethodsRefuseNamedParams pins the exception, because it is the
// one shape a name table cannot express.
//
// deleteSpansByTraceID and friends read the whole params array as the list of
// ids: ["a","b"] is two ids, not one parameter holding two. Modelling that as
// a named slot would nest the array a level deeper and delete nothing. Refusal
// with an explanation beats a silent no-op.
func TestVariadicMethodsRefuseNamedParams(t *testing.T) {
	for _, method := range []string{
		"deleteSpansByTraceID", "deleteLogsByRefs",
	} {
		t.Run(method, func(t *testing.T) {
			_, err := normalizeParams(method, json.RawMessage(`{"traceIDs":["a","b"]}`))
			require.Error(t, err)
			require.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)
			require.Contains(t, err.Error(), "named parameters")

			// The array form still works, untouched.
			got, err := normalizeParams(method, json.RawMessage(`["a","b"]`))
			require.NoError(t, err)
			require.Equal(t, json.RawMessage(`["a","b"]`), got)
		})
	}
}

// TestEmptyObjectMeansNoParams covers the shape that has no name to look up.
//
// `params: {}` is an ordinary way to call a method that takes nothing, and it
// must not depend on the method appearing in the name table.
func TestEmptyObjectMeansNoParams(t *testing.T) {
	for _, method := range []string{"getStats", "clearTraces", "searchTraceSummaries"} {
		for _, raw := range []string{`{}`, ` { } `} {
			got, err := normalizeParams(method, json.RawMessage(raw))
			require.NoError(t, err, "%s with %q", method, raw)
			require.JSONEq(t, `[]`, string(got))
		}
	}
}

func TestParseSearchParams(t *testing.T) {
	got, err := parseSearchParams(json.RawMessage(`[null,20,{"query":"value"},7,{"field":"duration","direction":"desc"}]`))
	require.NoError(t, err)
	assert.Nil(t, got.timeRange.Start)
	require.NotNil(t, got.timeRange.End)
	assert.Equal(t, uint64(20), *got.timeRange.End)
	assert.Equal(t, map[string]any{"query": "value"}, got.query)
	require.NotNil(t, got.options.Limit)
	assert.Equal(t, int64(7), *got.options.Limit)
	assert.Equal(t, &search.Sort{Field: "duration", Direction: "desc"}, got.options.Sort)
}

// TestTimestampParamsAcceptNumbersWithoutLosingPrecision verifies exact numeric
// timestamp decoding.
//
// A nanosecond timestamp is around 1.8e18, far past float64's exact-integer
// limit of 2^53. Decoded the ordinary way, three of four realistic timestamps
// round -- by up to 65ns -- which would move a query boundary without failing.
// Params are decoded with UseNumber so a JSON number arrives as text and
// parses exactly; this proves the decode path, not just the parser.
func TestTimestampParamsAcceptNumbersWithoutLosingPrecision(t *testing.T) {
	// Values chosen because they do NOT survive a float64 round trip.
	lossy := []int64{
		1787348704416123456,
		1787348704416123457,
		1787277368394484963,
	}

	for _, want := range lossy {
		require.NotEqual(t, want, int64(float64(want)),
			"fixture must actually be lossy through float64, or it proves nothing")

		t.Run(fmt.Sprintf("number_%d", want), func(t *testing.T) {
			var params []any
			require.NoError(t, decodeParams(
				json.RawMessage(fmt.Sprintf(`[%d, %d]`, want, want)), &params))

			got, err := parseTimestampParam(params[0], "startTime")
			require.NoError(t, err)
			require.Equal(t, want, got, "decoded through a JSON number, exactly")
		})

		t.Run(fmt.Sprintf("string_%d", want), func(t *testing.T) {
			var params []any
			require.NoError(t, decodeParams(
				json.RawMessage(fmt.Sprintf(`["%d"]`, want)), &params))

			got, err := parseTimestampParam(params[0], "startTime")
			require.NoError(t, err)
			require.Equal(t, want, got, "the original string form still works")
		})
	}
}

// TestTimestampParamErrorsSayWhatIsWrong guards the half that matters to a
// caller who cannot read this file: the message, not just the code.
func TestTimestampParamErrorsSayWhatIsWrong(t *testing.T) {
	t.Run("float64 is refused rather than rounded", func(t *testing.T) {
		// Reaching the parser with a float64 means the decoder was bypassed and
		// the precision is already gone. Rounding it would hide that.
		_, err := parseTimestampParam(float64(1787348704416123456), "startTime")
		require.Error(t, err)
		require.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)
		require.Contains(t, err.Error(), "startTime")
		require.Contains(t, err.Error(), "float64")
	})

	t.Run("wrong type names the parameter and the type", func(t *testing.T) {
		_, err := parseTimestampParam(true, "endTime")
		require.Error(t, err)
		require.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)
		require.Contains(t, err.Error(), "endTime")
		require.Contains(t, err.Error(), "bool")
	})

	t.Run("unparseable text is quoted back", func(t *testing.T) {
		var params []any
		require.NoError(t, decodeParams(json.RawMessage(`["not-a-number"]`), &params))
		_, err := parseTimestampParam(params[0], "startTime")
		require.Error(t, err)
		require.Contains(t, err.Error(), `"not-a-number"`)
	})
}

func TestOptionalTimestampParam(t *testing.T) {
	got, err := parseOptionalTimestampParam(nil, "startTime")
	require.NoError(t, err)
	require.Nil(t, got)

	got, err = parseOptionalTimestampParam(json.Number("1787348704416123457"), "endTime")
	require.NoError(t, err)
	require.Equal(t, uint64(1787348704416123457), *got)

	got, err = parseOptionalTimestampParam("42", "startTime")
	require.NoError(t, err)
	require.Equal(t, uint64(42), *got)

	got, err = parseOptionalTimestampParam(json.Number("18446744073709551615"), "endTime")
	require.NoError(t, err)
	require.Equal(t, ^uint64(0), *got)

	_, err = parseOptionalTimestampParam("-1", "startTime")
	require.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)

	_, err = parseOptionalTimestampParam("18446744073709551616", "endTime")
	require.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)

	_, err = parseOptionalTimestampParam(true, "endTime")
	require.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)

	_, err = parseTimestampParam(nil, "targetBuckets")
	require.ErrorIs(t, err, jsonrpc2.ErrInvalidParams,
		"non-bound numeric fields must remain non-null")

	start, end := json.Number("10"), json.Number("20")
	for _, tc := range []struct {
		name             string
		startParam       any
		endParam         any
		wantStartPresent bool
		wantEndPresent   bool
	}{
		{"unbounded", nil, nil, false, false},
		{"end only", nil, end, false, true},
		{"start only", start, nil, true, false},
		{"bounded", start, end, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			timeRange, err := parseTimeRange(tc.startParam, tc.endParam)
			require.NoError(t, err)
			require.Equal(t, tc.wantStartPresent, timeRange.Start != nil)
			require.Equal(t, tc.wantEndPresent, timeRange.End != nil)
		})
	}
}
