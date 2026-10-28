package arrowbridge

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"math"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory/mallocator"
	pv "github.com/apache/arrow-go/v18/parquet/variant"
	"github.com/duckdb/duckdb-go/v2"
	"github.com/stretchr/testify/require"
)

func openDB(t *testing.T, path string) (*sql.DB, driver.Conn) {
	t.Helper()
	c, err := duckdb.NewConnector(path, nil)
	require.NoError(t, err)
	db := sql.OpenDB(c)
	conn, err := c.Connect(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close(); require.NoError(t, db.Close()) })
	require.NoError(t, Load(t.Context(), conn))
	return db, conn
}

func batch(t *testing.T, n int) (arrow.RecordBatch, *mallocator.Mallocator) {
	t.Helper()
	alloc := Allocator()
	fields := []arrow.Field{
		{Name: "id", Type: arrow.PrimitiveTypes.Int64},
		{Name: "reading", Type: arrow.PrimitiveTypes.Float64},
		{Name: "points", Type: arrow.ListOf(arrow.StructOf(
			arrow.Field{Name: "timestamp", Type: arrow.PrimitiveTypes.Uint64},
			arrow.Field{Name: "value", Type: arrow.BinaryTypes.Binary},
		)), Nullable: true},
	}
	b := array.NewRecordBuilder(alloc, arrow.NewSchema(fields, nil))
	defer b.Release()
	for i := 0; i < n; i++ {
		b.Field(0).(*array.Int64Builder).Append(int64(i))
		bits := uint64(0x8000000000000000)
		if i%2 != 0 {
			bits = 0x7ff8000000000042
		}
		b.Field(1).(*array.Float64Builder).Append(math.Float64frombits(bits))
		list := b.Field(2).(*array.ListBuilder)
		if i%3 == 0 {
			list.AppendNull()
			continue
		}
		list.Append(true)
		if i%3 == 1 {
			continue
		}
		point := list.ValueBuilder().(*array.StructBuilder)
		point.Append(true)
		point.FieldBuilder(0).(*array.Uint64Builder).Append(math.MaxUint64)
		var vb pv.Builder
		require.NoError(t, vb.AppendFloat64(math.Float64frombits(bits)))
		v, err := vb.Build()
		require.NoError(t, err)
		raw := append(append([]byte(nil), v.Metadata().Bytes()...), v.Bytes()...)
		point.FieldBuilder(1).(*array.BinaryBuilder).Append(raw)
	}
	rec := b.NewRecordBatch()
	defer rec.Release()
	// Match production context sharing: dictionaries on fields inside a struct.
	indices := array.NewInt32Builder(alloc)
	defer indices.Release()
	for i := 0; i < n; i++ {
		indices.Append(int32(i % 2))
	}
	ix := indices.NewArray()
	defer ix.Release()
	values := array.NewStringBuilder(alloc)
	defer values.Release()
	values.AppendValues([]string{"雪", "O'Reilly"}, nil)
	vs := values.NewArray()
	defer vs.Release()
	dict := array.NewDictionaryArray(&arrow.DictionaryType{IndexType: arrow.PrimitiveTypes.Int32, ValueType: arrow.BinaryTypes.String}, ix, vs)
	defer dict.Release()
	ctx, err := array.NewStructArray([]arrow.Array{dict}, []string{"name"})
	require.NoError(t, err)
	defer ctx.Release()
	fields = append(fields, arrow.Field{Name: "context", Type: ctx.DataType()})
	return array.NewRecordBatch(arrow.NewSchema(fields, nil), append(rec.Columns(), ctx), int64(n)), alloc
}

const createSQL = `create table landed(id bigint primary key,reading double,points struct(timestamp ubigint,value variant)[],context struct(name varchar))`
const insertSQL = `insert into landed select id,reading,list_transform(points,lambda p:struct_update(p,value:=variant_bytes_to_variant(p.value))),context from appended_data`

func TestLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "arrow.duckdb")
	db, conn := openDB(t, path)
	_, err := db.Exec(createSQL)
	require.NoError(t, err)
	const count = 5003
	write := func(ctx context.Context, query string) error {
		rec, alloc := batch(t, count)
		err := Insert(ctx, conn, query, rec)
		rec.Release()
		alloc.AssertSize(t, 0)
		return err
	}
	tx, err := conn.Begin()
	require.NoError(t, err)
	require.NoError(t, write(t.Context(), insertSQL))
	require.NoError(t, tx.Commit())
	verify := func() {
		var n int
		require.NoError(t, db.QueryRow(`select count(*) from landed where context.name=case id%2 when 0 then '雪' else 'O''Reilly' end and
		case id%3 when 0 then points is null when 1 then len(points)=0 else len(points)=1 and points[1].timestamp=18446744073709551615::ubigint end`).Scan(&n))
		require.Equal(t, count, n)
		for _, id := range []int{2, 5} {
			var native, nested float64
			require.NoError(t, db.QueryRow(`select reading,points[1].value::double from landed where id=?`, id).Scan(&native, &nested))
			bits := uint64(0x8000000000000000)
			if id%2 != 0 {
				bits = 0x7ff8000000000042
			}
			require.Equal(t, bits, math.Float64bits(native))
			require.Equal(t, bits, math.Float64bits(nested))
		}
	}
	verify()
	require.NoError(t, write(t.Context(), `select id from appended_data limit 1`))
	require.Error(t, write(t.Context(), insertSQL))                           // primary key failure after consumption
	require.Error(t, write(t.Context(), `select missing from appended_data`)) // bind failure
	tx, err = conn.Begin()
	require.NoError(t, err)
	_, err = conn.(driver.ExecerContext).ExecContext(t.Context(), `delete from landed`, nil)
	require.NoError(t, err)
	require.NoError(t, write(t.Context(), insertSQL))
	require.NoError(t, tx.Rollback())
	verify()
	require.NoError(t, conn.Close())
	require.NoError(t, db.Close())
	// Reopen without retaining a process-local Arrow view in the catalog.
	db, conn = openDB(t, path)
	verify()
}

func TestTokenLifetimeAndConcurrentInputs(t *testing.T) {
	db, _ := openDB(t, "")
	const workers = 4
	var wg sync.WaitGroup
	errors := make(chan error, workers)
	for i := 0; i < workers; i++ {
		// Export all inputs before any execution to exercise registry coexistence.
		rec, alloc := batch(t, 4097)
		token, release, err := publish(rec)
		require.NoError(t, err)
		rec.Release()
		wg.Add(1)
		go func() {
			defer wg.Done()
			runtime.GC()
			var n int
			err := db.QueryRow(fmt.Sprintf(`select count(*) from otel_arrow_scan(%d::ubigint) where context.name='雪'`, token)).Scan(&n)
			if err == nil && n != 2049 {
				err = fmt.Errorf("row count: %d", n)
			}
			release()
			alloc.AssertSize(t, 0)
			errors <- err
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	rec, alloc := batch(t, 3)
	token, release, err := publish(rec)
	require.NoError(t, err)
	rec.Release()
	release()
	release()
	alloc.AssertSize(t, 0)
	_, err = db.Exec(fmt.Sprintf(`select * from otel_arrow_scan(%d::ubigint)`, token))
	require.ErrorContains(t, err, "expired Arrow batch token")
}

func TestCancellation(t *testing.T) {
	_, conn := openDB(t, "")
	rec, alloc := batch(t, 5003)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, Insert(ctx, conn, `select * from appended_data`, rec), context.Canceled)
	// A large non-materialized join keeps the query active after Arrow is consumed.
	ctx, cancel = context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	err := Insert(ctx, conn, `select sum(a.id*r.i) from appended_data a cross join range(1000000000) r(i)`, rec)
	require.Error(t, err)
	require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
	rec.Release()
	alloc.AssertSize(t, 0)
	// Cancellation must leave the connection reusable.
	_, err = conn.(driver.ExecerContext).ExecContext(t.Context(), "select 1", nil)
	require.NoError(t, err)
}
