package metrics

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"

	"github.com/duckdb/duckdb-go/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// StreamIdentity is the 8-field compound identity of a metric stream.
//
// All fields are strings (including IsMonotonic, which uses "true"/"false"
// with the empty string reserved for metric types that don't have a
// monotonicity property). The empty string is the natural "not
// applicable" marker for metric types that don't carry a particular
// field -- Gauge has no temporality and no monotonicity, Histogram has
// no monotonicity, etc. Storing identity exclusively as comparable
// strings lets StreamIdentity be used directly as a map key.
//
// On the database side, every identity column is NOT NULL with an
// empty-string ("" or false) default, mirroring this convention. That
// indirection (rather than letting NULL mean "N/A") is deliberate: in
// DuckDB's SQL-standard UNIQUE constraint, two NULLs are distinct, so
// "Gauge with NULL temporality" would never collide on insert and
// every batch would create a new metric_streams row. NOT-NULL columns
// avoid that entirely.
type StreamIdentity struct {
	Name                   string
	Unit                   string
	MetricType             string
	AggregationTemporality string
	IsMonotonic            string
	ScopeName              string
	ScopeVersion           string
	ServiceName            string
}

// ServiceNameFromAttrs returns the value of the resource attribute
// service.name, or empty string if it isn't set. Used both at ingest
// time (to denormalize the column on metric_streams / spans / logs)
// and as part of metric stream identity.
func ServiceNameFromAttrs(attrs pcommon.Map) string {
	if v, ok := attrs.Get("service.name"); ok {
		return v.AsString()
	}
	return ""
}

// upsertMetricStreams resolves a batch of stream identities to their UUIDs
// in metric_streams, inserting any that don't yet exist. Returns a map
// from identity to UUID for every distinct identity passed in (whether
// newly inserted or pre-existing).
//
// Implementation: two passes against the same connection.
//
//  1. INSERT ... ON CONFLICT DO NOTHING for every identity. ON CONFLICT
//     uses the UNIQUE constraint on the 8-field identity. Because every
//     identity column is NOT NULL with sensible defaults, dedup works
//     across batches without the NULL-vs-NULL pitfall.
//  2. SELECT the ids back keyed by the same 8-tuple. INSERT ... RETURNING
//     skips rows that hit ON CONFLICT, so a single SELECT after the
//     insert is the simplest way to cover both new and existing rows.
//
// Why two round-trips per batch (instead of per-identity): a typical
// OTLP request has dozens of distinct metrics. Two round-trips per
// batch keeps ingest constant-cost in identity count, which preserves
// the appender's high-volume path for datapoints/attributes/exemplars.
//
// Connection use: the duckdb driver's *Conn type implements ExecContext
// / QueryContext directly. We use them on the same conn the caller
// already holds because (a) wrapping the conn in a *sql.DB would risk
// concurrent use of the same underlying handle, and (b) the appender
// path needs the same conn and duckdb does not multiplex.
func upsertMetricStreams(ctx context.Context, conn driver.Conn, identities []StreamIdentity) (map[StreamIdentity]duckdb.UUID, error) {
	out := make(map[StreamIdentity]duckdb.UUID, len(identities))
	if len(identities) == 0 {
		return out, nil
	}

	dconn, ok := conn.(*duckdb.Conn)
	if !ok {
		return nil, fmt.Errorf("upsertMetricStreams: %w: connection is not a *duckdb.Conn", ErrMetricsStoreInternal)
	}

	// driver.Conn-level ExecContext does NOT run CheckNamedValue (that's
	// database/sql's job). When we feed values like duckdb.UUID through
	// the raw driver path, the binder sees the wrapper type and refuses
	// to bind. We approximate what database/sql would do: ask the
	// duckdb driver's CheckNamedValue first; if it returns
	// driver.ErrSkip ("I don't handle this; use default conversion"),
	// fall back to driver.DefaultParameterConverter. For values the
	// driver does handle directly (its native types), CheckNamedValue
	// mutates the NamedValue's Value field in place.
	prepareArg := func(v any) (driver.Value, error) {
		nv := driver.NamedValue{Value: v}
		err := dconn.CheckNamedValue(&nv)
		if err == nil {
			return nv.Value, nil
		}
		if !errors.Is(err, driver.ErrSkip) {
			return nil, err
		}
		return driver.DefaultParameterConverter.ConvertValue(v)
	}

	// Deduplicate inputs so the INSERT and SELECT both see one row per
	// distinct identity. Inputs are tiny (dozens), so a Go-side map
	// dedupe beats relying on the SQL deduplication to recover later.
	seen := make(map[StreamIdentity]struct{}, len(identities))
	distinct := make([]StreamIdentity, 0, len(identities))
	for _, id := range identities {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		distinct = append(distinct, id)
	}

	rowPlaceholders := make([]string, len(distinct))
	insertArgs := make([]driver.NamedValue, 0, len(distinct)*9)
	for i, id := range distinct {
		// Pass the UUID as a string literal; DuckDB casts it to UUID
		// during INSERT, and this avoids the duckdb.UUID -> [16]byte
		// fallback path that the bare driver.Conn binder doesn't
		// recognize as the UUID type.
		newID := uuid.NewString()
		rowPlaceholders[i] = "(?::uuid, ?, ?, ?, ?, ?, ?, ?, ?)"
		var err error
		insertArgs, err = appendNamedValues(insertArgs, prepareArg,
			newID,
			id.Name,
			id.Unit,
			id.MetricType,
			id.AggregationTemporality,
			isMonotonicToBool(id.IsMonotonic),
			id.ScopeName,
			id.ScopeVersion,
			id.ServiceName,
		)
		if err != nil {
			return nil, fmt.Errorf("upsertMetricStreams: %w: prep insert arg: %w", ErrMetricsStoreInternal, err)
		}
	}

	insertSQL := fmt.Sprintf(
		`insert into metric_streams (id, name, unit, metric_type, aggregation_temporality, is_monotonic, scope_name, scope_version, service_name)
		 values %s
		 on conflict (name, unit, metric_type, aggregation_temporality, is_monotonic, scope_name, scope_version, service_name) do nothing`,
		strings.Join(rowPlaceholders, ", "),
	)
	if _, err := dconn.ExecContext(ctx, insertSQL, insertArgs); err != nil {
		return nil, fmt.Errorf("upsertMetricStreams: %w: insert: %w", ErrMetricsStoreInternal, err)
	}

	// SELECT-back with one tuple-equality clause per identity, OR'd
	// together. All identity columns are NOT NULL so plain `=` is
	// sufficient -- no `is not distinct from` ceremony.
	tupleClauses := make([]string, len(distinct))
	selectArgs := make([]driver.NamedValue, 0, len(distinct)*8)
	for i, id := range distinct {
		tupleClauses[i] = `(name = ? and unit = ? and metric_type = ?
			and aggregation_temporality = ?
			and is_monotonic = ?
			and scope_name = ?
			and scope_version = ?
			and service_name = ?)`
		var err error
		selectArgs, err = appendNamedValues(selectArgs, prepareArg,
			id.Name,
			id.Unit,
			id.MetricType,
			id.AggregationTemporality,
			isMonotonicToBool(id.IsMonotonic),
			id.ScopeName,
			id.ScopeVersion,
			id.ServiceName,
		)
		if err != nil {
			return nil, fmt.Errorf("upsertMetricStreams: %w: prep select arg: %w", ErrMetricsStoreInternal, err)
		}
	}
	selectSQL := fmt.Sprintf(
		`select id, name, unit, metric_type, aggregation_temporality, is_monotonic, scope_name, scope_version, service_name
		 from metric_streams
		 where %s`,
		strings.Join(tupleClauses, " or "),
	)
	rows, err := dconn.QueryContext(ctx, selectSQL, selectArgs)
	if err != nil {
		return nil, fmt.Errorf("upsertMetricStreams: %w: select: %w", ErrMetricsStoreInternal, err)
	}
	defer rows.Close()

	// driver.Rows.Next() takes a pre-sized destination slice and returns
	// io.EOF when done -- we recreate sql.Rows-style iteration here.
	dest := make([]driver.Value, 9)
	for {
		if err := rows.Next(dest); err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, fmt.Errorf("upsertMetricStreams: %w: scan: %w", ErrMetricsStoreInternal, err)
		}
		id, err := decodeStreamID(dest[0])
		if err != nil {
			return nil, fmt.Errorf("upsertMetricStreams: %w: %w", ErrMetricsStoreInternal, err)
		}
		metricType := stringOrEmpty(dest[3])
		key := StreamIdentity{
			Name:                   stringOrEmpty(dest[1]),
			Unit:                   stringOrEmpty(dest[2]),
			MetricType:             metricType,
			AggregationTemporality: stringOrEmpty(dest[4]),
			IsMonotonic:            boolValueToIdentityString(dest[5], metricType),
			ScopeName:              stringOrEmpty(dest[6]),
			ScopeVersion:           stringOrEmpty(dest[7]),
			ServiceName:            stringOrEmpty(dest[8]),
		}
		out[key] = id
	}

	if len(out) != len(distinct) {
		// Should be impossible -- we just inserted everything we asked
		// about and the SELECT covers all of them. If it happens, the
		// likely cause is a normalization bug in the identity tuple:
		// surface it loudly rather than silently miss an identity.
		return nil, fmt.Errorf("upsertMetricStreams: %w: resolved %d of %d identities", ErrMetricsStoreInternal, len(out), len(distinct))
	}
	return out, nil
}

// appendNamedValues converts a positional argument list into the
// driver.NamedValue form that the duckdb driver's ExecContext /
// QueryContext expect, applying the supplied prep function to each
// value first. The prep function is the conn's CheckNamedValue, which
// unwraps duckdb-specific wrapper types (UUID, etc.) to their raw
// driver.Value forms. Ordinal is 1-based per database/sql convention.
func appendNamedValues(args []driver.NamedValue, prep func(any) (driver.Value, error), vs ...any) ([]driver.NamedValue, error) {
	for _, v := range vs {
		val, err := prep(v)
		if err != nil {
			return nil, err
		}
		args = append(args, driver.NamedValue{
			Ordinal: len(args) + 1,
			Value:   val,
		})
	}
	return args, nil
}

// isMonotonicToBool maps the IsMonotonic identity string ("true",
// "false", or "" for not-applicable metric types) onto the boolean
// stored in metric_streams.is_monotonic. Anything other than "true"
// becomes false, including the not-applicable case -- readers know
// from metric_type whether monotonicity is meaningful.
func isMonotonicToBool(s string) bool {
	return s == "true"
}

// decodeStreamID normalizes a UUID coming back from the driver.Conn
// QueryContext path into a duckdb.UUID. The duckdb driver may surface a
// UUID as either a duckdb.UUID directly, or (when the server has cast
// it from a string at INSERT time) as a raw []byte / [16]byte. Handle
// both shapes here so callers can keep working with duckdb.UUID
// regardless of how the value happened to be returned.
func decodeStreamID(v driver.Value) (duckdb.UUID, error) {
	switch t := v.(type) {
	case duckdb.UUID:
		return t, nil
	case [16]byte:
		return duckdb.UUID(t), nil
	case []byte:
		if len(t) != 16 {
			return duckdb.UUID{}, fmt.Errorf("decodeStreamID: expected 16 bytes, got %d", len(t))
		}
		var u duckdb.UUID
		copy(u[:], t)
		return u, nil
	case string:
		parsed, err := uuid.Parse(t)
		if err != nil {
			return duckdb.UUID{}, fmt.Errorf("decodeStreamID: parse %q: %w", t, err)
		}
		return duckdb.UUID(parsed), nil
	default:
		return duckdb.UUID{}, fmt.Errorf("decodeStreamID: unsupported value type %T", v)
	}
}

// stringOrEmpty reads a driver.Value as a string, returning "" for nil
// (which is how the duckdb driver represents SQL NULL). Used while
// reconstructing a StreamIdentity from a returned row.
func stringOrEmpty(v driver.Value) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// boolValueToIdentityString maps a driver.Value (bool) back into the
// identity-string form ("true"/"false") used by StreamIdentity. The
// metric_type drives the not-applicable case: Gauge / Histogram /
// ExponentialHistogram / Empty all carry an empty IsMonotonic in the
// identity tuple regardless of the (defaulted-false) stored value, so
// roundtripped identities compare equal to the originals. Sum is the
// only type for which is_monotonic is part of the wire schema.
func boolValueToIdentityString(v driver.Value, metricType string) string {
	if metricType != "Sum" {
		return ""
	}
	if b, ok := v.(bool); ok {
		if b {
			return "true"
		}
		return "false"
	}
	return ""
}
