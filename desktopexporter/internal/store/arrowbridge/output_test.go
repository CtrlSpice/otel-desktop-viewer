package arrowbridge

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/stretchr/testify/require"
)

func TestQueryNativeBatches(t *testing.T) {
	db, _ := openDB(t, "")
	var records []arrow.RecordBatch
	defer func() {
		for _, r := range records {
			r.Release()
		}
	}()
	var schema *arrow.Schema
	err := Query(t.Context(), db, `select i::bigint as id,
		18446744073709551615::ubigint as timestamp,
		[{'name': '雪', 'value': $1::double}] as nested
		from range(20003) t(i) order by i desc`, []any{math.Float64frombits(0x7ff8000000000042)},
		func(s *arrow.Schema) error { schema = s; return nil },
		func(r arrow.RecordBatch) error { r.Retain(); records = append(records, r); return nil })
	require.NoError(t, err)
	require.NotNil(t, schema)
	require.Greater(t, len(records), 1)
	// Buffers outlive the COPY executor and its exporter. Ordering must survive
	// multiple worker-prepared COPY batches and multiple Arrow arrays per batch.
	next := int64(20002)
	for _, r := range records {
		ids := r.Column(0).(*array.Int64)
		timestamps := r.Column(1).(*array.Uint64)
		nested := r.Column(2).(*array.List).ListValues().(*array.Struct)
		for i := 0; i < int(r.NumRows()); i++ {
			require.Equal(t, next, ids.Value(i))
			next--
			require.Equal(t, uint64(math.MaxUint64), timestamps.Value(i))
			require.Equal(t, "雪", nested.Field(0).(*array.String).Value(i))
			require.Equal(t, uint64(0x7ff8000000000042), math.Float64bits(nested.Field(1).(*array.Float64).Value(i)))
		}
	}
	require.Equal(t, int64(-1), next)
}

func TestQueryEmptyFailureAndSnapshot(t *testing.T) {
	db, _ := openDB(t, "")
	beginCount, rowCount := 0, int64(0)
	begin := func(*arrow.Schema) error { beginCount++; return nil }
	consume := func(r arrow.RecordBatch) error { rowCount += r.NumRows(); return nil }
	require.NoError(t, Query(t.Context(), db, `select 1::bigint as id where false`, nil, begin, consume))
	require.Equal(t, 1, beginCount)
	require.Zero(t, rowCount)
	_, err := db.Exec(`create table snapshot_test(id bigint)`)
	require.NoError(t, err)
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	defer tx.Rollback()
	_, err = tx.Exec(`insert into snapshot_test values (42)`)
	require.NoError(t, err)
	require.NoError(t, Query(t.Context(), tx, `select * from snapshot_test`, nil, begin, consume))
	require.Equal(t, int64(1), rowCount)
	require.NoError(t, tx.Rollback())
	failure := errors.New("consumer stopped")
	require.ErrorIs(t, Query(t.Context(), db, `select * from range(10000)`, nil, begin,
		func(arrow.RecordBatch) error { return failure }), failure)
	require.ErrorIs(t, Query(t.Context(), db, `select 1`, nil,
		func(*arrow.Schema) error { return failure }, consume), failure)
	ctx, cancel := context.WithCancel(t.Context())
	require.ErrorIs(t, Query(ctx, db, `select * from range(10000)`, nil, begin,
		func(arrow.RecordBatch) error { cancel(); return nil }), context.Canceled)
	// Failures must release the connection and deregister all output tokens.
	require.NoError(t, Query(t.Context(), db, `select 1`, nil, begin, consume))
	outputs.Lock()
	require.Empty(t, outputs.calls)
	outputs.Unlock()
	_, err = db.Exec(`copy (select 1) to 'otel-arrow-0' (format otel_arrow_output)`)
	require.ErrorContains(t, err, "consumer failed")
}
