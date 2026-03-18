package store

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/schema"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			s, err := NewStore(ctx, tt.dbPath)
			require.NoError(t, err, "store should initialize without error")
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
			err = s.WithConn(func(conn driver.Conn) error {
				return spans.Ingest(ctx, conn, traces)
			})
			assert.NoError(t, err, "spans table should exist and accept data")

			logData := buildStoreTestLogs()
			err = s.WithConn(func(conn driver.Conn) error {
				return logs.Ingest(ctx, conn, logData)
			})
			assert.NoError(t, err, "logs table should exist and accept data")

			metricData := buildStoreTestMetrics()
			err = s.WithConn(func(conn driver.Conn) error {
				return metrics.Ingest(ctx, conn, metricData)
			})
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
			s, err = NewStore(ctx, tt.dbPath)
			require.NoError(t, err, "store should reopen without error")
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

// TestStoreIndexesCreated verifies that all IndexCreationQueries are applied on store init.
func TestStoreIndexesCreated(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "")
	require.NoError(t, err)
	defer s.Close()

	var count int
	err = s.DB().QueryRowContext(ctx, "SELECT count(*) FROM duckdb_indexes() WHERE schema_name = 'main'").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, len(schema.IndexCreationQueries), count, "index count should match IndexCreationQueries")
}

// TestStoreConstraintsEnforced verifies that inline CHECK constraints on the datapoints and
// attributes tables are enforced by the database. It checks that inserting a row that violates
// chk_metric_type_valid is rejected.
func TestStoreConstraintsEnforced(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "")
	require.NoError(t, err)
	defer s.Close()

	// chk_metric_type_valid rejects unknown metric_type values.
	_, err = s.DB().ExecContext(ctx, `
		insert into metrics (id, name, description, unit, resource_dropped_attributes_count,
			scope_name, scope_version, scope_dropped_attributes_count, received)
		values (gen_random_uuid(), 'test', '', '', 0, '', '', 0, 0)
	`)
	require.NoError(t, err, "inserting a metric row should succeed")

	var metricID string
	require.NoError(t, s.DB().QueryRowContext(ctx, "select id from metrics where name = 'test'").Scan(&metricID))

	_, err = s.DB().ExecContext(ctx, `
		insert into datapoints (id, metric_id, metric_type, timestamp, start_time, flags)
		values (gen_random_uuid(), ?, 'InvalidType', 0, 0, 0)
	`, metricID)
	assert.Error(t, err, "inserting a datapoint with invalid metric_type should violate chk_metric_type_valid")
}

// TestStorePersistentReopenIdempotent verifies that reopening a persistent store does not fail
// due to duplicate indexes.
func TestStorePersistentReopenIdempotent(t *testing.T) {
	const dbPath = "./reopen_test.db"
	t.Cleanup(func() { os.Remove(dbPath) })

	ctx := context.Background()

	s, err := NewStore(ctx, dbPath)
	require.NoError(t, err)
	require.NoError(t, s.Close())

	// Reopening must not panic or fatal - constraints use "already exists" guard,
	// indexes use IF NOT EXISTS.
	s2, err := NewStore(ctx, dbPath)
	require.NoError(t, err)

	var indexCount int
	err = s2.DB().QueryRowContext(ctx, "SELECT count(*) FROM duckdb_indexes() WHERE schema_name = 'main'").Scan(&indexCount)
	require.NoError(t, err)
	assert.Equal(t, len(schema.IndexCreationQueries), indexCount)
	require.NoError(t, s2.Close())
}

// TestStoreExponentialHistogramConstraint verifies that the fixed chk_exponential_histogram_fields
// constraint accepts a valid ExponentialHistogram datapoint.
func TestStoreExponentialHistogramConstraint(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "")
	require.NoError(t, err)
	defer s.Close()

	m := pmetric.NewMetrics()
	rm := m.ResourceMetrics().AppendEmpty()
	sm := rm.ScopeMetrics().AppendEmpty()
	metric := sm.Metrics().AppendEmpty()
	metric.SetName("exp_hist_constraint_test")
	exp := metric.SetEmptyExponentialHistogram()
	exp.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	dp := exp.DataPoints().AppendEmpty()
	dp.SetTimestamp(pcommon.Timestamp(time.Now().UnixNano()))
	dp.SetCount(10)
	dp.SetSum(100.0)
	dp.SetMin(5.0)
	dp.SetMax(20.0)
	dp.SetScale(1)
	dp.SetZeroCount(2)
	dp.Positive().SetOffset(0)
	dp.Positive().BucketCounts().FromRaw([]uint64{3, 4, 3})
	dp.Negative().SetOffset(0)
	dp.Negative().BucketCounts().FromRaw([]uint64{1, 2})

	err = s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, m)
	})
	assert.NoError(t, err, "ExponentialHistogram ingest should satisfy chk_exponential_histogram_fields constraint")
}

func TestStoreLifecycleErrors(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "")
	require.NoError(t, err)

	// Test using store after close
	err = s.Close()
	assert.NoError(t, err, "first close should succeed")

	// Try to use the store after closing
	err = s.WithConn(func(conn driver.Conn) error {
		return nil
	})
	assert.Error(t, err, "should get error when using closed store")
	assert.True(t, errors.Is(err, ErrStoreConnectionClosed), "error should be ErrStoreConnectionClosed")

	// Try to close an already closed store - should be a no-op
	err = s.Close()
	assert.NoError(t, err, "closing an already closed store should be a no-op")

	// Try WithConn on a double-closed store
	err = s.WithConn(func(conn driver.Conn) error {
		return nil
	})
	assert.Error(t, err, "should get error when reading from closed store")
	assert.True(t, errors.Is(err, ErrStoreConnectionClosed), "error should be ErrStoreConnectionClosed")

	assert.Nil(t, s.DB(), "DB() should be nil after close")
}
