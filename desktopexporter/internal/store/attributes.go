package store

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

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
			valueStr, attrType := convertAttributeValue(v)
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
				return err
			}
		}
	}
	return nil
}

// convertAttributeValue converts a pcommon.Value to (value string, type string) for normalized attributes table.
// Reuses type detection logic from attributes package, adapted for normalized schema.
func convertAttributeValue(v pcommon.Value) (valueStr string, attrType string) {
	switch v.Type() {
	case pcommon.ValueTypeStr:
		return v.Str(), "string"
	case pcommon.ValueTypeInt:
		return strconv.FormatInt(v.Int(), 10), "int64"
	case pcommon.ValueTypeDouble:
		return strconv.FormatFloat(v.Double(), 'f', -1, 64), "float64"
	case pcommon.ValueTypeBool:
		return strconv.FormatBool(v.Bool()), "bool"
	case pcommon.ValueTypeBytes:
		// Convert bytes to hex string
		bytes := v.Bytes()
		return hex.EncodeToString(bytes.AsRaw()), "string"
	case pcommon.ValueTypeSlice:
		return convertSliceValue(v)
	default:
		// Fallback to string representation
		return fmt.Sprintf("%v", v.AsRaw()), "string"
	}
}

// convertSliceValue converts a pcommon.Value slice to (value string, attr_type string).
// Uses JSON array format for the value; attr_type is derived from the first element (or "string[]" for empty).
func convertSliceValue(v pcommon.Value) (valueStr string, attrType string) {
	slice := v.Slice()
	if slice.Len() == 0 {
		return "[]", "string[]"
	}

	firstItem := slice.At(0)
	switch firstItem.Type() {
	case pcommon.ValueTypeStr:
		attrType = "string[]"
	case pcommon.ValueTypeInt:
		attrType = "int64[]"
	case pcommon.ValueTypeDouble:
		attrType = "float64[]"
	case pcommon.ValueTypeBool:
		attrType = "boolean[]"
	default:
		attrType = "string[]"
	}

	var parts []string
	for i := 0; i < slice.Len(); i++ {
		item := slice.At(i)
		switch item.Type() {
		case pcommon.ValueTypeStr:
			parts = append(parts, `"`+strings.ReplaceAll(item.Str(), `"`, `\"`)+`"`)
		case pcommon.ValueTypeInt:
			parts = append(parts, strconv.FormatInt(item.Int(), 10))
		case pcommon.ValueTypeDouble:
			parts = append(parts, strconv.FormatFloat(item.Double(), 'f', -1, 64))
		case pcommon.ValueTypeBool:
			parts = append(parts, strconv.FormatBool(item.Bool()))
		default:
			parts = append(parts, fmt.Sprintf("%v", item.AsRaw()))
		}
	}
	return "[" + strings.Join(parts, ",") + "]", attrType
}
