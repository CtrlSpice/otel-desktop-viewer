package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	storequery "github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/query"
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
	"searchTraces":          {"startTime", "endTime", "query", "limit", "sort"},
	"searchSpans":           {"traceID", "query"},
	"getTrace":              {"traceID"},
	"getSpan":               {"spanID", "traceID"},
	"searchLogs":            {"startTime", "endTime", "query", "limit", "sort"},
	"getTraceLogs":          {"traceID"},
	"getLog":                {"logID"},
	"searchMetricSummaries": {"startTime", "endTime", "query", "limit", "sort"},
	"getMetric": {
		"streamID", "startTime", "endTime", "targetBuckets", "seriesIDs",
		"quantiles", "tzOffsetNs", "viewBuckets",
		"sparklineBuckets", "selectedSeriesIDs", "tzName",
		"datapointSeriesIDs", "datapointSeriesLimit",
	},
	"getMetricAggregate": {
		"streamID", "startTime", "endTime", "targetBuckets", "seriesIDs",
		"quantiles", "tzOffsetNs", "viewBuckets",
		"selectedSeriesIDs", "tzName",
	},
	"searchAttributes":       {"term"},
	"getFieldValues":         {"signal", "field", "term", "limit"},
	"getAttributesByTraceID": {"traceID"},
	"getTraceSpanCount":      {"traceID"},
	"deleteMetricStream":     {"streamID"},
	"query":                  {"sql", "limit"},
}

// normalizeParams rewrites object-form params into the positional array form.
//
// Array params and absent params pass through untouched.
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

func decodePositionalParams(raw json.RawMessage, minParams, maxParams int) ([]any, error) {
	var params []any
	if err := decodeParams(raw, &params); err != nil || len(params) < minParams || len(params) > maxParams {
		return nil, jsonrpc2.ErrInvalidParams
	}
	return params, nil
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

// getMetricParams holds the common metric request plus detail-only fields.
type getMetricParams struct {
	streamID             string
	timeRange            store.TimeRange
	targetBuckets        int64
	seriesIDs            []string
	quantiles            []float64
	tzOffsetNs           int64
	viewBuckets          int64
	sparklineBuckets     int64
	selectedSeriesIDs    []string
	datapointSeriesIDs   []string
	datapointSeriesLimit int64
	tzName               string
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

func parseOptionalQuantiles(params []any, index int) ([]float64, error) {
	if len(params) <= index || params[index] == nil {
		return nil, nil
	}
	raw, ok := params[index].([]any)
	if !ok {
		return nil, jsonrpc2.ErrInvalidParams
	}
	var quantiles []float64
	for _, value := range raw {
		var quantile float64
		switch number := value.(type) {
		case json.Number:
			parsed, err := number.Float64()
			if err != nil {
				return nil, fmt.Errorf("quantiles must be numbers, got %q: %w", number.String(), jsonrpc2.ErrInvalidParams)
			}
			quantile = parsed
		case float64:
			quantile = number
		default:
			return nil, fmt.Errorf("quantiles must be numbers, got %T: %w", value, jsonrpc2.ErrInvalidParams)
		}
		if quantile < 0 || quantile > 1 {
			return nil, fmt.Errorf("quantiles must be between 0 and 1, got %v: %w", quantile, jsonrpc2.ErrInvalidParams)
		}
		quantiles = append(quantiles, quantile)
	}
	return quantiles, nil
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

func parseGetMetricParams(raw json.RawMessage, aggregateOnly bool) (getMetricParams, error) {
	maxParams := 13
	if aggregateOnly {
		maxParams = 10
	}
	params, err := decodePositionalParams(raw, 3, maxParams)
	if err != nil {
		return getMetricParams{}, err
	}
	streamID, err := parseIDParam(params[0], ErrInvalidStreamID, normalizeUUID)
	if err != nil {
		return getMetricParams{}, err
	}
	timeRange, err := parseTimeRange(params[1], params[2])
	if err != nil {
		return getMetricParams{}, err
	}
	targetBuckets, err := parseOptionalNonNegativeInt(params, 3, "targetBuckets")
	if err != nil {
		return getMetricParams{}, err
	}
	seriesIDs, err := parseOptionalStringList(params, 4)
	if err != nil {
		return getMetricParams{}, err
	}
	quantiles, err := parseOptionalQuantiles(params, 5)
	if err != nil {
		return getMetricParams{}, err
	}
	tzOffsetNs, err := parseOptionalInt(params, 6, "tzOffsetNs")
	if err != nil {
		return getMetricParams{}, err
	}
	viewBuckets, err := parseOptionalNonNegativeInt(params, 7, "viewBuckets")
	if err != nil {
		return getMetricParams{}, err
	}

	selectedSeriesIndex, tzNameIndex := 9, 10
	var sparklineBuckets int64
	if aggregateOnly {
		selectedSeriesIndex, tzNameIndex = 8, 9
	} else {
		sparklineBuckets, err = parseOptionalNonNegativeInt(params, 8, "sparklineBuckets")
		if err != nil {
			return getMetricParams{}, err
		}
	}
	selectedSeriesIDs, err := parseOptionalStringList(params, selectedSeriesIndex)
	if err != nil {
		return getMetricParams{}, err
	}
	tzName, err := parseOptionalString(params, tzNameIndex)
	if err != nil {
		return getMetricParams{}, err
	}

	var datapointSeriesIDs []string
	var datapointSeriesLimit int64
	if !aggregateOnly {
		datapointSeriesIDs, err = parseOptionalStringList(params, 11)
		if err != nil {
			return getMetricParams{}, err
		}
		datapointSeriesLimit, err = parseOptionalNonNegativeInt(params, 12, "datapointSeriesLimit")
		if err != nil {
			return getMetricParams{}, err
		}
	}

	return getMetricParams{
		streamID: streamID, timeRange: timeRange, targetBuckets: targetBuckets,
		seriesIDs: seriesIDs, quantiles: quantiles, tzOffsetNs: tzOffsetNs,
		viewBuckets: viewBuckets, sparklineBuckets: sparklineBuckets,
		selectedSeriesIDs: selectedSeriesIDs, datapointSeriesIDs: datapointSeriesIDs,
		datapointSeriesLimit: datapointSeriesLimit, tzName: tzName,
	}, nil
}

// getFieldValues returns distinct values of one completable column, for
// value completion in the search box. Which fields complete is decided by
// each signal package's allowlist, so an unknown field is an invalid-params
// answer rather than an empty list -- a typo should look like one. The limit
// is clamped rather than trusted: the caller wants a dropdown, not a dump.
type fieldValuesParams struct {
	signal string
	field  string
	term   string
	limit  int64
}

func parseFieldValuesParams(raw json.RawMessage) (fieldValuesParams, error) {
	params, err := decodePositionalParams(raw, 4, 4)
	if err != nil {
		return fieldValuesParams{}, err
	}
	values := make([]string, 3)
	for index := range values {
		value, ok := params[index].(string)
		if !ok {
			return fieldValuesParams{}, jsonrpc2.ErrInvalidParams
		}
		values[index] = value
	}
	// Same decoder targetBuckets uses: accepts a JSON number or a string.
	limit, err := parseTimestampParam(params[3], "limit")
	if err != nil {
		return fieldValuesParams{}, err
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 500 {
		limit = 500
	}
	return fieldValuesParams{signal: values[0], field: values[1], term: values[2], limit: limit}, nil
}

// parseIDParams unmarshals a request's params as a non-empty array of entity
// IDs, validating and normalizing each element with the given normalize
// function. A malformed array returns ErrInvalidParams; a malformed element
// returns invalidIDErr (the signal-specific -3200x code). Previously these
// params went straight into SQL, where a non-string or non-UUID value became
// a DB cast error reported as a generic internal error.
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
// paths: searchSpans, getTraceLogs, getLog, getMetric, getAttributesByTraceID,
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
// Strings were the original and only accepted form, for a real reason:
// nanosecond timestamps are around 1.8e18, and JSON numbers decoded into `any`
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
// The error says which parameter and what was wrong with it. The bare
// "invalid params" this used to return gave a caller nothing to act on, which
// costs time even when the caller can read this file.
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
