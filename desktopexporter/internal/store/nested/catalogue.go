package nested

import (
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/searchindex"
	"github.com/duckdb/duckdb-go/v2"
)

// A received metric owns its descriptor and points. RootKey groups received
// records under their logical metric without replacing earlier descriptors.
func catalogueMetric(b *searchindex.Batch, metric, record duckdb.UUID, metadata []byte, resource, scope map[string]any, name, description, unit string) error {
	r, s, err := b.Contexts(resource, scope)
	if err != nil {
		return err
	}
	key := ingest.FormatUUID(record)
	o := searchindex.Object{Ref: "metric:record:" + key, Signal: "metric", Owner: "metric", RootKey: ingest.FormatUUID(metric), RecordKey: key, ResourceRef: r, ScopeRef: s}
	if err := o.AddNative("metadata", metadata); err != nil {
		return err
	}
	if err := o.AddStrings(map[string]any{"name": name, "description": description, "unit": unit}); err != nil {
		return err
	}
	b.Add(o)
	return nil
}

func cataloguePoint(b *searchindex.Batch, metric, record, point duckdb.UUID, attrs []byte, resource, scope map[string]any) error {
	r, s, err := b.Contexts(resource, scope)
	if err != nil {
		return err
	}
	key := ingest.FormatUUID(point)
	o := searchindex.Object{Ref: "metric:datapoint:" + key, Signal: "metric", Owner: "datapoint", RootKey: ingest.FormatUUID(metric), RecordKey: key, ParentRef: "metric:record:" + ingest.FormatUUID(record), ResourceRef: r, ScopeRef: s}
	if err := o.AddNative("attributes", attrs); err != nil {
		return err
	}
	b.Add(o)
	return nil
}
