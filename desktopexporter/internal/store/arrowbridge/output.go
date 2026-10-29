package arrowbridge

/*
#include "bridge.h"
*/
import "C"

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/cdata"
)

// QueryExecer is implemented by sql.DB, sql.Conn and sql.Tx. Reads use the
// caller's connection/transaction, including its snapshot and cancellation.
type QueryExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type outputCall struct {
	ctx     context.Context
	schema  *arrow.Schema
	begin   func(*arrow.Schema) error
	consume func(arrow.RecordBatch) error
	err     error
}

var outputs = struct {
	sync.Mutex
	next  uint64
	calls map[uint64]*outputCall
}{calls: make(map[uint64]*outputCall)}

// Query visits native Arrow batches without constructing JSON or Go row trees.
// begin runs once, including for an empty result. consume borrows each record
// until it returns; retain the record explicitly to keep it beyond the callback.
// Only internally generated SELECT SQL belongs here, never submitted SQL text.
// Callbacks run synchronously and must not query the same connection.
func Query(ctx context.Context, db QueryExecer, query string, args []any,
	begin func(*arrow.Schema) error, consume func(arrow.RecordBatch) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	call := &outputCall{ctx: ctx, begin: begin, consume: consume}
	outputs.Lock()
	if outputs.next == ^uint64(0) {
		outputs.Unlock()
		return errors.New("Arrow output token space exhausted")
	}
	outputs.next++
	token := outputs.next
	outputs.calls[token] = call
	outputs.Unlock()
	defer func() {
		outputs.Lock()
		delete(outputs.calls, token)
		outputs.Unlock()
	}()
	_, err := db.ExecContext(ctx, fmt.Sprintf("COPY (%s) TO 'otel-arrow-%d' (FORMAT otel_arrow_output)", query, token), args...)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if call.err != nil {
		return call.err
	}
	if err == nil && call.schema == nil {
		return errors.New("Arrow query completed without a result schema")
	}
	return err
}

//export otelArrowOutput
func otelArrowOutput(token C.uint64_t, schema *C.struct_ArrowSchema, data *C.struct_ArrowArray) C.int {
	outputs.Lock()
	call := outputs.calls[uint64(token)]
	outputs.Unlock()
	if call == nil {
		return 1
	}
	if call.err != nil {
		return 1
	}
	call.err = call.ctx.Err()
	if call.err != nil {
		return 1
	}
	if schema != nil {
		call.schema, call.err = cdata.ImportCArrowSchema((*cdata.CArrowSchema)(unsafe.Pointer(schema)))
		if call.err == nil {
			call.err = call.begin(call.schema)
		}
	} else if call.schema == nil {
		call.err = errors.New("Arrow output batch arrived before its schema")
	} else {
		var record arrow.RecordBatch
		record, call.err = cdata.ImportCRecordBatchWithSchema((*cdata.CArrowArray)(unsafe.Pointer(data)), call.schema)
		if call.err == nil {
			call.err = call.consume(record)
			record.Release()
		}
	}
	if call.err != nil {
		return 1
	}
	return 0
}
