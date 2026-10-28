package searchindex

import (
	"context"
	"database/sql/driver"
	"fmt"

	pv "github.com/apache/arrow-go/v18/parquet/variant"
	"github.com/duckdb/duckdb-go/v2"
	"github.com/google/uuid"
)

// Batch contains only rows selected by one native append pass. A failed pass
// discards it; Write runs after the final appender close in that transaction.
type Batch struct {
	objects  []Object
	contexts map[string]bool
}

func (o *Object) AddNative(root string, raw []byte) error {
	metadata, err := pv.NewMetadata(raw)
	if err != nil {
		return err
	}
	value, err := pv.NewWithMetadata(metadata, raw[metadata.SizeBytes():])
	if err != nil {
		return err
	}
	_, _, err = o.walkNative(value, []any{root}, "")
	return err
}

// NativeMatches compares an incoming native appender cell with the native
// metadata already returned by metric identity resolution. It adds no query.
func NativeMatches(raw, metadata, value []byte) (bool, error) {
	var incoming, stored Object
	v, err := pv.New(metadata, value)
	if err != nil {
		return false, err
	}
	_, storedID, err := stored.walkNative(v, []any{"metadata"}, "")
	if err != nil {
		return false, err
	}
	m, err := pv.NewMetadata(raw)
	if err != nil {
		return false, err
	}
	v, err = pv.NewWithMetadata(m, raw[m.SizeBytes():])
	if err != nil {
		return false, err
	}
	_, incomingID, err := incoming.walkNative(v, []any{"metadata"}, "")
	return incomingID == storedID, err
}

func (b *Batch) Contexts(resource, scope map[string]any) (string, string, error) {
	if b.contexts == nil {
		b.contexts = map[string]bool{}
	}
	refs := make([]string, 2)
	for i, input := range []map[string]any{resource, scope} {
		owner := "resource"
		if i == 1 {
			owner = "scope"
		}
		id, ok := input["id"].(duckdb.UUID)
		if !ok {
			return "", "", fmt.Errorf("invalid native %s identity", owner)
		}
		key := uuid.UUID(id).String()
		refs[i] = owner + ":" + key
		if b.contexts[refs[i]] {
			continue
		}
		o := Object{Ref: refs[i], Owner: owner, RecordKey: key}
		if err := o.AddNative("attributes", input["attributes"].([]byte)); err != nil {
			return "", "", err
		}
		fields := map[string]any{"schemaUrl": input["schema_url"]}
		if owner == "scope" {
			fields["name"] = input["name"]
			fields["version"] = input["version"]
		}
		if err := o.AddStrings(fields); err != nil {
			return "", "", err
		}
		b.objects = append(b.objects, o)
		b.contexts[refs[i]] = true
	}
	return refs[0], refs[1], nil
}

func (b *Batch) Add(o Object) { o.NewOwner = true; b.objects = append(b.objects, o) }
func (b *Batch) Write(ctx context.Context, conn driver.Conn) error {
	return Write(ctx, connection{conn}, b.objects)
}

func (b *Batch) Metric(id duckdb.UUID, metadata []byte, resource, scope map[string]any, name, description, unit string) error {
	root := uuid.UUID(id).String()
	r, s, err := b.Contexts(resource, scope)
	if err != nil {
		return err
	}
	o := Object{Ref: "metric:metric:" + root, Signal: "metric", Owner: "metric", RootKey: root, RecordKey: root, ResourceRef: r, ScopeRef: s}
	if err := o.AddNative("metadata", metadata); err != nil {
		return err
	}
	if err := o.AddStrings(map[string]any{"name": name, "description": description, "unit": unit}); err != nil {
		return err
	}
	b.objects = append(b.objects, o)
	return nil
}

func (b *Batch) Datapoint(metric, point duckdb.UUID, attrs []byte, resource, scope map[string]any) error {
	root, key := uuid.UUID(metric).String(), uuid.UUID(point).String()
	r, s, err := b.Contexts(resource, scope)
	if err != nil {
		return err
	}
	o := Object{Ref: "metric:datapoint:" + key, Signal: "metric", Owner: "datapoint", RootKey: root, RecordKey: key, ParentRef: "metric:metric:" + root, ResourceRef: r, ScopeRef: s}
	if err := o.AddNative("attributes", attrs); err != nil {
		return err
	}
	b.Add(o)
	return nil
}

func (b *Batch) Exemplar(metric, point, exemplar duckdb.UUID, attrs []byte) error {
	root, key := uuid.UUID(metric).String(), uuid.UUID(exemplar).String()
	o := Object{Ref: "metric:exemplar:" + key, Signal: "metric", Owner: "exemplar", RootKey: root, RecordKey: key, ParentRef: "metric:datapoint:" + uuid.UUID(point).String()}
	if err := o.AddNative("attributes", attrs); err != nil {
		return err
	}
	b.Add(o)
	return nil
}
