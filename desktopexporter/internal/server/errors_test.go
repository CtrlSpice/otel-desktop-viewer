package server

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/duckdb/duckdb-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"golang.org/x/exp/jsonrpc2"
)

// A caller that goes away mid-query is routine -- the UI navigating during a
// stats poll, or a tab closing. It is not an internal error, and treating it as
// one both misreports it to whatever is left listening and fills the log with
// noise the operator can do nothing about.
func TestCancellationIsNotAnInternalError(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "bare cancellation", err: context.Canceled},
		{name: "deadline exceeded", err: context.DeadlineExceeded},
		{
			// The shape actually observed: DuckDB's interrupt message wrapped
			// around the cancellation that caused it.
			name: "duckdb interrupt wrapping cancellation",
			err: fmt.Errorf("GetStats: %w: %w\nINTERRUPT Error: Interrupted!",
				spans.ErrSpansStoreInternal, context.Canceled),
		},
		{
			name: "dictionary interrupt restored as cancellation",
			err: fmt.Errorf("dictionary flush attributes: %w: %w", ingest.ErrIngestInternal,
				errors.Join(context.Canceled, &duckdb.Error{Type: duckdb.ErrorTypeInterrupt, Msg: "Interrupted!"})),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, ErrRequestCanceled, mapStoreError(tc.err))
			assert.NotEqual(t, jsonrpc2.ErrInternal, mapStoreError(tc.err))
		})
	}
}

// Cancellation must not reach the log; genuine internal failures still must.
func TestHandleStoreErrorLogging(t *testing.T) {
	newHandler := func() (*JSONRPCHandler, *observer.ObservedLogs) {
		core, logs := observer.New(zap.ErrorLevel)
		return &JSONRPCHandler{logger: zap.New(core)}, logs
	}

	t.Run("cancellation is silent", func(t *testing.T) {
		h, logs := newHandler()
		err := h.handleStoreError(context.Background(), fmt.Errorf("query aborted: %w", context.Canceled))
		assert.Equal(t, ErrRequestCanceled, err)
		assert.Zero(t, logs.Len(), "cancellation should not be logged")
	})

	t.Run("internal errors are still logged", func(t *testing.T) {
		h, logs := newHandler()
		err := h.handleStoreError(context.Background(), fmt.Errorf("disk on fire"))
		assert.Equal(t, jsonrpc2.ErrInternal, err)
		require.Equal(t, 1, logs.Len(), "unexpected failures must still be logged")
		assert.Equal(t, "store error", logs.All()[0].Message)
	})

	t.Run("expected outcomes stay silent", func(t *testing.T) {
		h, logs := newHandler()
		err := h.handleStoreError(context.Background(), spans.ErrTraceIDNotFound)
		assert.Equal(t, ErrTraceNotFound, err)
		assert.Zero(t, logs.Len())
	})

	t.Run("nil passes through", func(t *testing.T) {
		h, logs := newHandler()
		assert.NoError(t, h.handleStoreError(context.Background(), nil))
		assert.Zero(t, logs.Len())
	})
}

func TestHandleStoreErrorNormalizesTypedInterrupts(t *testing.T) {
	newHandler := func() (*JSONRPCHandler, *observer.ObservedLogs) {
		core, logs := observer.New(zap.ErrorLevel)
		return &JSONRPCHandler{logger: zap.New(core)}, logs
	}
	interrupt := func() error {
		return &duckdb.Error{Type: duckdb.ErrorTypeInterrupt, Msg: "Interrupted!"}
	}

	tests := []struct {
		name    string
		ctx     context.Context
		err     error
		want    error
		logWant int
	}{
		{
			name:    "cancelled context and raw interrupt are silent cancellation",
			ctx:     canceledContext(t),
			err:     interrupt(),
			want:    ErrRequestCanceled,
			logWant: 0,
		},
		{
			name:    "live context interrupt remains internal",
			ctx:     context.Background(),
			err:     interrupt(),
			want:    jsonrpc2.ErrInternal,
			logWant: 1,
		},
		{
			name:    "cancelled context non-interrupt remains internal",
			ctx:     canceledContext(t),
			err:     &duckdb.Error{Type: duckdb.ErrorTypeConversion, Msg: "conversion failed"},
			want:    jsonrpc2.ErrInternal,
			logWant: 1,
		},
		{
			name:    "deadline context interrupt is silent cancellation",
			ctx:     expiredContext(t),
			err:     interrupt(),
			want:    ErrRequestCanceled,
			logWant: 0,
		},
		{
			name:    "already wrapped cancellation stays silent",
			ctx:     canceledContext(t),
			err:     errors.Join(context.Canceled, interrupt()),
			want:    ErrRequestCanceled,
			logWant: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, logs := newHandler()
			assert.Equal(t, tc.want, h.handleStoreError(tc.ctx, tc.err))
			assert.Len(t, logs.All(), tc.logWant)
			if tc.logWant > 0 {
				assert.Equal(t, tc.err, logs.All()[0].Context[0].Interface)
			}
		})
	}
}

func canceledContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func expiredContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	t.Cleanup(cancel)
	return ctx
}
