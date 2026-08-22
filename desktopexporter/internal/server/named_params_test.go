package server

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"

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
		{"getTraceAttributes",
			`{"startTime":1,"endTime":2}`, `[1,2]`},
		{"searchAttributes",
			`{"term":"http"}`, `["http"]`},
		// Order in the object must not matter.
		{"searchLogs",
			`{"endTime":"2","startTime":"1"}`, `["1","2"]`},
		// The wide one, fully populated, in a deliberately shuffled order.
		{"getMetric",
			`{"tzName":"UTC","streamID":"s","startTime":"1","endTime":"2",
			  "targetBuckets":10,"seriesIDs":["a"],"quantiles":[0.5],
			  "tzOffsetNs":0,"fitToData":true,"viewBuckets":5,
			  "sparklineBuckets":6,"selectedSeriesIDs":["b"],
			  "datapointSeriesIDs":["c"],"datapointSeriesLimit":7}`,
			`["s","1","2",10,["a"],[0.5],0,true,5,6,["b"],["c"],7,"UTC"]`},
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
	// targetBuckets is skipped, seriesIDs is not. A shorter array would drop
	// seriesIDs entirely; the handler gates optional params on
	// `len(params) >= n && params[n-1] != nil`, so the gap must be an
	// explicit null and the array must stay long enough to reach it.
	got, err := normalizeParams("getMetric", json.RawMessage(
		`{"streamID":"s","startTime":"1","endTime":"2","seriesIDs":["a"]}`))
	require.NoError(t, err)

	var out []any
	require.NoError(t, json.Unmarshal(got, &out))
	require.Len(t, out, 5, "must reach index 4, not stop at the gap")
	require.Nil(t, out[3], "the skipped parameter is null, not missing")
	require.Equal(t, []any{"a"}, out[4])
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
	// Methods that genuinely take no parameters.
	noParams := map[string]bool{
		"clearTraces": true, "clearLogs": true, "clearMetrics": true,
		"getStats": true,
	}
	for _, m := range dispatchedMethods() {
		if noParams[m] {
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
