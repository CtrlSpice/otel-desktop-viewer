package main

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
)

// Use the registered upstream factory and the resolved production YAML, rather
// than a second copy of the regexes or a test-only redaction implementation.
func startRedaction(t *testing.T, traces consumer.Traces, logs consumer.Logs, metrics consumer.Metrics) (processor.Traces, processor.Logs, processor.Metrics) {
	t.Helper()
	cfg, err := resolveConfig(t, testOptions())
	require.NoError(t, err)
	require.NoError(t, cfg.Validate())
	factories, err := components()
	require.NoError(t, err)
	id := component.MustNewID("redaction")
	factory := factories.Processors[id.Type()]
	set := processor.Settings{ID: id, TelemetrySettings: componenttest.NewNopTelemetrySettings()}
	tr, err := factory.CreateTraces(t.Context(), set, cfg.Processors[id], traces)
	require.NoError(t, err)
	lr, err := factory.CreateLogs(t.Context(), set, cfg.Processors[id], logs)
	require.NoError(t, err)
	mr, err := factory.CreateMetrics(t.Context(), set, cfg.Processors[id], metrics)
	require.NoError(t, err)
	for _, p := range []component.Component{tr, lr, mr} {
		require.NoError(t, p.Start(t.Context(), componenttest.NewNopHost()))
		t.Cleanup(func() { require.NoError(t, p.Shutdown(context.Background())) })
	}
	return tr, lr, mr
}

func TestRedactionSecretValues(t *testing.T) {
	sink := &consumertest.LogsSink{}
	_, p, _ := startRedaction(t, consumertest.NewNop(), sink, consumertest.NewNop())
	for _, input := range []string{
		`{"access":"synthetic-secret","refresh":"synthetic-secret","credentialID":"keep-id"}`,
		`{"clientSecret":"synthetic-secret","password":"synthetic-secret"}`,
		`export MY_API_KEY=synthetic-secret; echo ok`,
		`https://example.test/?access_token=synthetic-secret&count=42`,
		`Authorization: Bearer synthetic-secret`,
		`Basic c3ludGhldGljLXNlY3JldA==`,
		`eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJzeW50aGV0aWMifQ.c3ludGhldGlj`,
		"sk-proj-" + strings.Repeat("a", 58) + "T3BlbkFJ" + strings.Repeat("b", 58),
		"sk-svcacct-" + strings.Repeat("a", 74) + "T3BlbkFJ" + strings.Repeat("b", 74),
		"sk-" + strings.Repeat("a", 20) + "T3BlbkFJ" + strings.Repeat("b", 20),
		"sk-ant-api03-" + strings.Repeat("a", 93) + "AA",
		"sk-ant-admin01-" + strings.Repeat("a", 93) + "AA",
		"ghp_" + strings.Repeat("a", 36),
		"gho_" + strings.Repeat("a", 36),
		"ghu_" + strings.Repeat("a", 36),
		"ghs_" + strings.Repeat("a", 36),
		"ghr_" + strings.Repeat("a", 36),
		"github_pat_" + strings.Repeat("a", 82),
		"xoxb-1234567890-1234567890-" + strings.Repeat("a", 32),
		"xoxp-1234567890-1234567890-1234567890-" + strings.Repeat("a", 32),
		"xapp-1-ABCDEF-1234567890-" + strings.Repeat("a", 32),
		"-----BEGIN PRIVATE KEY-----\nsynthetic-secret\n-----END PRIVATE KEY-----",
		`{"secret":["synthetic-secret","second-secret"]}`,
		`{"token":{"nested":"synthetic-secret"}}`,
		`cookie="synthetic-secret"`,
		`AWS_SECRET_ACCESS_KEY=synthetic-secret`,
		`password="synthetic-secret with spaces"`,
		`password='synthetic-secret with spaces'`,
	} {
		t.Run(input, func(t *testing.T) {
			logs := plog.NewLogs()
			log := logs.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()
			log.Body().SetStr(input)
			log.Attributes().PutStr("payload", input)
			require.NoError(t, p.ConsumeLogs(t.Context(), logs))
			got := sink.AllLogs()[len(sink.AllLogs())-1].ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)
			for _, value := range []string{got.Body().Str(), got.Attributes().AsRaw()["payload"].(string)} {
				assert.NotEqual(t, input, value)
				assert.NotContains(t, value, "synthetic-secret")
				assert.NotContains(t, value, "syntheticsecret")
				assert.NotContains(t, value, "second-secret")
				assert.NotContains(t, value, "c3ludGhldGlj")
				assert.Contains(t, value, "****")
			}
		})
	}
}

func redactionFixture() (ptrace.Traces, plog.Logs, pmetric.Metrics) {
	traces, logs, metrics := ptrace.NewTraces(), plog.NewLogs(), pmetric.NewMetrics()
	rs, rl, rm := traces.ResourceSpans().AppendEmpty(), logs.ResourceLogs().AppendEmpty(), metrics.ResourceMetrics().AppendEmpty()
	ss, sl, sm := rs.ScopeSpans().AppendEmpty(), rl.ScopeLogs().AppendEmpty(), rm.ScopeMetrics().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetName("credential created")
	span.SetTraceID(pcommon.TraceID{15: 1})
	span.SetSpanID(pcommon.SpanID{7: 2})
	span.SetStartTimestamp(1700000000000000000)
	span.SetEndTimestamp(1700000000000000001)
	event := span.Events().AppendEmpty()
	event.SetName("credential created")
	event.SetTimestamp(span.StartTimestamp())
	log := sl.LogRecords().AppendEmpty()
	log.SetTimestamp(span.StartTimestamp())
	log.SetTraceID(span.TraceID())
	log.SetSpanID(span.SpanID())
	body := log.Body().SetEmptySlice()
	body.AppendEmpty().SetStr("credential created")
	body.AppendEmpty().SetStr(`{"access":"synthetic-secret-access","refresh":"synthetic-secret-refresh","credentialID":"keep-id","integrationID":"keep-integration","type":"oauth","active":true,"expires":1700000000000,"gen_ai.usage.input_tokens":42}`)
	nested := body.AppendEmpty().SetEmptyMap()
	nested.PutStr("api_key", "synthetic-secret-body")
	nested.PutInt("exact", math.MaxInt64)
	nested.PutEmptySlice("nested").AppendEmpty().SetEmptyMap().PutStr("password", "synthetic-secret-nested-body")
	metric := sm.Metrics().AppendEmpty()
	metric.SetName("gen_ai.usage.input_tokens")
	dp := metric.SetEmptyGauge().DataPoints().AppendEmpty()
	dp.SetTimestamp(span.StartTimestamp())
	dp.SetIntValue(math.MaxInt64)
	for _, attrs := range []pcommon.Map{
		rs.Resource().Attributes(), ss.Scope().Attributes(), span.Attributes(), event.Attributes(),
		rl.Resource().Attributes(), sl.Scope().Attributes(), log.Attributes(),
		rm.Resource().Attributes(), sm.Scope().Attributes(), dp.Attributes(),
	} {
		attrs.PutStr("service.name", "redaction-test")
		attrs.PutStr("http.request.header.authorization", "synthetic-secret-header")
		attrs.PutStr("clientSecret", "synthetic-secret-client")
		attrs.PutStr("payload", `{"refresh_token":"synthetic-secret-json","credentialID":"keep-id"}`)
		attrs.PutEmptyMap("nested").PutStr("password", "synthetic-secret-nested")
		attrs.PutEmptySlice("array").AppendEmpty().SetStr("Bearer synthetic-secret-array")
		attrs.PutStr("credentialID", "keep-id")
		attrs.PutInt("gen_ai.usage.input_tokens", 42)
		attrs.PutInt("exact", math.MaxInt64)
	}
	return traces, logs, metrics
}

func TestRedactionAllSupportedOwners(t *testing.T) {
	ts, ls, ms := &consumertest.TracesSink{}, &consumertest.LogsSink{}, &consumertest.MetricsSink{}
	tp, lp, mp := startRedaction(t, ts, ls, ms)
	traces, logs, metrics := redactionFixture()
	require.NoError(t, tp.ConsumeTraces(t.Context(), traces))
	require.NoError(t, lp.ConsumeLogs(t.Context(), logs))
	require.NoError(t, mp.ConsumeMetrics(t.Context(), metrics))
	traces, logs, metrics = ts.AllTraces()[0], ls.AllLogs()[0], ms.AllMetrics()[0]
	rs, rl, rm := traces.ResourceSpans().At(0), logs.ResourceLogs().At(0), metrics.ResourceMetrics().At(0)
	ss, sl, sm := rs.ScopeSpans().At(0), rl.ScopeLogs().At(0), rm.ScopeMetrics().At(0)
	span, log, metric := ss.Spans().At(0), sl.LogRecords().At(0), sm.Metrics().At(0)
	for i, attrs := range []pcommon.Map{
		rs.Resource().Attributes(), ss.Scope().Attributes(), span.Attributes(), span.Events().At(0).Attributes(),
		rl.Resource().Attributes(), sl.Scope().Attributes(), log.Attributes(),
		rm.Resource().Attributes(), sm.Scope().Attributes(), metric.Gauge().DataPoints().At(0).Attributes(),
	} {
		assert.NotContains(t, attrs.AsRaw(), "redaction.masked.count", i)
		for key, value := range attrs.All() {
			assert.NotContains(t, value.AsString(), "synthetic-secret", "owner %d key %s", i, key)
		}
		assert.Equal(t, "keep-id", attrs.AsRaw()["credentialID"], i)
		assert.Equal(t, int64(42), attrs.AsRaw()["gen_ai.usage.input_tokens"], i)
		assert.Equal(t, int64(math.MaxInt64), attrs.AsRaw()["exact"], i)
	}
	assert.Equal(t, pcommon.TraceID{15: 1}, span.TraceID())
	assert.Equal(t, pcommon.SpanID{7: 2}, span.SpanID())
	assert.Equal(t, pcommon.Timestamp(1700000000000000000), span.StartTimestamp())
	assert.Equal(t, span.TraceID(), log.TraceID())
	assert.Equal(t, span.StartTimestamp(), log.Timestamp())
	assert.Equal(t, "credential created", log.Body().Slice().At(0).Str())
	assert.NotContains(t, log.Body().AsString(), "synthetic-secret")
	assert.Contains(t, log.Body().Slice().At(1).Str(), `"credentialID":"keep-id"`)
	assert.Contains(t, log.Body().Slice().At(1).Str(), `"integrationID":"keep-integration"`)
	assert.Contains(t, log.Body().Slice().At(1).Str(), `"type":"oauth"`)
	assert.Contains(t, log.Body().Slice().At(1).Str(), `"active":true`)
	assert.Contains(t, log.Body().Slice().At(1).Str(), `"expires":1700000000000`)
	assert.Contains(t, log.Body().Slice().At(1).Str(), `"gen_ai.usage.input_tokens":42`)
	exact, ok := log.Body().Slice().At(2).Map().Get("exact")
	require.True(t, ok)
	assert.Equal(t, int64(math.MaxInt64), exact.Int())
	assert.Equal(t, int64(math.MaxInt64), metric.Gauge().DataPoints().At(0).IntValue())
}

func TestRedactionPreservesNonmatchingValues(t *testing.T) {
	sink := &consumertest.LogsSink{}
	_, p, _ := startRedaction(t, consumertest.NewNop(), sink, consumertest.NewNop())
	logs := plog.NewLogs()
	records := logs.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords()
	log := records.AppendEmpty()
	attrs := log.Attributes()
	for _, key := range []string{"gen_ai.usage.input_tokens", "gen_ai.usage.output_tokens", "token_count", "max_tokens", "credentialID", "trace_id", "span_id", "timestamp"} {
		attrs.PutStr(key, "1234567890123456789")
	}
	attrs.PutInt("int", math.MaxInt64)
	attrs.PutDouble("negative-zero", math.Copysign(0, -1))
	attrs.PutDouble("nan", math.Float64frombits(0x7ff8000000000001))
	attrs.PutDouble("infinity", math.Inf(1))
	attrs.PutBool("bool", true)
	attrs.PutEmptyBytes("bytes").FromRaw([]byte{0, 1, 255})
	attrs.PutEmpty("empty")
	attrs.PutEmptyMap("map").PutInt("exact", math.MaxInt64)
	attrs.PutEmptySlice("array").AppendEmpty().SetDouble(math.SmallestNonzeroFloat64)
	log.Body().SetStr(`{"credentialID":"keep-id","gen_ai.usage.input_tokens":9223372036854775807,"token_count":42,"text":"benign payload"}`)
	attrs.CopyTo(records.AppendEmpty().Body().SetEmptyMap())
	credentialBody := records.AppendEmpty().Body().SetEmptySlice()
	credentialBody.AppendEmpty().SetStr("credential created")
	credentialBody.AppendEmpty().SetStr(`{"credentialID":"keep-id","integrationID":"keep-integration","type":"oauth","active":true}`)
	// Protobuf comparison retains IEEE bits, integers, and all container types.
	before, err := (&plog.ProtoMarshaler{}).MarshalLogs(logs)
	require.NoError(t, err)
	require.NoError(t, p.ConsumeLogs(t.Context(), logs))
	after, err := (&plog.ProtoMarshaler{}).MarshalLogs(sink.AllLogs()[0])
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestRedactionUpstreamCoverageBoundaries(t *testing.T) {
	ts, ms := &consumertest.TracesSink{}, &consumertest.MetricsSink{}
	tp, _, mp := startRedaction(t, ts, consumertest.NewNop(), ms)
	traces, _, metrics := redactionFixture()
	span := traces.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	const unprocessed = "Bearer synthetic-unprocessed"
	span.SetName(unprocessed)
	span.Status().SetMessage(unprocessed)
	span.Events().At(0).SetName(unprocessed)
	span.Links().AppendEmpty().Attributes().PutStr("password", unprocessed)
	metric := metrics.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0)
	metric.Metadata().PutStr("password", unprocessed)
	metric.Gauge().DataPoints().At(0).Exemplars().AppendEmpty().FilteredAttributes().PutStr("password", unprocessed)
	require.NoError(t, tp.ConsumeTraces(t.Context(), traces))
	require.NoError(t, mp.ConsumeMetrics(t.Context(), metrics))
	span = ts.AllTraces()[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	assert.Equal(t, unprocessed, span.Name())
	assert.Equal(t, unprocessed, span.Status().Message())
	assert.Equal(t, unprocessed, span.Events().At(0).Name())
	assert.Equal(t, unprocessed, span.Links().At(0).Attributes().AsRaw()["password"])
	metric = ms.AllMetrics()[0].ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0)
	assert.Equal(t, unprocessed, metric.Metadata().AsRaw()["password"])
	assert.Equal(t, unprocessed, metric.Gauge().DataPoints().At(0).Exemplars().At(0).FilteredAttributes().AsRaw()["password"])
	// Matching composite attributes become strings; nonmatching ones retain type.
	nested, ok := span.Attributes().Get("nested")
	require.True(t, ok)
	assert.Equal(t, pcommon.ValueTypeStr, nested.Type())
	assert.True(t, strings.Contains(nested.Str(), "****"))
}
