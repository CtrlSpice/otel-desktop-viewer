package telemetry

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
)

// The point of these tests is that call sites are unconditional: nothing in the
// exporter checks whether telemetry is on before calling a method, so every
// method on a disabled Telemetry has to be callable, return a usable context,
// and hand back a closer that is safe to call -- with an error or without.

func TestDisabledIsSafeToCall(t *testing.T) {
	tel := Disabled()
	require.NotNil(t, tel)

	boom := errors.New("boom")

	t.Run("ingest", func(t *testing.T) {
		ctx, end := tel.Ingest(context.Background(), "traces", 12)
		require.NotNil(t, ctx)
		require.NotNil(t, end)
		assert.NotPanics(t, func() { end(nil) })

		ctx, end = tel.Ingest(ctx, "logs", 0)
		require.NotNil(t, ctx)
		assert.NotPanics(t, func() { end(boom) })
	})

	t.Run("rpc", func(t *testing.T) {
		ctx, end := tel.RPC(context.Background(), "getTraceSummaries")
		require.NotNil(t, ctx)
		require.NotNil(t, end)
		assert.NotPanics(t, func() { end(2048, nil) })

		_, end = tel.RPC(ctx, "getTrace")
		assert.NotPanics(t, func() { end(0, boom) })
	})

	t.Run("rpc encode", func(t *testing.T) {
		end := tel.RPCEncode(context.Background())
		require.NotNil(t, end)
		assert.NotPanics(t, func() { end(nil) })

		assert.NotPanics(t, func() { tel.RPCEncode(context.Background())(boom) })
	})

	t.Run("retention", func(t *testing.T) {
		ctx, end := tel.Retention(context.Background())
		require.NotNil(t, ctx)
		require.NotNil(t, end)
		assert.NotPanics(t, func() { end(4096, 1024, nil) })

		_, end = tel.Retention(ctx)
		assert.NotPanics(t, func() { end(0, 0, boom) })
	})
}

// Disabled suppresses ingest instrumentation, so Ingest hands back the context
// it was given rather than starting a span.
func TestDisabledIngestPassesContextThrough(t *testing.T) {
	type key struct{}
	parent := context.WithValue(context.Background(), key{}, "value")

	ctx, end := Disabled().Ingest(parent, "metrics", 3)
	end(nil)

	assert.Equal(t, parent, ctx)
}

// New with enabled=false must give back something equally safe, so that
// "telemetry: off" is not a special case for any caller.
func TestNewDisabledIsSafeToCall(t *testing.T) {
	tel, err := New(componenttest.NewNopTelemetrySettings(), false, false)
	require.NoError(t, err)
	require.NotNil(t, tel)

	assert.NotPanics(t, func() {
		_, endIngest := tel.Ingest(context.Background(), "traces", 1)
		endIngest(nil)

		_, endRPC := tel.RPC(context.Background(), "getTrace")
		endRPC(1, nil)

		tel.RPCEncode(context.Background())(nil)

		_, endRetention := tel.Retention(context.Background())
		endRetention(1, 0, nil)
	})
}

// Enabled telemetry backed by the nop providers must be just as safe, and the
// instruments must all have been constructed.
func TestNewEnabledIsSafeToCall(t *testing.T) {
	tel, err := New(componenttest.NewNopTelemetrySettings(), true, true)
	require.NoError(t, err)
	require.NotNil(t, tel)

	assert.NotPanics(t, func() {
		ctx, endIngest := tel.Ingest(context.Background(), "traces", 7)
		endIngest(nil)

		ctx, endRPC := tel.RPC(ctx, "getTraceSummaries")
		tel.RPCEncode(ctx)(nil)
		endRPC(512, nil)

		_, endRetention := tel.Retention(ctx)
		endRetention(2048, 1024, nil)
	})
}
