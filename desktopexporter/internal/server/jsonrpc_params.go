package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/google/uuid"
	"golang.org/x/exp/jsonrpc2"
)

// methodParamNames names each method's positional parameters, in order.
//
// JSON-RPC 2.0 permits params as an array or an object, and this is what makes
// the object form work: a named request is reordered into the positional array
// the handlers already read, so nothing downstream changes shape.
//
// Names match the Go store signatures and wire fields.
//
// Adding a parameter means appending here as well, and a test walks every
// method to catch a list that has fallen behind its handler's bounds.
//
// deleteSpansByTraceID and deleteLogsByRefs are deliberately
// absent. They are variadic -- parseIDParams reads the whole params array as
// the list of ids, so ["a","b"] is two ids rather than one parameter holding
// two. There is no position to give a name to, and modelling them as a single
// named slot would nest the array one level deeper and break the delete. A
// named call to them is refused with a message saying so, which is the honest
// answer.
var methodParamNames = map[string][]string{
	"getServiceLabels":       {"serviceRef"},
	"discoverTelemetry":      {"options"},
	"searchTraceSummaries":   {"startTime", "endTime", "query", "limit", "sort"},
	"getTraceView":           {"traceID", "query", "spanIDs", "columns"},
	"getTraceColumnPreviews": {"traceID", "columns"},
	"getTraceOverview":       {"traceID"},
	"getSpan":                {"spanID", "traceID", "limit"},
	"searchLogSummaries":     {"startTime", "endTime", "query", "limit", "sort"},
	"searchLogPage":          {"startTime", "endTime", "query", "limit", "sort", "pageSize", "offset", "columns"},
	"getLogErrorNavigation":  {"startTime", "endTime", "query", "limit", "sort", "selectedLogRef"},
	"getTraceLogSummaries":   {"traceID"},
	"getLog":                 {"logRef"},
	"searchMetricSummaries":  {"startTime", "endTime", "query", "limit", "sort"},
	"getMetric":              {"metricRef"},
	"getMetricSeries":        {"metricRef", "seriesRef", "startTime", "endTime", "includeDatapoints"},
	"getMetricSeriesOrder":   {"metricRef", "startTime", "endTime", "seriesRefs", "key", "direction"},
	"getMetricView": {
		"metricRef", "startTime", "endTime", "targetBuckets", "seriesRefs",
		"quantiles", "tzOffsetNs", "viewBuckets",
		"sparklineBuckets", "selectedSeriesRefs", "tzName",
		"datapointSeriesRefs", "datapointSeriesLimit",
	},
	"getMetricAggregateView": {
		"metricRef", "startTime", "endTime", "targetBuckets", "seriesRefs",
		"quantiles", "tzOffsetNs", "viewBuckets",
		"selectedSeriesRefs", "tzName", "datapointRef",
	},
	"searchAttributeMatches":                {"term", "signal", "owner", "path", "type"},
	"getFieldValueCompletions":              {"signal", "field", "term", "limit"},
	"getTraceAttributeDefinitionsByTraceID": {"traceID"},
	"getTraceSpanCount":                     {"traceID"},
	"deleteMetric":                          {"metricRef"},
	"query":                                 {"sql", "limit"},
}

// normalizeParams rewrites object-form params into the positional array form.
//
// Array params and absent params pass through untouched.
//
// A gap between named parameters becomes an
// explicit null rather than a shorter array, because the handlers gate
// optional parameters on `len(params) >= n && params[n-1] != nil` and a
// shorter array would silently drop everything after the gap. And an unknown
// name is an error rather than being ignored: a caller who misspells
// `startTime` must not receive results for a different window.
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

func decodePositionalParams(raw json.RawMessage, minParams, maxParams int) ([]any, error) {
	var params []any
	if err := decodeParams(raw, &params); err != nil || len(params) < minParams || len(params) > maxParams {
		return nil, jsonrpc2.ErrInvalidParams
	}
	return params, nil
}

type searchParams struct {
	timeRange store.TimeRange
	query     any
	options   search.ResultOptions
}

func parseSearchParams(raw json.RawMessage) (searchParams, error) {
	params, err := decodePositionalParams(raw, 2, 5)
	if err != nil {
		return searchParams{}, err
	}
	return searchParamsFromValues(params)
}

func searchParamsFromValues(params []any) (searchParams, error) {
	timeRange, err := parseTimeRange(params[0], params[1])
	if err != nil {
		return searchParams{}, err
	}
	var query any
	if len(params) >= 3 {
		query = params[2]
	}
	options, err := parseSearchOptions(params)
	if err != nil {
		return searchParams{}, err
	}
	return searchParams{timeRange: timeRange, query: query, options: options}, nil
}

func parseSearchLimit(params []any) (*int64, error) {
	if len(params) < 4 || params[3] == nil {
		return nil, nil
	}
	limit, err := parseTimestampParam(params[3], "limit")
	if err != nil {
		return nil, err
	}
	if limit < 1 {
		return nil, fmt.Errorf("limit must be positive: %w", jsonrpc2.ErrInvalidParams)
	}
	return &limit, nil
}

func parseSearchOptions(params []any) (search.ResultOptions, error) {
	limit, err := parseSearchLimit(params)
	if err != nil {
		return search.ResultOptions{}, err
	}
	if len(params) < 5 || params[4] == nil {
		return search.ResultOptions{Limit: limit}, nil
	}

	rawSort, ok := params[4].(map[string]any)
	if !ok {
		return search.ResultOptions{}, fmt.Errorf("sort must be an object: %w", jsonrpc2.ErrInvalidParams)
	}
	field, fieldOK := rawSort["field"].(string)
	direction, directionOK := rawSort["direction"].(string)
	if !fieldOK || !directionOK || len(rawSort) != 2 {
		return search.ResultOptions{}, fmt.Errorf("sort must contain only string field and direction properties: %w", jsonrpc2.ErrInvalidParams)
	}
	return search.ResultOptions{
		Limit: limit,
		Sort:  &search.Sort{Field: field, Direction: direction},
	}, nil
}

func parseOptionalInt(params []any, index int, name string) (int64, error) {
	if len(params) <= index || params[index] == nil {
		return 0, nil
	}
	return parseTimestampParam(params[index], name)
}

func parseOptionalNonNegativeInt(params []any, index int, name string) (int64, error) {
	value, err := parseOptionalInt(params, index, name)
	if err != nil {
		return 0, err
	}
	if value < 0 {
		return 0, jsonrpc2.ErrInvalidParams
	}
	return value, nil
}

func parseOptionalStringList(params []any, index int) ([]string, error) {
	if len(params) <= index || params[index] == nil {
		return nil, nil
	}
	raw, ok := params[index].([]any)
	if !ok {
		return nil, jsonrpc2.ErrInvalidParams
	}
	values := make([]string, 0, len(raw))
	for _, value := range raw {
		text, ok := value.(string)
		if !ok {
			return nil, jsonrpc2.ErrInvalidParams
		}
		values = append(values, text)
	}
	return values, nil
}

func parseOptionalString(params []any, index int) (string, error) {
	if len(params) <= index || params[index] == nil {
		return "", nil
	}
	value, ok := params[index].(string)
	if !ok {
		return "", jsonrpc2.ErrInvalidParams
	}
	return value, nil
}

// parseIDParams unmarshals a request's params as a non-empty array of entity
// IDs, validating and normalizing each element with the given normalize
// function. A malformed array returns ErrInvalidParams; a malformed element
// returns invalidIDErr (the signal-specific -3200x code).
func parseIDParams(raw json.RawMessage, invalidIDErr error, normalize func(string) (string, error)) ([]any, error) {
	var params []any
	if err := decodeParams(raw, &params); err != nil {
		return nil, jsonrpc2.ErrInvalidParams
	}

	if len(params) == 0 {
		return nil, jsonrpc2.ErrInvalidParams
	}

	ids := make([]any, 0, len(params))
	for _, param := range params {
		s, ok := param.(string)
		if !ok {
			return nil, invalidIDErr
		}
		id, err := normalize(s)
		if err != nil {
			return nil, invalidIDErr
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// parseIDParam validates and normalizes a single entity ID param (read
// paths: getTraceView, getTraceLogSummaries, getLog, getMetric, getTraceAttributeDefinitionsByTraceID,
// getTraceSpanCount). Like parseIDParams, a bad value returns the
// signal-specific -3200x code instead of reaching SQL as a cast error.
func parseIDParam(param any, invalidIDErr error, normalize func(string) (string, error)) (string, error) {
	s, ok := param.(string)
	if !ok {
		return "", invalidIDErr
	}
	id, err := normalize(s)
	if err != nil {
		return "", invalidIDErr
	}
	return id, nil
}

func parseSingleIDParam(raw json.RawMessage, invalidIDErr error, normalize func(string) (string, error)) (string, error) {
	params, err := decodePositionalParams(raw, 1, 1)
	if err != nil {
		return "", err
	}
	return parseIDParam(params[0], invalidIDErr, normalize)
}

func parseSingleStringParam(raw json.RawMessage) (string, error) {
	params, err := decodePositionalParams(raw, 1, 1)
	if err != nil {
		return "", err
	}
	value, ok := params[0].(string)
	if !ok {
		return "", jsonrpc2.ErrInvalidParams
	}
	return value, nil
}

// normalizeUUID validates a 128-bit entity ID (trace IDs: 32-char hex on the
// wire; log IDs: tool-minted dashed UUIDs; both stored in uuid columns) and
// returns it in canonical dashed form. Accepts 32-char hex and
// UUID-with-dashes. The length gate is NOT redundant with uuid.Parse: it
// exists to reject the braced {...} and urn:uuid: forms Parse would
// otherwise accept, keeping the API surface at exactly the two shapes we
// serve.
func normalizeUUID(s string) (string, error) {
	if len(s) != 32 && len(s) != 36 {
		return "", fmt.Errorf("ID must be 32-char hex or a dashed UUID, got %d chars", len(s))
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

// parseTimestampParam reads a whole number sent either as a JSON string or as
// a JSON number.
//
// Nanosecond timestamps are around 1.8e18, and JSON numbers decoded into `any`
// become float64, which is exact only to 2^53. Three of four realistic
// timestamps lose precision that way -- by up to 65ns -- which would move a
// query boundary silently rather than failing.
//
// So numbers are accepted, but only because params are decoded with
// UseNumber: a JSON number arrives as json.Number, which is text, and parses
// to int64 exactly. float64 is rejected outright rather than rounded, since
// reaching this function with one means the decoder was bypassed and the
// precision is already gone.
//
// Errors identify the parameter and invalid value.
func parseTimestampParam(param any, paramName string) (int64, error) {
	var text string
	switch v := param.(type) {
	case string:
		text = v
	case json.Number:
		text = v.String()
	case float64:
		return 0, fmt.Errorf(
			"%s decoded as float64, which cannot hold a nanosecond timestamp exactly: %w",
			paramName, jsonrpc2.ErrInvalidParams)
	default:
		return 0, fmt.Errorf(
			"%s must be a whole number, as a JSON number or a decimal string, got %T: %w",
			paramName, param, jsonrpc2.ErrInvalidParams)
	}

	parsed, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, fmt.Errorf(
			"%s must be a whole number, got %q: %w",
			paramName, text, jsonrpc2.ErrInvalidParams)
	}
	return parsed, nil
}

// parseOptionalTimestampParam is reserved for nullable time bounds. All other
// numeric parameters keep parseTimestampParam's strict non-null contract.
func parseOptionalTimestampParam(param any, paramName string) (*uint64, error) {
	if param == nil {
		return nil, nil
	}
	var text string
	switch v := param.(type) {
	case string:
		text = v
	case json.Number:
		text = v.String()
	case float64:
		return nil, fmt.Errorf(
			"%s decoded as float64, which cannot hold a nanosecond timestamp exactly: %w",
			paramName, jsonrpc2.ErrInvalidParams)
	default:
		return nil, fmt.Errorf(
			"%s must be an unsigned whole number, as a JSON number or a decimal string, got %T: %w",
			paramName, param, jsonrpc2.ErrInvalidParams)
	}
	parsed, err := strconv.ParseUint(text, 10, 64)
	if err != nil {
		return nil, fmt.Errorf(
			"%s must be an unsigned whole number, got %q: %w",
			paramName, text, jsonrpc2.ErrInvalidParams)
	}
	return &parsed, nil
}

func parseTimeRange(startParam, endParam any) (store.TimeRange, error) {
	startTime, err := parseOptionalTimestampParam(startParam, "startTime")
	if err != nil {
		return store.TimeRange{}, err
	}
	endTime, err := parseOptionalTimestampParam(endParam, "endTime")
	if err != nil {
		return store.TimeRange{}, err
	}
	return store.TimeRange{Start: startTime, End: endTime}, nil
}

// decodeParams unmarshals a request's params with UseNumber, so JSON numbers
// arrive as json.Number rather than float64 and keep full integer precision.
//
// Every params decode in this file goes through here. Using json.Unmarshal
// directly would silently reintroduce the float64 rounding that
// parseTimestampParam exists to avoid.
func decodeParams(raw json.RawMessage, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	return dec.Decode(dst)
}

func validateNoParams(raw json.RawMessage) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var params []any
	if err := decodeParams(raw, &params); err != nil || len(params) != 0 {
		return jsonrpc2.ErrInvalidParams
	}
	return nil
}
