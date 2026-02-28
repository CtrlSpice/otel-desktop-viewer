package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

type storeTest struct {
	name    string
	dbPath  string
	cleanup func()
}

// TestStore runs a comprehensive suite of tests on the store.
func TestStore(t *testing.T) {
	tests := []storeTest{
		{
			name:   "in-memory store",
			dbPath: "",
		},
		{
			name:   "persistent store",
			dbPath: "./quack.db",
			cleanup: func() {
				os.Remove("./quack.db")
			},
		},
	}

	runStoreTests(t, tests)
}

// buildStoreTestTraces returns ptrace.Traces with two traces for store tests.
func buildStoreTestTraces() ptrace.Traces {
	tr := ptrace.NewTraces()
	base := time.Now().UnixNano()
	// Trace 1
	rs1 := tr.ResourceSpans().AppendEmpty()
	rs1.Resource().Attributes().PutStr("service.name", "svc1")
	ss1 := rs1.ScopeSpans().AppendEmpty()
	s1 := ss1.Spans().AppendEmpty()
	s1.SetTraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})
	s1.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 1})
	s1.SetName("span1")
	s1.SetStartTimestamp(pcommon.Timestamp(base))
	s1.SetEndTimestamp(pcommon.Timestamp(base + 1))
	// Trace 2
	rs2 := tr.ResourceSpans().AppendEmpty()
	rs2.Resource().Attributes().PutStr("service.name", "svc2")
	ss2 := rs2.ScopeSpans().AppendEmpty()
	s2 := ss2.Spans().AppendEmpty()
	s2.SetTraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2})
	s2.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 2})
	s2.SetName("span2")
	s2.SetStartTimestamp(pcommon.Timestamp(base + 100))
	s2.SetEndTimestamp(pcommon.Timestamp(base + 101))
	return tr
}

func buildStoreTestLogs() plog.Logs {
	logs := plog.NewLogs()
	rl := logs.ResourceLogs().AppendEmpty()
	sl := rl.ScopeLogs().AppendEmpty()
	for i := 0; i < 3; i++ {
		rec := sl.LogRecords().AppendEmpty()
		rec.SetTimestamp(pcommon.Timestamp(time.Now().UnixNano()))
		rec.Body().SetStr("log body")
	}
	return logs
}

func buildStoreTestMetrics() pmetric.Metrics {
	metrics := pmetric.NewMetrics()
	rm := metrics.ResourceMetrics().AppendEmpty()
	sm := rm.ScopeMetrics().AppendEmpty()
	m := sm.Metrics().AppendEmpty()
	m.SetName("store_test_metric")
	m.SetUnit("1")
	gauge := m.SetEmptyGauge()
	dp := gauge.DataPoints().AppendEmpty()
	dp.SetTimestamp(pcommon.Timestamp(time.Now().UnixNano()))
	dp.SetDoubleValue(42.0)
	return metrics
}

func runStoreTests(t *testing.T, tests []storeTest) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Test store initialization
			s := NewStore(ctx, tt.dbPath)
			assert.NotNil(t, s, "store should not be nil")
			assert.NotNil(t, s.db, "database connection should not be nil")
			assert.NotNil(t, s.conn, "duckdb connection should not be nil")

			// For file-based stores, verify file creation
			if tt.dbPath != "" {
				fileInfo, err := os.Stat(tt.dbPath)
				assert.NoErrorf(t, err, "database file does not exist: %v", err)
				assert.Greater(t, fileInfo.Size(), int64(0), "database file should not be empty")
			}

			// Ingest two traces, three logs, and one metric via pdata
			traces := buildStoreTestTraces()
			s.Lock()
			err := spans.Ingest(ctx, s.Conn(), traces)
			s.Unlock()
			assert.NoError(t, err, "spans table should exist and accept data")

			logData := buildStoreTestLogs()
			s.Lock()
			err = logs.Ingest(ctx, s.Conn(), logData)
			s.Unlock()
			assert.NoError(t, err, "logs table should exist and accept data")

			metricData := buildStoreTestMetrics()
			s.Lock()
			err = metrics.Ingest(ctx, s.Conn(), metricData)
			s.Unlock()
			assert.NoError(t, err, "metrics table should exist and accept data")

			// Verify data was inserted correctly
			summariesRaw, err := spans.SearchTraces(ctx, s.DB(), 0, 1<<63-1, nil)
			assert.NoError(t, err, "should be able to retrieve trace summaries")
			var summaries []map[string]any
			assert.NoError(t, json.Unmarshal(summariesRaw, &summaries))
			assert.Len(t, summaries, 2, "should have two traces")

			logsRaw, err := logs.Search(ctx, s.DB(), 0, 1<<63-1, nil)
			assert.NoError(t, err, "should be able to retrieve logs")
			var logEntries []any
			assert.NoError(t, json.Unmarshal(logsRaw, &logEntries))
			assert.Len(t, logEntries, 3, "should have three logs")

			metricsRaw, err := metrics.Search(ctx, s.DB(), 0, 1<<63-1, nil)
			assert.NoError(t, err, "should be able to retrieve metrics")
			var metricEntries []any
			assert.NoError(t, json.Unmarshal(metricsRaw, &metricEntries))
			assert.Len(t, metricEntries, 1, "should have one metric")

			// Test store closure
			err = s.Close()
			assert.NoError(t, err, "store should close without error")

			// Test store reopening
			s = NewStore(ctx, tt.dbPath)
			assert.NotNil(t, s, "store should be reopened successfully")
			assert.NotNil(t, s.db, "database connection should be reestablished")
			assert.NotNil(t, s.conn, "duckdb connection should be reestablished")

			// Verify data after reopening
			summariesRaw, err = spans.SearchTraces(ctx, s.DB(), 0, 1<<63-1, nil)
			assert.NoError(t, err, "should be able to retrieve trace summaries after reopening")
			assert.NoError(t, json.Unmarshal(summariesRaw, &summaries))

			logsRaw, err = logs.Search(ctx, s.DB(), 0, 1<<63-1, nil)
			assert.NoError(t, err, "should be able to retrieve logs after reopening")
			assert.NoError(t, json.Unmarshal(logsRaw, &logEntries))

			metricsRaw, err = metrics.Search(ctx, s.DB(), 0, 1<<63-1, nil)
			assert.NoError(t, err, "should be able to retrieve metrics after reopening")
			assert.NoError(t, json.Unmarshal(metricsRaw, &metricEntries))

			// Verify persistence behavior
			if tt.dbPath == "" {
				// In-memory store should be empty after reopening
				assert.Len(t, summaries, 0, "in-memory store should be empty after reopening")
				assert.Len(t, logEntries, 0, "in-memory store should be empty after reopening")
				assert.Len(t, metricEntries, 0, "in-memory store should be empty after reopening")
			} else {
				// Persistent store should retain data
				assert.Len(t, summaries, 2, "persistent store should retain traces after reopening")
				assert.Len(t, logEntries, 3, "persistent store should retain logs after reopening")
				assert.Len(t, metricEntries, 1, "persistent store should retain metrics after reopening")
			}

			// Clean up
			err = s.Close()
			assert.NoErrorf(t, err, "could not close database: %v", err)

			if tt.cleanup != nil {
				tt.cleanup()
			}
		})
	}
}

func TestStoreLifecycleErrors(t *testing.T) {
	ctx := context.Background()
	s := NewStore(ctx, "")
	assert.NotNil(t, s)

	// Test using store after close
	err := s.Close()
	assert.NoError(t, err, "first close should succeed")

	// Try to use the store after closing
	err = s.CheckConnection()
	assert.Error(t, err, "should get error when using closed store")
	assert.True(t, errors.Is(err, ErrStoreConnectionClosed), "error should be ErrStoreConnectionClosed")

	// Try to close an already closed store - should be a no-op
	err = s.Close()
	assert.NoError(t, err, "closing an already closed store should be a no-op")

	// Try some other operations on closed store (callers use CheckConnection first)
	err = s.CheckConnection()
	assert.Error(t, err, "should get error when reading from closed store")
	assert.True(t, errors.Is(err, ErrStoreConnectionClosed), "error should be ErrStoreConnectionClosed")

	// After close, DB() is nil; callers should use CheckConnection() before using store.DB().
	assert.Nil(t, s.DB(), "DB() should be nil after close")
}
