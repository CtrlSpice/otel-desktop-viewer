// Package commitbridge observes successful DuckDB write transactions on each
// store connection. The native callback only invalidates a shared revision.
package commitbridge

/*
#cgo CXXFLAGS: -std=c++17 -DDUCKDB_STATIC_BUILD -I${SRCDIR}/include
#include <stdint.h>
int32_t otel_commits_register(void);
*/
import "C"

import (
	"context"
	"database/sql/driver"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
)

var (
	registration = C.otel_commits_register()
	nextID       atomic.Uint64
	observers    sync.Map
)

// Observer coalesces commits without retaining events or blocking on readers.
type Observer struct {
	id       uint64
	mu       sync.Mutex
	revision uint64
	changed  chan struct{}
	closed   bool
}

func New() *Observer {
	o := &Observer{id: nextID.Add(1), changed: make(chan struct{})}
	observers.Store(o.id, o)
	return o
}

// Init attaches the observer to a new writer or pooled connection.
func (o *Observer) Init(conn driver.ExecerContext) error {
	if registration != 0 {
		return fmt.Errorf("register otel_commits: %d", registration)
	}
	_, err := conn.ExecContext(context.Background(), "load otel_commits", nil)
	if err != nil {
		return err
	}
	_, err = conn.ExecContext(context.Background(), "set session otel_commit_observer = "+strconv.FormatUint(o.id, 10), nil)
	return err
}

// Snapshot returns an atomic revision and wake-up channel. A closed channel
// means callers must take another snapshot, not that each commit was queued.
func (o *Observer) Snapshot() (revision uint64, changed <-chan struct{}, closed bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.revision, o.changed, o.closed
}

func (o *Observer) Close() {
	observers.Delete(o.id)
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.closed {
		o.closed = true
		close(o.changed)
	}
}

//export otel_commit_notify
func otel_commit_notify(id C.uint64_t) {
	value, ok := observers.Load(uint64(id))
	if !ok {
		return
	}
	o := value.(*Observer)
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.closed {
		o.revision++
		close(o.changed)
		o.changed = make(chan struct{})
	}
}
