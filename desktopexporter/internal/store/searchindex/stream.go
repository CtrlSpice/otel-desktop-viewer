package searchindex

import (
	"context"
	"database/sql/driver"
	"fmt"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/arrowbridge"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"sort"
	"strings"
)

// Batch-local dictionaries are populated as native records are encoded. Only
// unique dictionary rows and compact integer associations survive each Add.
// Go-owned slices allow an abandoned ingestion attempt to release everything
// without requiring native Arrow allocations to outlive the write call.
type streamCatalogue struct {
	owners, paths, values       [][]string
	links                       [][3]uint32
	ownerIDs, pathIDs, valueIDs map[string]uint32
}

func intern(rows *[][]string, ids map[string]uint32, key string, row func() []string) uint32 {
	if id, ok := ids[key]; ok {
		return id
	}
	id := uint32(len(*rows))
	ids[key] = id
	*rows = append(*rows, row())
	return id
}

func (b *Batch) addStream(o Object) {
	if b.stream == nil {
		b.stream = &streamCatalogue{ownerIDs: map[string]uint32{}, pathIDs: map[string]uint32{}, valueIDs: map[string]uint32{}}
	}
	c := b.stream
	if _, ok := c.ownerIDs[o.Ref]; ok {
		return
	}
	ownerID := uint32(len(c.owners))
	c.ownerIDs[o.Ref] = ownerID
	identities := make([]string, 0, len(o.Entries))
	seen := map[[2]uint32]bool{}
	for _, e := range o.Entries {
		path := jsonText(e.Path)
		pkey := digest(o.Owner, path, e.Kind)
		pi := intern(&c.paths, c.pathIDs, pkey, func() []string {
			key := e.KeyText
			if len(e.Path) > 1 {
				relative := e.Path[1:]
				if e.Path[0] == "body" {
					relative = e.Path
				}
				if len(relative) == 1 {
					if s, ok := relative[0].(string); ok {
						key = s
					} else {
						key = jsonText(relative)
					}
				} else {
					key = jsonText(relative)
				}
			} else if len(e.Path) > 0 {
				key = fmt.Sprint(e.Path[0])
			}
			scope := o.Owner
			if len(e.Path) > 0 && e.Path[0] == "metadata" {
				scope = "metadata"
			}
			return []string{pkey, o.Owner, path, e.Kind, key, e.KeyText, scope, fmt.Sprint(e.Attribute)}
		})
		value := ""
		if e.NativeText != nil {
			value = *e.NativeText
		}
		vkey := e.NativeIdentity
		if vkey == "" {
			vkey = digest(e.Kind, value)
		}
		vi := intern(&c.values, c.valueIDs, vkey, func() []string {
			if e.Container {
				value = ""
			}
			return []string{vkey, e.Kind, value}
		})
		pair := [2]uint32{pi, vi}
		if !seen[pair] {
			c.links = append(c.links, [3]uint32{ownerID, pi, vi})
			seen[pair] = true
		}
		identities = append(identities, pkey+vkey)
	}
	sort.Strings(identities)
	fingerprint := digest(o.Signal, o.Owner, o.RootKey, o.RecordKey, o.ParentRef, o.ResourceRef, o.ScopeRef, strings.Join(identities, ""))
	c.owners = append(c.owners, []string{o.Ref, o.Signal, o.Owner, o.RootKey, o.RecordKey, o.ParentRef, o.ResourceRef, o.ScopeRef, fingerprint})
}

func dictionaryColumns(names []string, rows [][]string) *array.RecordBuilder {
	fields := make([]arrow.Field, len(names)+1)
	for i, name := range names {
		fields[i] = arrow.Field{Name: name, Type: arrow.BinaryTypes.String}
	}
	fields[len(names)] = arrow.Field{Name: "local_id", Type: arrow.PrimitiveTypes.Uint32}
	b := array.NewRecordBuilder(arrowbridge.Allocator(), arrow.NewSchema(fields, nil))
	for id, row := range rows {
		for i, v := range row {
			b.Field(i).(*array.StringBuilder).Append(v)
		}
		b.Field(len(names)).(*array.Uint32Builder).Append(uint32(id))
	}
	return b
}

func streamRegister(ctx context.Context, conn driver.Conn, name string, b *array.RecordBuilder) (func(), error) {
	record := b.NewRecordBatch()
	defer record.Release()
	fields := make([]string, record.NumCols())
	for i, f := range record.Schema().Fields() {
		kind := "varchar"
		if f.Type.ID() == arrow.UINT32 {
			kind = "uinteger"
		}
		if f.Type.ID() == arrow.BINARY {
			kind = "blob"
		}
		fields[i] = f.Name + " " + kind
	}
	exec := connection{conn}
	if _, err := exec.ExecContext(ctx, "create temp table "+name+"("+strings.Join(fields, ",")+")"); err != nil {
		return nil, err
	}
	if err := arrowbridge.Insert(ctx, conn, "insert into "+name+" select * from appended_data", record); err != nil {
		return nil, err
	}
	return func() { _, _ = exec.ExecContext(ctx, "drop table "+name) }, nil
}

func (b *Batch) writeStream(ctx context.Context, conn driver.Conn) error {
	c := b.stream
	if c == nil {
		return nil
	}
	defer func() { b.stream = nil; b.contexts = nil }()
	owners := dictionaryColumns([]string{"ref", "signal", "owner", "root_key", "record_key", "parent_ref", "resource_ref", "scope_ref", "fingerprint"}, c.owners)
	defer owners.Release()
	paths := dictionaryColumns([]string{"identity", "owner", "path", "kind", "key", "key_text", "attribute_scope", "is_attribute"}, c.paths)
	defer paths.Release()
	values := dictionaryColumns([]string{"identity", "kind", "value"}, c.values)
	defer values.Release()
	links := array.NewRecordBuilder(arrowbridge.Allocator(), arrow.NewSchema([]arrow.Field{{Name: "owner_id", Type: arrow.PrimitiveTypes.Uint32}, {Name: "path_id", Type: arrow.PrimitiveTypes.Uint32}, {Name: "value_id", Type: arrow.PrimitiveTypes.Uint32}}, nil))
	defer links.Release()
	for _, link := range c.links {
		for i, v := range link {
			links.Field(i).(*array.Uint32Builder).Append(v)
		}
	}
	for i, columns := range []*array.RecordBuilder{owners, paths, values, links} {
		release, err := streamRegister(ctx, conn, []string{"stream_owners", "stream_paths", "stream_values", "stream_links"}[i], columns)
		if err != nil {
			return err
		}
		defer release()
	}
	_, err := (connection{conn}).ExecContext(ctx, `with
 added_owners as materialized(insert into search_owners(ref,signal,owner,root_key,record_key,parent_ref,resource_ref,scope_ref,fingerprint)
 select ref,signal,owner,root_key,record_key,nullif(parent_ref,''),nullif(resource_ref,''),nullif(scope_ref,''),fingerprint from stream_owners on conflict(ref) do nothing returning id,ref),
 added_paths as materialized(insert into search_paths(identity,owner,path,kind,key,key_text,attribute_scope,is_attribute)
 select identity,owner,path::json,kind,key,key_text,attribute_scope,is_attribute::boolean from stream_paths i where not exists(select 1 from search_paths p where p.identity=i.identity) returning id),
 path_ids as materialized(select p.id,i.local_id from search_paths p join stream_paths i using(identity) cross join(select count(*) n from added_paths) done where done.n>=0),
 added_values as materialized(insert into search_values(identity,kind,value)
 select identity,kind,value from stream_values i where not exists(select 1 from search_values v where v.identity=i.identity) returning id),
 value_ids as materialized(select v.id,i.local_id from search_values v join stream_values i using(identity) cross join(select count(*) n from added_values) done where done.n>=0)
 insert into search_postings select o.id,p.id,v.id from stream_links l
 join stream_owners incoming on incoming.local_id=l.owner_id join added_owners o using(ref)
 join path_ids p on p.local_id=l.path_id join value_ids v on v.local_id=l.value_id`)
	return err
}
