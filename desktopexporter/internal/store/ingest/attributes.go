package ingest

import (
	"fmt"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
	"github.com/duckdb/duckdb-go/v2"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// AttributeOwnerIDs specifies which entity owns the attributes.
// Populate only the IDs that apply; leave others nil.
type AttributeOwnerIDs struct {
	SpanID      *duckdb.UUID
	EventID     *duckdb.UUID
	LinkID      *duckdb.UUID
	LogID       *duckdb.UUID
	MetricID    *duckdb.UUID
	DataPointID *duckdb.UUID
	ExemplarID  *duckdb.UUID
}

// AttributeBatchItem pairs an attribute map with the entity IDs and scope that own it.
// Scope is the semantic owner: "resource", "scope", "span", "event", "link" (or "log", "metric", etc. for other signals).
type AttributeBatchItem struct {
	Attrs pcommon.Map
	IDs   AttributeOwnerIDs
	Scope string // e.g. "resource", "scope", "span", "event", "link"
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
				ids.MetricID,
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
