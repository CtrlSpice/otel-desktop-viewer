package ingest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/duckdb/duckdb-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Exercise the real ExecContext and dictionary wrapping boundary. DuckDB's
// scheduler-dependent raw interrupt shape is covered below without timing it.
func TestExecArgsCanceledContext(t *testing.T) {
	connector, err := duckdb.NewConnector("", nil)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, connector.Close()) })

	conn, err := connector.Connect(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, conn.Close()) })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = execArgs(ctx, conn, "select 1", nil, "attributes")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.ErrorIs(t, err, ErrIngestInternal)
}

func TestInterruptedContextError(t *testing.T) {
	interrupted := &duckdb.Error{Type: duckdb.ErrorTypeInterrupt, Msg: "Interrupted!"}
	notInterrupted := &duckdb.Error{Type: duckdb.ErrorTypeExecutor, Msg: "execution failed"}

	tests := []struct {
		name       string
		ctx        context.Context
		err        error
		contextErr error
		same       bool
	}{
		{
			name:       "cancelled context and interrupt",
			ctx:        cancelledContext(t),
			err:        interrupted,
			contextErr: context.Canceled,
		},
		{
			name:       "expired deadline and interrupt",
			ctx:        expiredContext(t),
			err:        interrupted,
			contextErr: context.DeadlineExceeded,
		},
		{
			name: "live context and interrupt",
			ctx:  context.Background(),
			err:  interrupted,
			same: true,
		},
		{
			name: "cancelled context and non-interrupt database error",
			ctx:  cancelledContext(t),
			err:  notInterrupted,
			same: true,
		},
		{
			name:       "interrupt already wrapping cancellation",
			ctx:        cancelledContext(t),
			err:        errors.Join(context.Canceled, interrupted),
			contextErr: context.Canceled,
			same:       true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := interruptedContextError(tc.ctx, tc.err)
			if tc.contextErr == nil {
				assert.NotErrorIs(t, got, context.Canceled)
				assert.NotErrorIs(t, got, context.DeadlineExceeded)
				assert.Same(t, tc.err, got)
				return
			}

			require.ErrorIs(t, got, tc.contextErr)
			var duckErr *duckdb.Error
			require.ErrorAs(t, got, &duckErr)
			assert.Same(t, interrupted, duckErr)
			if tc.same {
				assert.Same(t, tc.err, got)
			}
		})
	}
}

func cancelledContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cancel()
	return ctx
}

func expiredContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	t.Cleanup(cancel)
	return ctx
}
