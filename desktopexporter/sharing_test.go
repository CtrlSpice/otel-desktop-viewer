package desktopexporter

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Deleting sharedcomponent removed the guarantee that all three signal
// pipelines got one exporter instance. What has to hold instead is weaker but
// sufficient: three independent exporters resolving the *same* store, so the
// store's own RWMutex still serializes every writer.
//
// If this ever fails, each pipeline has its own DuckDB and the mutex is
// guarding nothing.
func TestAllSignalsResolveTheSameStore(t *testing.T) {
	ctx := context.Background()
	host, _ := startTestExtension(t)
	cfg := createDefaultConfig().(*Config)

	var exporters []*desktopExporter
	for i := 0; i < 3; i++ {
		e, _, err := newSignalExporter(cfg, testExporterSettings(t))
		require.NoError(t, err)
		require.NoError(t, e.Start(ctx, host))
		exporters = append(exporters, e)
	}

	require.NotNil(t, exporters[0].store)
	assert.Same(t, exporters[0].store, exporters[1].store)
	assert.Same(t, exporters[0].store, exporters[2].store)
}

func traceBatch(name string, round, n int) ptrace.Traces {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", name)
	ss := rs.ScopeSpans().AppendEmpty()
	for i := 0; i < n; i++ {
		s := ss.Spans().AppendEmpty()
		s.SetTraceID(pcommon.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, byte(round + 1), byte(i + 1)})
		s.SetSpanID(pcommon.SpanID{1, 2, 3, 4, 5, 6, byte(round + 1), byte(i + 1)})
		s.SetName("span")
		now := time.Now()
		s.SetStartTimestamp(pcommon.NewTimestampFromTime(now))
		s.SetEndTimestamp(pcommon.NewTimestampFromTime(now.Add(time.Millisecond)))
	}
	return td
}

func logBatch(name string, n int) plog.Logs {
	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("service.name", name)
	sl := rl.ScopeLogs().AppendEmpty()
	for i := 0; i < n; i++ {
		lr := sl.LogRecords().AppendEmpty()
		lr.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
		lr.SetSeverityNumber(plog.SeverityNumberInfo)
		lr.Body().SetStr("hello")
	}
	return ld
}

func metricBatch(name string, n int) pmetric.Metrics {
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", name)
	sm := rm.ScopeMetrics().AppendEmpty()
	m := sm.Metrics().AppendEmpty()
	m.SetName("concurrent.gauge")
	g := m.SetEmptyGauge()
	for i := 0; i < n; i++ {
		dp := g.DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
		dp.SetDoubleValue(float64(i))
	}
	return md
}

// Hammer all three signals through their own queues while the viewer is being
// read over RPC. Every writer is a different exporter instance and a different
// queue consumer goroutine, all funnelling into one store -- which is exactly
// the arrangement sharedcomponent used to make impossible. Run under -race.
func TestConcurrentSignalsShareOneStore(t *testing.T) {
	ctx := context.Background()
	set := testExporterSettings(t)
	host, endpoint := startTestExtension(t)
	cfg := createDefaultConfig()

	tExp, err := createTracesExporter(ctx, set, cfg)
	require.NoError(t, err)
	lExp, err := createLogsExporter(ctx, set, cfg)
	require.NoError(t, err)
	mExp, err := createMetricsExporter(ctx, set, cfg)
	require.NoError(t, err)

	for _, e := range []component.Component{tExp, lExp, mExp} {
		require.NoError(t, e.Start(ctx, host))
	}

	const rounds = 20
	var wg sync.WaitGroup
	errs := make(chan error, rounds*3)

	wg.Add(4)
	go func() {
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			if err := tExp.ConsumeTraces(ctx, traceBatch("concurrent", i, 5)); err != nil {
				errs <- err
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			if err := lExp.ConsumeLogs(ctx, logBatch("concurrent", 5)); err != nil {
				errs <- err
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			if err := mExp.ConsumeMetrics(ctx, metricBatch("concurrent", 5)); err != nil {
				errs <- err
			}
		}
	}()
	// Readers run against the same store through the extension's HTTP server,
	// so the RWMutex is exercised for read/write overlap too, not just
	// write/write.
	go func() {
		defer wg.Done()
		body := `{"jsonrpc":"2.0","id":1,"method":"getStats","params":[]}`
		for i := 0; i < rounds; i++ {
			resp, err := http.Post("http://"+endpoint+"/rpc", "application/json",
				bytes.NewBufferString(body))
			if err != nil {
				errs <- err
				continue
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	}()

	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent signal work failed: %v", err)
	}

	for _, e := range []component.Component{tExp, lExp, mExp} {
		require.NoError(t, e.Shutdown(ctx))
	}

	// Shutting the pipelines down must drain their queues into the store
	// before the extension closes it -- the collector shuts extensions down
	// after pipeline components, which is the ordering this relies on.
	resp, err := http.Post("http://"+endpoint+"/rpc", "application/json",
		bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"getStats","params":[]}`))
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	got := string(raw)
	assert.False(t, strings.Contains(got, `"error"`), "stats call errored: %s", got)

	// Assert the work actually landed, not merely that the call succeeded --
	// every batch could be silently dropped and the response would still be
	// well-formed. 20 rounds x 5 spans, and the same for logs and datapoints.
	var stats struct {
		Result struct {
			Traces  struct{ SpanCount int } `json:"traces"`
			Logs    struct{ LogCount int }  `json:"logs"`
			Metrics struct {
				DataPointCount int `json:"dataPointCount"`
			} `json:"metrics"`
		} `json:"result"`
	}
	t.Logf("STATS BODY: %s", got)
	require.NoError(t, json.Unmarshal(raw, &stats), "stats body: %s", got)
	assert.Equal(t, rounds*5, stats.Result.Traces.SpanCount)
	assert.Equal(t, rounds*5, stats.Result.Logs.LogCount)
	assert.Equal(t, rounds*5, stats.Result.Metrics.DataPointCount)
}
