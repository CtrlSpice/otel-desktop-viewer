package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

func TestRPCTraceContextAndFailureStatus(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		failure    bool
	}{
		{"success", `{"jsonrpc":"2.0","id":1,"method":"query","params":{"sql":"SELECT 1"}}`, false},
		{"query error", `{"jsonrpc":"2.0","id":1,"method":"query","params":{"sql":"SELECT * FROM missing_table"}}`, true},
		{"decode error", `{`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			defer provider.Shutdown(context.Background())
			settings := componenttest.NewNopTelemetrySettings()
			settings.TracerProvider = provider
			tel, err := telemetry.New(settings, true, false)
			require.NoError(t, err)
			db, err := store.NewStore(context.Background(), "", zap.NewNop())
			require.NoError(t, err)
			defer db.Close()
			server, err := NewServer("localhost:0", 0, db, zap.NewNop(), tel)
			require.NoError(t, err)
			request := httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(tc.body))
			request.Header.Set("traceparent", "00-11111111111111111111111111111111-2222222222222222-01")
			server.rpcHandler(httptest.NewRecorder(), request)
			spans := recorder.Ended()
			require.Len(t, spans, 2)
			encode, rpc := spans[0], spans[1]
			require.Equal(t, "11111111111111111111111111111111", rpc.SpanContext().TraceID().String())
			require.Equal(t, "2222222222222222", rpc.Parent().SpanID().String())
			require.True(t, rpc.Parent().IsRemote())
			require.Equal(t, trace.SpanKindServer, rpc.SpanKind())
			require.Equal(t, rpc.SpanContext().SpanID(), encode.Parent().SpanID())
			if tc.failure {
				require.Equal(t, codes.Error, rpc.Status().Code)
				require.NotEmpty(t, rpc.Events())
			} else {
				require.Equal(t, codes.Unset, rpc.Status().Code)
				require.Equal(t, "viewer.rpc query", rpc.Name())
			}
		})
	}
}
