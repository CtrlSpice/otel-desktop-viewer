package store

import (
	"github.com/google/uuid"
	"github.com/marcboeker/go-duckdb/v2"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// AttributeOwnerIDs specifies which entity owns the attributes.
// Populate only the IDs that apply; leave others nil. Used by IngestAttributes
// to build the correct row (SpanID VARCHAR, EventID UUID, ..., Key, Value, Type).
type AttributeOwnerIDs struct {
	SpanID      string
	EventID     *uuid.UUID
	LinkID      *uuid.UUID
	LogID       *uuid.UUID
	MetricID    *uuid.UUID
	DataPointID *uuid.UUID
	ExemplarID  *uuid.UUID
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
			valueStr, attrType := ValueToStringAndType(v)
			if err := appender.AppendRow(
				ids.SpanID,      // SpanID VARCHAR
				ids.EventID,     // EventID UUID
				ids.LinkID,      // LinkID UUID
				ids.LogID,       // LogID UUID
				ids.MetricID,    // MetricID UUID
				ids.DataPointID, // DataPointID UUID
				ids.ExemplarID,  // ExemplarID UUID
				scope,          // Scope VARCHAR
				k,              // Key VARCHAR
				valueStr,       // Value VARCHAR
				attrType,       // Type attr_type
			); err != nil {
				return err
			}
		}
	}
	return nil
}
