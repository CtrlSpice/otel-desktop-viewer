package arrowread

import (
	"bytes"
	"database/sql"
	"math"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/arrowbridge"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	pv "github.com/apache/arrow-go/v18/parquet/variant"
	"github.com/duckdb/duckdb-go/v2"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func nativeTestDB(t *testing.T) *sql.DB {
	t.Helper()
	connector, err := duckdb.NewConnector("", nil)
	require.NoError(t, err)
	db := sql.OpenDB(connector)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	conn, err := connector.Connect(t.Context())
	require.NoError(t, err)
	require.NoError(t, arrowbridge.Load(t.Context(), conn))
	require.NoError(t, conn.Close())
	return db
}

func variantValue(a arrow.Array, i int) (pv.Value, error) {
	if ext, ok := a.(array.ExtensionArray); ok {
		a = ext.Storage()
	}
	s := a.(*array.Struct)
	return pv.New(s.Field(0).(*array.Binary).Value(i), s.Field(1).(*array.Binary).Value(i))
}

func TestNativeVariantResponse(t *testing.T) {
	db := nativeTestDB(t)
	v := pcommon.NewValueMap()
	v.Map().PutInt("__proto__", 9007199254740993)
	v.Map().PutEmpty("null")
	l := v.Map().PutEmptySlice("items")
	l.AppendEmpty().SetInt(1)
	l.AppendEmpty().SetDouble(1)
	l.AppendEmpty().SetDouble(math.Float64frombits(0x7ff8000000000042))
	l.AppendEmpty().SetDouble(math.Copysign(0, -1))
	l.AppendEmpty().SetEmptyMap()
	l.AppendEmpty().SetEmptySlice()
	l.AppendEmpty().SetEmptyBytes().FromRaw([]byte{0, 255})
	encoded, err := ingest.ValueBytes(v)
	require.NoError(t, err)
	r, err := Read(t.Context(), db, `select i::bigint as id,
		[{'timestamp': 18446744073709551615::ubigint,
		'attributes': case when i<9000 then variant_bytes_to_variant($1::blob)
		else 'last batch'::variant end}] as exemplars
		from range(9001) t(i) order by i`, encoded)
	require.NoError(t, err)
	defer r.Close()
	require.Equal(t, int64(9001), r.NumRows())
	var buf bytes.Buffer
	require.NoError(t, r.Write(t.Context(), &buf))
	reader, err := ipc.NewReader(&buf)
	require.NoError(t, err)
	defer reader.Release()
	var rows int64
	var batches int
	for reader.Next() {
		record := reader.RecordBatch()
		batches++
		rows += record.NumRows()
		ex := record.Column(1).(*array.List).ListValues().(*array.Struct)
		require.Equal(t, uint64(math.MaxUint64), ex.Field(0).(*array.Uint64).Value(0))
		field := ex.DataType().(*arrow.StructType).Field(1)
		name, ok := field.Metadata.GetValue("ARROW:extension:name")
		require.True(t, ok)
		require.Equal(t, "arrow.parquet.variant", name)
		// A later string uses the same two binary fields as the earlier maps.
		storage := field.Type.(*arrow.StructType)
		require.Equal(t, arrow.BINARY, storage.Field(0).Type.ID())
		require.Equal(t, arrow.BINARY, storage.Field(1).Type.ID())
		for row := range ex.Len() {
			value, err := variantValue(ex.Field(1), row)
			require.NoError(t, err)
			if record.Column(0).(*array.Int64).Value(row) == 9000 {
				require.Equal(t, pv.String, value.Type())
				require.Equal(t, "last batch", value.Value())
				continue
			}
			object := value.Value().(pv.ObjectValue)
			integer, err := object.ValueByKey("__proto__")
			require.NoError(t, err)
			require.Equal(t, int64(9007199254740993), integer.Value.Value())
			null, err := object.ValueByKey("null")
			require.NoError(t, err)
			require.Equal(t, pv.Null, null.Value.Type())
			list, err := object.ValueByKey("items")
			require.NoError(t, err)
			items := list.Value.Value().(pv.ArrayValue)
			require.EqualValues(t, 7, items.Len())
			first, err := items.Value(0)
			require.NoError(t, err)
			require.Contains(t, []pv.Type{pv.Int8, pv.Int16, pv.Int32, pv.Int64}, first.Type())
			require.EqualValues(t, 1, first.Value())
			second, err := items.Value(1)
			require.NoError(t, err)
			require.Equal(t, pv.Double, second.Type())
			require.Equal(t, float64(1), second.Value())
			nan, err := items.Value(2)
			require.NoError(t, err)
			require.Equal(t, uint64(0x7ff8000000000042), math.Float64bits(nan.Value().(float64)))
			zero, err := items.Value(3)
			require.NoError(t, err)
			require.True(t, math.Signbit(zero.Value().(float64)))
			emptyMap, err := items.Value(4)
			require.NoError(t, err)
			require.Zero(t, emptyMap.Value().(pv.ObjectValue).NumElements())
			emptyArray, err := items.Value(5)
			require.NoError(t, err)
			require.Zero(t, emptyArray.Value().(pv.ArrayValue).Len())
			binary, err := items.Value(6)
			require.NoError(t, err)
			require.Equal(t, []byte{0, 255}, binary.Value())
		}
	}
	require.NoError(t, reader.Err())
	require.Equal(t, int64(9001), rows)
	require.Greater(t, batches, 1)
}

func TestNativeVariantDeepValues(t *testing.T) {
	db := nativeTestDB(t)
	for _, shape := range []string{"arrays", "maps", "mixed"} {
		t.Run(shape, func(t *testing.T) {
			value := pcommon.NewValueEmpty()
			child := value
			const depth = 200
			for level := range depth {
				if shape == "maps" || shape == "mixed" && level%2 == 0 {
					child = child.SetEmptyMap().PutEmpty("child")
				} else {
					items := child.SetEmptySlice()
					items.AppendEmpty().SetBool(false)
					child = items.AppendEmpty()
				}
			}
			child.SetInt(9007199254740993)
			encoded, err := ingest.ValueBytes(value)
			require.NoError(t, err)
			result, err := Read(t.Context(), db, `select [{'attributes': variant_bytes_to_variant($1::blob)}] as items`, encoded)
			require.NoError(t, err)
			defer result.Close()
			require.Len(t, result.records, 1)
			beforeItems := result.records[0].Column(0).(*array.List).ListValues().(*array.Struct)
			before, err := variantValue(beforeItems.Field(0), 0)
			require.NoError(t, err)
			var output bytes.Buffer
			require.NoError(t, result.Write(t.Context(), &output))
			reader, err := ipc.NewReader(&output)
			require.NoError(t, err)
			defer reader.Release()
			require.True(t, reader.Next())
			afterItems := reader.RecordBatch().Column(0).(*array.List).ListValues().(*array.Struct)
			after, err := variantValue(afterItems.Field(0), 0)
			require.NoError(t, err)
			require.Equal(t, before.Metadata().Bytes(), after.Metadata().Bytes())
			require.Equal(t, before.Bytes(), after.Bytes())
			for level := range depth {
				if shape == "maps" || shape == "mixed" && level%2 == 0 {
					field, err := after.Value().(pv.ObjectValue).ValueByKey("child")
					require.NoError(t, err)
					after = field.Value
				} else {
					items := after.Value().(pv.ArrayValue)
					require.EqualValues(t, 2, items.Len())
					flag, err := items.Value(0)
					require.NoError(t, err)
					require.Equal(t, false, flag.Value())
					after, err = items.Value(1)
					require.NoError(t, err)
				}
			}
			require.Equal(t, int64(9007199254740993), after.Value())
			require.False(t, reader.Next())
			require.NoError(t, reader.Err())
		})
	}
}
