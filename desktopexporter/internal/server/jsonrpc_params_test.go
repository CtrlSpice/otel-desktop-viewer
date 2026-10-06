package server

import (
	"encoding/json"
	"fmt"
	"math"
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
		{"searchTraces",
			`{"startTime":"1","endTime":"2"}`, `["1","2"]`},
		{"searchSpans",
			`{"traceID":"abc"}`, `["abc"]`},
		{"getLog",
			`{"logID":"L1"}`, `["L1"]`},
		{"searchAttributes",
			`{"term":"http"}`, `["http"]`},
		{"query",
			`{"limit":5,"sql":"select 1"}`, `["select 1",5]`},
		{"getMetric",
			`{"metricID":"m"}`, `["m"]`},
		{"getMetricSeries",
			`{"endTime":null,"seriesID":"s","metricID":"m","startTime":"1"}`,
			`["m","s","1",null]`},
		// Order in the object must not matter.
		{"searchLogs",
			`{"endTime":"2","startTime":"1"}`, `["1","2"]`},
		// The wide one, fully populated, in a deliberately shuffled order.
		{"getMetricView",
			`{"tzName":"UTC","metricID":"s","startTime":"1","endTime":"2",
			  "targetBuckets":10,"seriesIDs":["a"],"quantiles":[0.5],
			  "tzOffsetNs":0,"viewBuckets":5,
			  "sparklineBuckets":6,"selectedSeriesIDs":["b"],
			  "datapointSeriesIDs":["c"],"datapointSeriesLimit":7}`,
			`["s","1","2",10,["a"],[0.5],0,5,6,["b"],"UTC",["c"],7]`},
		{"getMetricAggregateView",
			`{"tzName":"UTC","metricID":"s","startTime":"1","endTime":"2",
			  "targetBuckets":10,"seriesIDs":["a"],"quantiles":[0.5],
			  "tzOffsetNs":0,"viewBuckets":5,"selectedSeriesIDs":["b"]}`,
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

func TestMetricNamedParamContracts(t *testing.T) {
	require.Equal(t, []string{"metricID"}, methodParamNames["getMetric"])
	require.Equal(t, []string{"metricID", "seriesID", "startTime", "endTime"}, methodParamNames["getMetricSeries"])
	require.Equal(t, []string{
		"metricID", "startTime", "endTime", "targetBuckets", "seriesIDs",
		"quantiles", "tzOffsetNs", "viewBuckets", "sparklineBuckets",
		"selectedSeriesIDs", "tzName", "datapointSeriesIDs", "datapointSeriesLimit",
	}, methodParamNames["getMetricView"])
	require.Equal(t, []string{
		"metricID", "startTime", "endTime", "targetBuckets", "seriesIDs",
		"quantiles", "tzOffsetNs", "viewBuckets", "selectedSeriesIDs", "tzName",
	}, methodParamNames["getMetricAggregateView"])
}

func TestNamedParamsGapsBecomeNull(t *testing.T) {
	// targetBuckets is skipped, seriesIDs is not. A shorter array would drop
	// seriesIDs entirely; the handler gates optional params on
	// `len(params) >= n && params[n-1] != nil`, so the gap must be an
	// explicit null and the array must stay long enough to reach it.
	got, err := normalizeParams("getMetricView", json.RawMessage(
		`{"metricID":"s","startTime":"1","endTime":"2","seriesIDs":["a"]}`))
	require.NoError(t, err)

	var out []any
	require.NoError(t, json.Unmarshal(got, &out))
	require.Len(t, out, 5, "must reach index 4, not stop at the gap")
	require.Nil(t, out[3], "the skipped parameter is null, not missing")
	require.Equal(t, []any{"a"}, out[4])
}

func TestNamedSearchLimitLeavesNullQueryGap(t *testing.T) {
	for _, method := range []string{"searchTraces", "searchLogs", "searchMetricSummaries"} {
		t.Run(method, func(t *testing.T) {
			got, err := normalizeParams(method, json.RawMessage(
				`{"startTime":"1","endTime":"2","limit":5}`))
			require.NoError(t, err)
			require.JSONEq(t, `["1","2",null,5]`, string(got))
		})
	}
}

func TestNamedSearchSortLeavesEarlierGapsNull(t *testing.T) {
	for _, method := range []string{"searchTraces", "searchLogs", "searchMetricSummaries"} {
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
	_, err := normalizeParams("searchTraces", json.RawMessage(
		`{"startTime":"1","endTime":"2","statTime":"3"}`))
	require.Error(t, err)
	require.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)
	require.Contains(t, err.Error(), `"statTime"`, "names the typo")
	require.Contains(t, err.Error(), `"startTime"`, "lists what it does take")
}

func TestPositionalParamsPassThroughUntouched(t *testing.T) {
	// The frontend sends arrays and must be unaffected, byte for byte.
	for _, raw := range []string{`["1","2"]`, `[]`, `null`, ``} {
		got, err := normalizeParams("searchTraces", json.RawMessage(raw))
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
		"clearTraces": true, "clearLogs": true, "clearMetrics": true,
		"getStats":             true,
		"getTraceAttributes":   true,
		"getLogAttributes":     true,
		"getMetricAttributes":  true,
		"deleteSpansByTraceID": true,
		"deleteLogByID":        true,
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
	for _, m := range regexp.MustCompile(`case "([a-zA-Z]+)":`).FindAllSubmatch(src, -1) {
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
		"deleteSpansByTraceID", "deleteLogByID",
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

// TestDeleteMetricStreamTakesOneID guards the singular/plural slip this
// nearly shipped with: the method takes a bare id, not a list.
func TestDeleteMetricStreamTakesOneID(t *testing.T) {
	got, err := normalizeParams("deleteMetricStream", json.RawMessage(`{"streamID":"s1"}`))
	require.NoError(t, err)
	require.JSONEq(t, `["s1"]`, string(got))

	_, err = normalizeParams("deleteMetricStream", json.RawMessage(`{"streamIDs":["s1"]}`))
	require.Error(t, err, "the plural name must not silently work")
}

// TestEmptyObjectMeansNoParams covers the shape that has no name to look up.
//
// `params: {}` is an ordinary way to call a method that takes nothing, and it
// must not depend on the method appearing in the name table -- getStats has no
// entry and never will. Found by calling getStats after the table went in, not
// by any test above, which is why it is a test now.
func TestEmptyObjectMeansNoParams(t *testing.T) {
	for _, method := range []string{"getStats", "clearTraces", "searchTraces"} {
		for _, raw := range []string{`{}`, ` { } `} {
			got, err := normalizeParams(method, json.RawMessage(raw))
			require.NoError(t, err, "%s with %q", method, raw)
			require.JSONEq(t, `[]`, string(got))
		}
	}
}

func TestParseQueryParams(t *testing.T) {
	for _, tc := range []struct {
		name          string
		raw           string
		wantStatement string
		wantLimit     uint64
		wantError     string
	}{
		{name: "default limit", raw: `["select 1"]`, wantStatement: "select 1", wantLimit: 25},
		{name: "explicit maximum", raw: `["select 1",18446744073709551615]`, wantStatement: "select 1", wantLimit: math.MaxUint64},
		{name: "wrong count", raw: `[]`, wantError: "invalid params"},
		{name: "statement type", raw: `[1]`, wantError: "sql must be a string"},
		{name: "limit type", raw: `["select 1",null]`, wantError: "limit must be a non-negative whole number"},
		{name: "negative limit", raw: `["select 1",-1]`, wantError: "limit must be a non-negative whole number"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseQueryParams(json.RawMessage(tc.raw))
			if tc.wantError != "" {
				require.Error(t, err)
				assert.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)
				assert.Contains(t, err.Error(), tc.wantError)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantStatement, got.statement)
			assert.Equal(t, tc.wantLimit, got.limit)
		})
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

func TestParseGetMetricViewParamsLayouts(t *testing.T) {
	metricID := "00000000-0000-0000-0000-000000000001"
	detailRaw := json.RawMessage(fmt.Sprintf(`[%q,null,20,4,[],[0.5],-3600,8,9,["selected"],"Europe/London",[],10]`, metricID))
	detail, err := parseGetMetricViewParams(detailRaw, false)
	require.NoError(t, err)
	assert.Equal(t, metricID, detail.metricID)
	assert.NotNil(t, detail.seriesIDs)
	assert.Empty(t, detail.seriesIDs)
	assert.Equal(t, []float64{0.5}, detail.quantiles)
	assert.Equal(t, int64(-3600), detail.tzOffsetNs)
	assert.Equal(t, int64(9), detail.sparklineBuckets)
	assert.Equal(t, []string{"selected"}, detail.selectedSeriesIDs)
	assert.Equal(t, "Europe/London", detail.tzName)
	assert.NotNil(t, detail.datapointSeriesIDs)
	assert.Empty(t, detail.datapointSeriesIDs)
	assert.Equal(t, int64(10), detail.datapointSeriesLimit)

	aggregateRaw := json.RawMessage(fmt.Sprintf(`[%q,10,null,4,null,null,null,8,["selected"],"UTC"]`, metricID))
	aggregate, err := parseGetMetricViewParams(aggregateRaw, true)
	require.NoError(t, err)
	assert.Equal(t, []string{"selected"}, aggregate.selectedSeriesIDs)
	assert.Equal(t, "UTC", aggregate.tzName)
	assert.Zero(t, aggregate.sparklineBuckets)
	assert.Nil(t, aggregate.datapointSeriesIDs)
}

func TestParseGetMetricSeriesParams(t *testing.T) {
	metricID := "00000000-0000-0000-0000-000000000001"
	seriesID := "00000000-0000-0000-0000-000000000002"
	got, err := parseGetMetricSeriesParams(json.RawMessage(fmt.Sprintf(`[%q,%q,null,"18446744073709551615"]`, metricID, seriesID)))
	require.NoError(t, err)
	assert.Equal(t, metricID, got.metricID)
	assert.Equal(t, seriesID, got.seriesID)
	assert.Nil(t, got.timeRange.Start)
	require.NotNil(t, got.timeRange.End)
	assert.Equal(t, uint64(math.MaxUint64), *got.timeRange.End)
}

func TestParseOptionalQuantilesPreservesValuesOrderAndPresence(t *testing.T) {
	absent, err := parseOptionalQuantiles([]any{}, 0)
	require.NoError(t, err)
	assert.Nil(t, absent)

	empty, err := parseOptionalQuantiles([]any{[]any{}}, 0)
	require.NoError(t, err)
	assert.Nil(t, empty)

	values, err := parseOptionalQuantiles([]any{[]any{
		json.Number("0.12345678901234566"),
		json.Number("0.9876543210987654"),
		float64(0.5),
	}}, 0)
	require.NoError(t, err)
	assert.Equal(t, []float64{0.12345678901234566, 0.9876543210987654, 0.5}, values)
	assert.Equal(t, []uint64{
		math.Float64bits(0.12345678901234566),
		math.Float64bits(0.9876543210987654),
		math.Float64bits(0.5),
	}, []uint64{
		math.Float64bits(values[0]),
		math.Float64bits(values[1]),
		math.Float64bits(values[2]),
	})

	for _, params := range [][]any{
		{[]any{json.Number("1e999")}},
		{[]any{json.Number("1.1")}},
		{[]any{"0.5"}},
	} {
		_, err := parseOptionalQuantiles(params, 0)
		assert.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)
	}
}

func TestParseFieldValuesParamsClampsLimit(t *testing.T) {
	for _, tc := range []struct {
		limit int
		want  int64
	}{{limit: -5, want: 1}, {limit: 10, want: 10}, {limit: 1000, want: 500}} {
		raw := json.RawMessage(fmt.Sprintf(`["traces","name","term",%d]`, tc.limit))
		got, err := parseFieldValuesParams(raw)
		require.NoError(t, err)
		assert.Equal(t, fieldValuesParams{signal: "traces", field: "name", term: "term", limit: tc.want}, got)
	}
}

// TestTimestampParamsAcceptNumbersWithoutLosingPrecision covers the trap that
// numeric timestamps used to fall into.
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
