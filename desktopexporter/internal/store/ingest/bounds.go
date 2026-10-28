package ingest

import (
	"context"
	"crypto/sha256"
	"database/sql/driver"
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/duckdb/duckdb-go/v2"
)

// BoundsBatch shares explicit histogram boundary vectors across datapoints.
type BoundsBatch struct {
	rows    map[duckdb.UUID][]float64
	flushed *FlushedIDs
}

func NewBoundsBatch(flushed *FlushedIDs) *BoundsBatch {
	return &BoundsBatch{rows: make(map[duckdb.UUID][]float64), flushed: flushed}
}

func BoundsID(bounds []float64) duckdb.UUID {
	h := sha256.New()
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(len(bounds)))
	h.Write(buf[:])
	for _, bound := range bounds {
		binary.BigEndian.PutUint64(buf[:], math.Float64bits(bound))
		h.Write(buf[:])
	}
	var id duckdb.UUID
	copy(id[:], h.Sum(nil)[:16])
	return id
}

func (b *BoundsBatch) AddBounds(bounds []float64) duckdb.UUID {
	id := BoundsID(bounds)
	if bounds == nil {
		bounds = []float64{}
	}
	b.rows[id] = bounds
	return id
}

func (b *BoundsBatch) Flush(ctx context.Context, conn driver.Conn) error {
	rows := unseen(b.flushed, b.rows)
	if len(rows) == 0 {
		return nil
	}
	ids := make([]string, 0, len(rows))
	vectors := make([][]float64, 0, len(rows))
	for id, bounds := range rows {
		ids = append(ids, FormatUUID(id))
		vectors = append(vectors, bounds)
	}
	if err := execArgs(ctx, conn, `insert into histogram_bounds
		select unnest(?::varchar[])::uuid, unnest(?::double[][])
		on conflict (id) do nothing`, []any{ids, vectors}, "histogram_bounds"); err != nil {
		return err
	}
	mark(b.flushed, rows)
	return nil
}

func execArgs(ctx context.Context, conn driver.Conn, query string, args []any, what string) error {
	dconn, ok := conn.(*duckdb.Conn)
	if !ok {
		return fmt.Errorf("%s: %w: connection is not DuckDB", what, ErrIngestInternal)
	}
	named, err := prepareNamedValues(dconn, args)
	if err != nil {
		return fmt.Errorf("flush %s: %w: %w", what, ErrIngestInternal, err)
	}
	_, err = dconn.ExecContext(ctx, query, named)
	if err != nil {
		return fmt.Errorf("flush %s: %w: %w", what, ErrIngestInternal, InterruptedContextError(ctx, err))
	}
	return nil
}

// InterruptedContextError restores cancellation identity lost by the driver.
func InterruptedContextError(ctx context.Context, err error) error {
	ctxErr := ctx.Err()
	if ctxErr == nil {
		return err
	}
	var duckErr *duckdb.Error
	if errors.As(err, &duckErr) && duckErr.Type == duckdb.ErrorTypeInterrupt && !errors.Is(err, ctxErr) {
		return errors.Join(ctxErr, err)
	}
	return err
}

func prepareNamedValues(dconn *duckdb.Conn, args []any) ([]driver.NamedValue, error) {
	out := make([]driver.NamedValue, 0, len(args))
	for i, arg := range args {
		nv := driver.NamedValue{Ordinal: i + 1, Value: arg}
		if err := dconn.CheckNamedValue(&nv); err != nil {
			converted, err := driver.DefaultParameterConverter.ConvertValue(arg)
			if err != nil {
				return nil, err
			}
			nv.Value = converted
		}
		out = append(out, nv)
	}
	return out, nil
}
