// Package arrowread retains native DuckDB Arrow batches for browser responses.
// Variant values keep their metadata and value bytes instead of expanding into
// nested Arrow types. Value nesting therefore does not deepen the Arrow schema.
package arrowread

import (
	"context"
	"fmt"
	"io"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/arrowbridge"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/ipc"
)

// Result owns native query batches and their schema. It must be closed to release
// the retained buffers. Variant's schema is independent of the values in a batch.
type Result struct {
	schema  *arrow.Schema
	records []arrow.RecordBatch
	rows    int64
}

func Read(ctx context.Context, db arrowbridge.QueryExecer, query string, args ...any) (*Result, error) {
	r := &Result{}
	err := arrowbridge.Query(ctx, db, query, args, func(schema *arrow.Schema) error {
		r.schema = schema
		return nil
	}, func(record arrow.RecordBatch) error {
		record.Retain()
		r.records = append(r.records, record)
		r.rows += record.NumRows()
		return nil
	})
	if err != nil {
		r.Close()
		return nil, err
	}
	return r, nil
}

func (r *Result) Schema() *arrow.Schema { return r.schema }
func (r *Result) NumRows() int64        { return r.rows }

// SingleInt64 reads a scalar control column in a one-row dataset. It does not
// project or materialize the associated nested payload.
func (r *Result) SingleInt64(name string) (int64, error) {
	indices := r.schema.FieldIndices(name)
	if r.rows != 1 || len(indices) != 1 {
		return 0, fmt.Errorf("expected one row and one %s column", name)
	}
	for _, record := range r.records {
		if record.NumRows() == 0 {
			continue
		}
		values, ok := record.Column(indices[0]).(*array.Int64)
		if !ok || values.IsNull(0) {
			return 0, fmt.Errorf("expected non-null int64 column %s", name)
		}
		return values.Value(0), nil
	}
	return 0, fmt.Errorf("missing result column %s", name)
}

func (r *Result) Close() {
	for _, record := range r.records {
		record.Release()
	}
	r.records = nil
}

// Write emits complete IPC only on success. A caller must treat a write error
// as a failed load, not append a JSON error or an IPC end marker to partial data.
func (r *Result) Write(ctx context.Context, dst io.Writer) (err error) {
	sink := &responseSink{dst: dst}
	writer := ipc.NewWriter(sink, ipc.WithSchema(r.schema))
	defer func() {
		if err != nil {
			// Close releases IPC dictionaries even after a failure, but must not
			// send a success end marker after cancellation or a write error.
			sink.aborted = true
			_ = writer.Close()
		} else {
			err = writer.Close()
		}
	}()
	for _, record := range r.records {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

type responseSink struct {
	dst     io.Writer
	aborted bool
}

func (s *responseSink) Write(p []byte) (int, error) {
	if s.aborted {
		return 0, io.ErrClosedPipe
	}
	return s.dst.Write(p)
}
