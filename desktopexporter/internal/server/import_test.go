package server

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/nested"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

func TestImportConfigRPC(t *testing.T) {
	for _, port := range []int{4318, 54318, 0} {
		s, err := NewServer("127.0.0.1:0", port, nil, zap.NewNop(), nil, WithOTLPGRPCPort(54317))
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewBufferString(`{"jsonrpc":"2.0","id":7,"method":"getImportConfig"}`))
		response := httptest.NewRecorder()
		s.server.Handler.ServeHTTP(response, request)
		var reply struct {
			ID     int `json:"id"`
			Result struct {
				Port     int `json:"otlpHttpPort"`
				GRPCPort int `json:"otlpGrpcPort"`
			} `json:"result"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &reply))
		require.Equal(t, 7, reply.ID)
		if port == 0 {
			require.NotNil(t, reply.Error)
			require.Contains(t, reply.Error.Message, "not configured")
		} else {
			require.Nil(t, reply.Error)
			require.Equal(t, port, reply.Result.Port)
			require.Equal(t, 54317, reply.Result.GRPCPort)
		}
	}
}

func TestImportTargetUsesStoredIdentityAndRedaction(t *testing.T) {
	server, db, teardown := setupServerWithStore(t)
	defer teardown()
	normalizer, err := importRedactors()
	require.NoError(t, err)
	lookup := func(signal string, body []byte) *store.ImportTarget {
		t.Helper()
		before, _, _ := db.CommitSnapshot()
		response, err := http.Post(server.URL+"/api/import-target/"+signal, "application/json", bytes.NewReader(body))
		require.NoError(t, err)
		defer response.Body.Close()
		require.Equal(t, http.StatusOK, response.StatusCode)
		var target *store.ImportTarget
		require.NoError(t, json.NewDecoder(response.Body).Decode(&target))
		after, _, _ := db.CommitSnapshot()
		require.Equal(t, before, after, "lookups must not write or notify")
		return target
	}
	t.Run("trace", func(t *testing.T) {
		data := ptrace.NewTraces()
		span := data.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
		span.SetTraceID(pcommon.TraceID{15: 1})
		span.SetSpanID(pcommon.SpanID{0: 0xff, 7: 2})
		span.SetName("first")
		body, err := (&ptrace.JSONMarshaler{}).MarshalTraces(data)
		require.NoError(t, err)
		require.Nil(t, lookup("traces", body))
		require.NoError(t, db.WithConn(func(conn driver.Conn) error { return spans.Ingest(t.Context(), conn, data) }))
		target := lookup("traces", body)
		require.NotNil(t, target)
		require.Equal(t, span.TraceID().String(), target.ID)
		require.Equal(t, span.SpanID().String(), target.SpanID)
	})
	t.Run("log", func(t *testing.T) {
		data := plog.NewLogs()
		resource := data.ResourceLogs().AppendEmpty()
		resource.Resource().Attributes().PutStr("password", "resource secret")
		scope := resource.ScopeLogs().AppendEmpty()
		scope.Scope().Attributes().PutStr("token", "scope secret")
		log := scope.LogRecords().AppendEmpty()
		log.SetTimestamp(18_446_744_073_709_551_615)
		log.SetObservedTimestamp(18_446_744_073_709_551_614)
		log.SetTraceID(pcommon.TraceID{15: 1})
		log.SetSpanID(pcommon.SpanID{0: 0xff, 7: 2})
		log.SetSeverityNumber(plog.SeverityNumberInfo)
		log.Body().SetStr("Authorization: Bearer synthetic-test-credential")
		log.Attributes().PutInt("exact", 9_007_199_254_740_993)
		body, err := (&plog.JSONMarshaler{}).MarshalLogs(data)
		require.NoError(t, err)
		require.Nil(t, lookup("logs", body))
		require.NoError(t, normalizer.logs.ConsumeLogs(t.Context(), data))
		require.NoError(t, db.WithConn(func(conn driver.Conn) error { return logs.Ingest(t.Context(), conn, data) }))
		target := lookup("logs", body)
		require.NotNil(t, target)
		require.NotEmpty(t, target.ID)
		require.Equal(t, target, lookup("logs", body), "an existing identical log is a valid destination")
		log.Attributes().PutInt("exact", 9_007_199_254_740_992)
		different, err := (&plog.JSONMarshaler{}).MarshalLogs(data)
		require.NoError(t, err)
		require.Nil(t, lookup("logs", different), "nearby integers must not match")
	})
	t.Run("metric", func(t *testing.T) {
		data := pmetric.NewMetrics()
		resource := data.ResourceMetrics().AppendEmpty()
		resource.Resource().Attributes().PutStr("api_key", "resource secret")
		scope := resource.ScopeMetrics().AppendEmpty()
		scope.Scope().SetName("test-scope")
		metric := scope.Metrics().AppendEmpty()
		metric.SetName("first.metric")
		metric.SetUnit("s")
		sum := metric.SetEmptySum()
		sum.SetIsMonotonic(true)
		sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		sum.DataPoints().AppendEmpty().SetIntValue(42)
		body, err := (&pmetric.JSONMarshaler{}).MarshalMetrics(data)
		require.NoError(t, err)
		require.Nil(t, lookup("metrics", body))
		require.NoError(t, normalizer.metrics.ConsumeMetrics(t.Context(), data))
		require.NoError(t, db.WithConn(func(conn driver.Conn) error {
			_, err := nested.IngestMetrics(t.Context(), conn, data)
			return err
		}))
		target := lookup("metrics", body)
		require.NotNil(t, target)
		require.NotEmpty(t, target.ID)
		sum.SetIsMonotonic(false)
		different, err := (&pmetric.JSONMarshaler{}).MarshalMetrics(data)
		require.NoError(t, err)
		require.Nil(t, lookup("metrics", different))
	})
}
