// Package arrowbridge hands C-allocated Arrow batches to the stock DuckDB engine
// through a statically linked table function on the caller's existing connection.
package arrowbridge

/*
#cgo CFLAGS: -std=c11 -DDUCKDB_STATIC_BUILD
#include "bridge.h"
*/
import "C"

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/cdata"
	"github.com/apache/arrow-go/v18/arrow/memory/mallocator"
	_ "github.com/duckdb/duckdb-go/v2"
)

// Allocator returns C-owned buffers. Go-allocated buffers must not be passed to
// Insert: the importer retains their addresses across cgo calls until execution
// finishes. Each builder and record must still be released by its owner.
func Allocator() *mallocator.Mallocator { return mallocator.NewMallocator() }

// Registration must precede the first database open, including opens outside
// Store. Loading the registered extension is a per-database operation.
var registration = C.otel_arrow_register()

func Load(ctx context.Context, conn driver.Conn) error {
	if registration != 0 {
		return fmt.Errorf("register otel_arrow: %d", registration)
	}
	_, err := conn.(driver.ExecerContext).ExecContext(ctx, "LOAD otel_arrow", nil)
	return err
}

func publish(record arrow.RecordBatch) (uint64, func(), error) {
	schema := (*C.struct_ArrowSchema)(C.calloc(1, C.sizeof_struct_ArrowSchema))
	array := (*C.struct_ArrowArray)(C.calloc(1, C.sizeof_struct_ArrowArray))
	if schema == nil || array == nil {
		C.free(unsafe.Pointer(schema))
		C.free(unsafe.Pointer(array))
		return 0, nil, errors.New("allocate Arrow C structures")
	}
	cdata.ExportArrowRecordBatch(record, (*cdata.CArrowArray)(unsafe.Pointer(array)), (*cdata.CArrowSchema)(unsafe.Pointer(schema)))
	token := C.otel_arrow_publish(schema, array)
	if token == 0 {
		cdata.ReleaseCArrowArray((*cdata.CArrowArray)(unsafe.Pointer(array)))
		cdata.ReleaseCArrowSchema((*cdata.CArrowSchema)(unsafe.Pointer(schema)))
		C.free(unsafe.Pointer(schema))
		C.free(unsafe.Pointer(array))
		return 0, nil, errors.New("register Arrow batch")
	}
	return uint64(token), sync.OnceFunc(func() {
		C.otel_arrow_unpublish(token)
		C.free(unsafe.Pointer(schema))
		C.free(unsafe.Pointer(array))
	}), nil
}

// Insert executes an application-owned INSERT SELECT, replacing its single
// appended_data input with the one-shot Arrow scan. Query text must be a constant
// controlled by the application; received values belong in the Arrow batch.
// The caller owns the transaction and must serialize use of conn. Distinct
// connections can insert concurrently. The record must use Allocator buffers.
func Insert(ctx context.Context, conn driver.Conn, query string, record arrow.RecordBatch) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.Count(query, "appended_data") != 1 {
		return errors.New("Arrow insert requires exactly one appended_data input")
	}
	token, release, err := publish(record)
	if err != nil {
		return err
	}
	defer release()
	query = strings.Replace(query, "appended_data", "otel_arrow_scan("+strconv.FormatUint(token, 10)+"::ubigint)", 1)
	_, err = conn.(driver.ExecerContext).ExecContext(ctx, query, nil)
	return err
}
