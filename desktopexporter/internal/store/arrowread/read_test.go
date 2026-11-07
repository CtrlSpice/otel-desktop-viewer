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
	"github.com/duckdb/duckdb-go/v2"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestNativeVariantResponse(t *testing.T) {
	connector, err := duckdb.NewConnector("", nil)
	require.NoError(t, err)
	db := sql.OpenDB(connector)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	conn, err := connector.Connect(t.Context())
	require.NoError(t, err)
	require.NoError(t, arrowbridge.Load(t.Context(), conn))
	require.NoError(t, conn.Close())
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
	for reader.Next() {
		record := reader.RecordBatch()
		rows += record.NumRows()
		ex := record.Column(1).(*array.List).ListValues().(*array.Struct)
		require.Equal(t, uint64(math.MaxUint64), ex.Field(0).(*array.Uint64).Value(0))
		attrs := ex.Field(1).(*array.DenseUnion)
		// The last batch's string must be represented in every batch's schema.
		require.Equal(t, 2, attrs.NumFields())
		maps := attrs.Field(attrs.DataType().(*arrow.DenseUnionType).ChildIDs()[mapKind]).(*array.Map)
		if maps.Len() > 0 {
			values := maps.Items().(*array.DenseUnion)
			ints := values.Field(values.DataType().(*arrow.DenseUnionType).ChildIDs()[intKind]).(*array.Int64)
			require.Equal(t, int64(9007199254740993), ints.Value(0))
			lists := values.Field(values.DataType().(*arrow.DenseUnionType).ChildIDs()[listKind]).(*array.List)
			items := lists.ListValues().(*array.DenseUnion)
			floats := items.Field(items.DataType().(*arrow.DenseUnionType).ChildIDs()[doubleKind]).(*array.Float64)
			require.Equal(t, uint64(0x7ff8000000000042), math.Float64bits(floats.Value(1)))
			require.True(t, math.Signbit(floats.Value(2)))
		}
	}
	require.NoError(t, reader.Err())
	require.Equal(t, int64(9001), rows)
}
