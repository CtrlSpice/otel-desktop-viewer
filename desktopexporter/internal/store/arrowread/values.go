// Package arrowread projects native query results into browser-readable Arrow.
// Static columns retain their buffers. Dynamic telemetry values become standard
// Arrow scalar, Map, List and DenseUnion arrays, without a JSON intermediate.
package arrowread

import (
	"context"
	"fmt"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	pv "github.com/apache/arrow-go/v18/parquet/variant"
)

// These codes are the Arrow union's type IDs, not an application value wrapper.
const (
	nullKind = iota
	boolKind
	intKind
	doubleKind
	stringKind
	binaryKind
	mapKind
	listKind
	kindCount
)

type valueShape struct {
	seen  [kindCount]bool
	items [2]*valueShape // Map values and List elements have independent shapes.
}

func valueKind(v pv.Value) (int, error) {
	switch v.Type() {
	case pv.Null:
		return nullKind, nil
	case pv.Bool:
		return boolKind, nil
	case pv.Int8, pv.Int16, pv.Int32, pv.Int64:
		return intKind, nil
	case pv.Double:
		return doubleKind, nil
	case pv.String:
		return stringKind, nil
	case pv.Binary:
		return binaryKind, nil
	case pv.Object:
		return mapKind, nil
	case pv.Array:
		return listKind, nil
	default:
		return 0, fmt.Errorf("unsupported telemetry Variant type %v", v.Type())
	}
}

func (s *valueShape) observe(ctx context.Context, v pv.Value) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	kind, err := valueKind(v)
	if err != nil {
		return err
	}
	s.seen[kind] = true
	if kind < mapKind {
		return nil
	}
	index := kind - mapKind
	if s.items[index] == nil {
		s.items[index] = &valueShape{}
	}
	child := s.items[index]
	if kind == mapKind {
		for _, item := range v.Value().(pv.ObjectValue).Values() {
			if err := child.observe(ctx, item); err != nil {
				return err
			}
		}
	} else {
		for item := range v.Value().(pv.ArrayValue).Values() {
			if err := child.observe(ctx, item); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *valueShape) dataType() arrow.DataType {
	var fields []arrow.Field
	var codes []arrow.UnionTypeCode
	names := [...]string{"null", "bool", "int64", "float64", "string", "bytes", "map", "list"}
	for kind, seen := range s.seen {
		if !seen {
			continue
		}
		var dt arrow.DataType
		switch kind {
		case nullKind:
			dt = arrow.Null
		case boolKind:
			dt = arrow.FixedWidthTypes.Boolean
		case intKind:
			dt = arrow.PrimitiveTypes.Int64
		case doubleKind:
			dt = arrow.PrimitiveTypes.Float64
		case stringKind:
			dt = arrow.BinaryTypes.String
		case binaryKind:
			dt = arrow.BinaryTypes.Binary
		case mapKind:
			dt = arrow.MapOf(arrow.BinaryTypes.String, s.items[0].dataType())
		case listKind:
			dt = arrow.ListOf(s.items[1].dataType())
		}
		fields = append(fields, arrow.Field{Name: names[kind], Type: dt, Nullable: true})
		codes = append(codes, arrow.UnionTypeCode(kind))
	}
	if len(fields) == 0 {
		return arrow.Null
	}
	if len(fields) == 1 {
		return fields[0].Type
	}
	return arrow.DenseUnionOf(fields, codes)
}

func appendValue(ctx context.Context, b array.Builder, v pv.Value) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	kind, err := valueKind(v)
	if err != nil {
		return err
	}
	if union, ok := b.(*array.DenseUnionBuilder); ok {
		code := arrow.UnionTypeCode(kind)
		union.Append(code)
		b = union.Child(union.Type().(*arrow.DenseUnionType).ChildIDs()[code])
	}
	switch kind {
	case nullKind:
		b.AppendNull()
	case boolKind:
		b.(*array.BooleanBuilder).Append(v.Value().(bool))
	case intKind:
		var n int64
		switch x := v.Value().(type) {
		case int8:
			n = int64(x)
		case int16:
			n = int64(x)
		case int32:
			n = int64(x)
		case int64:
			n = x
		}
		b.(*array.Int64Builder).Append(n)
	case doubleKind:
		b.(*array.Float64Builder).Append(v.Value().(float64))
	case stringKind:
		b.(*array.StringBuilder).Append(v.Value().(string))
	case binaryKind:
		b.(*array.BinaryBuilder).Append(v.Value().([]byte))
	case mapKind:
		m := b.(*array.MapBuilder)
		m.Append(true)
		for key, child := range v.Value().(pv.ObjectValue).Values() {
			m.KeyBuilder().(*array.StringBuilder).Append(key)
			if err := appendValue(ctx, m.ItemBuilder(), child); err != nil {
				return err
			}
		}
	case listKind:
		l := b.(*array.ListBuilder)
		l.Append(true)
		for child := range v.Value().(pv.ArrayValue).Values() {
			if err := appendValue(ctx, l.ValueBuilder(), child); err != nil {
				return err
			}
		}
	}
	return nil
}

// projection is a schema tree. Only paths containing Variant need new arrays.
// It collects types, not values or a per-row object graph.
type projection struct {
	typeIn   arrow.DataType
	shape    *valueShape
	children []*projection
	changed  bool
}

func newProjection(f arrow.Field) *projection {
	dt := f.Type
	p := &projection{typeIn: dt}
	name, _ := f.Metadata.GetValue("ARROW:extension:name")
	if ext, ok := dt.(arrow.ExtensionType); ok {
		name = ext.ExtensionName()
	}
	if name == "arrow.parquet.variant" {
		p.shape, p.changed = &valueShape{}, true
		return p
	}
	if nested, ok := dt.(arrow.NestedType); ok {
		for _, field := range nested.Fields() {
			child := newProjection(field)
			p.children = append(p.children, child)
			p.changed = p.changed || child.changed
		}
	}
	return p
}

func (p *projection) observe(ctx context.Context, a arrow.Array) error {
	if !p.changed {
		return nil
	}
	if p.shape != nil {
		v := a
		for i := 0; i < v.Len(); i++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			if v.IsNull(i) {
				p.shape.seen[nullKind] = true
				continue
			}
			value, err := variantValue(v, i)
			if err != nil {
				return err
			}
			if err := p.shape.observe(ctx, value); err != nil {
				return err
			}
		}
		return nil
	}
	for i, child := range a.Data().Children() {
		ca := array.MakeFromData(child)
		err := p.children[i].observe(ctx, ca)
		ca.Release()
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *projection) dataType() arrow.DataType {
	if !p.changed {
		return p.typeIn
	}
	if p.shape != nil {
		return p.shape.dataType()
	}
	fields := p.typeIn.(arrow.NestedType).Fields()
	for i := range fields {
		fields[i] = p.children[i].field(fields[i])
	}
	switch dt := p.typeIn.(type) {
	case *arrow.StructType:
		return arrow.StructOf(fields...)
	case *arrow.ListType:
		return arrow.ListOfField(fields[0])
	case *arrow.LargeListType:
		return arrow.LargeListOfField(fields[0])
	case *arrow.FixedSizeListType:
		return arrow.FixedSizeListOfField(dt.Len(), fields[0])
	case *arrow.MapType:
		entries := fields[0].Type.(*arrow.StructType)
		out := arrow.MapOf(entries.Field(0).Type, entries.Field(1).Type)
		out.SetItemNullable(entries.Field(1).Nullable)
		out.KeysSorted = dt.KeysSorted
		return out
	case *arrow.DenseUnionType:
		return arrow.DenseUnionOf(fields, dt.TypeCodes())
	case *arrow.SparseUnionType:
		return arrow.SparseUnionOf(fields, dt.TypeCodes())
	default:
		panic(fmt.Sprintf("unsupported nested Variant projection: %s", p.typeIn))
	}
}

func (p *projection) field(f arrow.Field) arrow.Field {
	f.Type = p.dataType()
	if p.shape != nil {
		metadata := f.Metadata.ToMap()
		delete(metadata, "ARROW:extension:name")
		delete(metadata, "ARROW:extension:metadata")
		f.Metadata = arrow.MetadataFrom(metadata)
	}
	return f
}

func (p *projection) apply(ctx context.Context, alloc memory.Allocator, a arrow.Array) (arrow.Array, error) {
	if !p.changed {
		a.Retain()
		return a, nil
	}
	if p.shape != nil {
		b := array.NewBuilder(alloc, p.dataType())
		defer b.Release()
		b.Reserve(a.Len())
		v := a
		for i := 0; i < v.Len(); i++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if v.IsNull(i) {
				if u, ok := b.(*array.DenseUnionBuilder); ok {
					u.Append(nullKind)
					u.Child(u.Type().(*arrow.DenseUnionType).ChildIDs()[nullKind]).AppendNull()
				} else {
					b.AppendNull()
				}
				continue
			}
			value, err := variantValue(v, i)
			if err != nil {
				return nil, err
			}
			if err := appendValue(ctx, b, value); err != nil {
				return nil, err
			}
		}
		return b.NewArray(), nil
	}
	data := a.Data()
	children := make([]arrow.ArrayData, len(p.children))
	defer func() {
		for _, c := range children {
			if c != nil {
				c.Release()
			}
		}
	}()
	for i, child := range data.Children() {
		ca := array.MakeFromData(child)
		out, err := p.children[i].apply(ctx, alloc, ca)
		ca.Release()
		if err != nil {
			return nil, err
		}
		children[i] = out.Data()
		children[i].Retain()
		out.Release()
	}
	out := array.NewData(p.dataType(), a.Len(), data.Buffers(), children, a.NullN(), data.Offset())
	defer out.Release()
	return array.MakeFromData(out), nil
}

func variantValue(a arrow.Array, i int) (pv.Value, error) {
	if ext, ok := a.(array.ExtensionArray); ok {
		a = ext.Storage()
	}
	s := a.(*array.Struct)
	return pv.New(s.Field(0).(*array.Binary).Value(i), s.Field(1).(*array.Binary).Value(i))
}
