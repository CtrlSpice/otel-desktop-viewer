package ingest

import (
	"fmt"
	"sort"
	"strings"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
	"github.com/duckdb/duckdb-go/v2"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// AttributeOwnerIDs specifies which entity owns the attributes.
// Populate only the IDs that apply; leave others nil.
//
// MetricIngestID is the per-batch metric record (formerly MetricID, when
// the metrics table held identity). Resource and scope attributes for a
// metric live with the metric_ingest row, so a fresh batch of the same
// logical stream gets a fresh set of attribute rows -- which is what the
// OTel data model wants since resource/scope attributes are per-batch in
// principle (the same SDK could in theory ship an updated resource on a
// later batch, and we want to preserve that history).
type AttributeOwnerIDs struct {
	SpanID         *duckdb.UUID
	EventID        *duckdb.UUID
	LinkID         *duckdb.UUID
	LogID          *duckdb.UUID
	MetricIngestID *duckdb.UUID
	DataPointID    *duckdb.UUID
	ExemplarID     *duckdb.UUID
}

// AttributeBatchItem pairs an attribute map with the entity IDs and scope that own it.
// Scope is the semantic owner: "resource", "scope", "span", "event", "link" (or "log", "metric", etc. for other signals).
type AttributeBatchItem struct {
	Attrs pcommon.Map
	IDs   AttributeOwnerIDs
	Scope string // e.g. "resource", "scope", "span", "event", "link"
}

// AttrsCanonical returns the canonical "key=value|key=value|..." form of
// an attribute set, with keys in lexicographic order. Two attribute maps
// with the same keys and values (regardless of insertion order) produce
// the same string; any change in keys, values, or value types changes
// the string.
//
// Why this exists: the per-datapoint stream identity (i.e. "which
// (metric, attribute combination) does this sample belong to") used to
// be computed at query time via `string_agg(key || '=' || value order
// by key)` in every CTE that grouped by stream. Materialising the
// canonical form at ingest makes those queries an equality compare on
// a fixed varchar column.
//
// An earlier iteration sha1'd this string into a 20-byte digest under
// the (uninterrogated) assumption that a fixed-width column would
// meaningfully outperform a varchar one for GROUP BY. Reversed: storage
// savings on a local tool with bounded retention are negligible, the
// asymmetry between backend-hex and frontend-canonical keys caused real
// bugs in the chart-grouping code, and human-readability when poking
// the DB directly is worth preserving.
//
// The empty-attrs case returns "". Callers that want to distinguish
// "no attributes" from "attributes that happen to canonicalise to an
// empty string" should also check Len()==0; in practice we treat the
// canonical column on datapoints as nullable and store NULL when the
// attribute set is empty.
//
// Value formatting matches util.ValueToStringAndType so the canonical
// form is byte-identical to what gets written into attributes.value --
// a key/value collision between two streams would require two different
// attribute sets to produce identical concatenated strings, which can't
// happen because '=' and '|' are not used as field separators in the
// stored attribute values themselves: the key is fixed by position, so
// even a value containing '|' is unambiguously bound to its key.
func AttrsCanonical(attrs pcommon.Map) string {
	if attrs.Len() == 0 {
		return ""
	}
	keys := make([]string, 0, attrs.Len())
	for k := range attrs.All() {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Pre-size the buffer to roughly the expected length: one '=', one
	// '|' per pair plus key+value lengths. A single grow is fine even
	// when the estimate is off; this avoids many small reallocations
	// for the common ~5-attr case.
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('|')
		}
		v, _ := attrs.Get(k)
		valueStr, _ := util.ValueToStringAndType(v)
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(valueStr)
	}
	return b.String()
}

// IngestAttributes appends attribute rows for a batch of (attrs, ids) pairs.
// Each item's map is iterated and rows are appended with that item's IDs, so
// you can mix e.g. span attrs (SpanID only) and event attrs (EventID + SpanID) in one call.
func IngestAttributes(appender *duckdb.Appender, items []AttributeBatchItem) error {
	for _, item := range items {
		if item.Attrs.Len() == 0 {
			continue
		}
		ids := item.IDs
		scope := item.Scope
		for k, v := range item.Attrs.All() {
			valueStr, attrType := util.ValueToStringAndType(v)
			if err := appender.AppendRow(
				ids.SpanID,
				ids.EventID,
				ids.LinkID,
				ids.LogID,
				ids.MetricIngestID,
				ids.DataPointID,
				ids.ExemplarID,
				scope,
				k,
				valueStr,
				attrType,
			); err != nil {
				return fmt.Errorf("IngestAttributes: %w: %w", ErrIngestInternal, err)
			}
		}
	}
	return nil
}
