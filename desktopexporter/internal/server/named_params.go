package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"golang.org/x/exp/jsonrpc2"
)

// methodParamNames names each method's positional parameters, in order.
//
// JSON-RPC 2.0 permits params as an array or an object, and this is what makes
// the object form work: a named request is reordered into the positional array
// the handlers already read, so nothing downstream changes shape.
//
// The names are not invented here. Each is the name the same value already
// carries in the Go store signature and in the TypeScript client -- streamID,
// targetBuckets, tzOffsetNs -- and several were already spelled out in this
// file's own error messages. Spelling follows the wire, which emits traceID
// and spanID, so a caller passes back the name it was given.
//
// Adding a parameter means appending here as well, and a test walks every
// method to catch a list that has fallen behind its handler's bounds.
//
// deleteSpansByTraceID and deleteLogByID are deliberately
// absent. They are variadic -- parseIDParams reads the whole params array as
// the list of ids, so ["a","b"] is two ids rather than one parameter holding
// two. There is no position to give a name to, and modelling them as a single
// named slot would nest the array one level deeper and break the delete. A
// named call to them is refused with a message saying so, which is the honest
// answer.
var methodParamNames = map[string][]string{
	"searchTraces":          {"startTime", "endTime", "query"},
	"searchSpans":           {"traceID", "query"},
	"searchLogs":            {"startTime", "endTime", "query"},
	"getLog":                {"logID"},
	"searchMetricSummaries": {"startTime", "endTime", "query"},
	"getMetric": {
		"streamID", "startTime", "endTime", "targetBuckets", "seriesIDs",
		"quantiles", "tzOffsetNs", "fitToData", "viewBuckets",
		"sparklineBuckets", "selectedSeriesIDs", "datapointSeriesIDs",
		"datapointSeriesLimit", "tzName",
	},
	"getMetricAggregate": {
		"streamID", "startTime", "endTime", "targetBuckets", "seriesIDs",
		"quantiles", "tzOffsetNs", "fitToData", "viewBuckets",
		"sparklineBuckets", "selectedSeriesIDs", "datapointSeriesIDs",
		"datapointSeriesLimit", "tzName",
	},
	"getTraceAttributes":     {"startTime", "endTime"},
	"getLogAttributes":       {"startTime", "endTime"},
	"getMetricAttributes":    {"startTime", "endTime"},
	"searchAttributes":       {"term"},
	"getAttributesByTraceID": {"traceID"},
	"getTraceSpanCount":      {"traceID"},
	"deleteMetricStream":     {"streamID"},
}

// normalizeParams rewrites object-form params into the positional array form.
//
// Array params and absent params pass through untouched, so this cannot
// change the behaviour of any existing caller -- the frontend sends arrays and
// keeps doing so.
//
// Two decisions worth stating. A gap between named parameters becomes an
// explicit null rather than a shorter array, because the handlers gate
// optional parameters on `len(params) >= n && params[n-1] != nil` and a
// shorter array would silently drop everything after the gap. And an unknown
// name is an error rather than being ignored: a caller who misspells
// `startTime` should be told, not handed results for a window they did not
// ask for. Unknown names are the single most likely mistake here, and
// silence is the worst possible answer to it.
func normalizeParams(method string, raw json.RawMessage) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return raw, nil
	}

	// An empty object means "no parameters", which is true of every method --
	// including the ones with nothing to name. Rejecting it would break
	// `params: {}`, a perfectly ordinary way to call getStats.
	if bytes.Equal(bytes.Join(bytes.Fields(trimmed), nil), []byte("{}")) {
		return json.RawMessage("[]"), nil
	}

	names, ok := methodParamNames[method]
	if !ok {
		return nil, fmt.Errorf(
			"%s does not accept named parameters: %w", method, jsonrpc2.ErrInvalidParams)
	}

	var byName map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	if err := dec.Decode(&byName); err != nil {
		return nil, fmt.Errorf("params: %w: %w", jsonrpc2.ErrInvalidParams, err)
	}

	index := make(map[string]int, len(names))
	for i, n := range names {
		index[n] = i
	}

	highest := -1
	positional := make([]json.RawMessage, len(names))
	var unknown []string
	for name, value := range byName {
		i, ok := index[name]
		if !ok {
			unknown = append(unknown, name)
			continue
		}
		positional[i] = value
		if i > highest {
			highest = i
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, fmt.Errorf(
			"%s has no parameter %s; it takes %s: %w",
			method, strings.Join(quoteAll(unknown), ", "),
			strings.Join(quoteAll(names), ", "), jsonrpc2.ErrInvalidParams)
	}

	// Trailing absent parameters are simply not sent; interior ones become
	// null, which is what the optional-parameter checks already expect.
	positional = positional[:highest+1]
	for i, v := range positional {
		if v == nil {
			positional[i] = json.RawMessage("null")
		}
	}

	out, err := json.Marshal(positional)
	if err != nil {
		return nil, fmt.Errorf("params: %w: %w", jsonrpc2.ErrInternal, err)
	}
	return out, nil
}

func quoteAll(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = `"` + s + `"`
	}
	return out
}
