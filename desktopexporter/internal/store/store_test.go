package store

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/traces"
	"github.com/stretchr/testify/assert"
)

type storeTest struct {
	name    string
	dbPath  string
	cleanup func()
}

// TestHelper holds common test dependencies
type TestHelper struct {
	T     *testing.T
	Ctx   context.Context
	Store *Store
}

// SetupTest creates a new test helper and returns a teardown function
func SetupTest(t *testing.T) (*TestHelper, func()) {
	ctx := context.Background()
	store := NewStore(ctx, "")

	assert.NotNil(t, store, "store should not be nil")

	helper := &TestHelper{
		T:     t,
		Ctx:   ctx,
		Store: store,
	}

	teardown := func() {
		if helper.Store != nil {
			helper.Store.Close()
		}
	}

	return helper, teardown
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

			// Create sample telemetry
			sample := telemetry.NewSampleTelemetry()

			// Verify tables exist by inserting sample data
			err := s.AddSpans(ctx, sample.Spans)
			assert.NoError(t, err, "spans table should exist and accept sample data")

			err = s.AddLogs(ctx, sample.Logs)
			assert.NoError(t, err, "logs table should exist and accept sample data")

			// Verify data was inserted correctly
			summaries, err := s.GetTraceSummaries(ctx)
			assert.NoError(t, err, "should be able to retrieve trace summaries")
			assert.Len(t, summaries, 2, "should have two traces from sample data")

			logs, err := s.GetLogs(ctx)
			assert.NoError(t, err, "should be able to retrieve logs")
			assert.Len(t, logs, 3, "should have three logs from sample data")

			// Test store closure
			err = s.Close()
			assert.NoError(t, err, "store should close without error")

			// Test store reopening
			s = NewStore(ctx, tt.dbPath)
			assert.NotNil(t, s, "store should be reopened successfully")
			assert.NotNil(t, s.db, "database connection should be reestablished")
			assert.NotNil(t, s.conn, "duckdb connection should be reestablished")

			// Verify data after reopening
			summaries, err = s.GetTraceSummaries(ctx)
			assert.NoError(t, err, "should be able to retrieve trace summaries after reopening")

			logs, err = s.GetLogs(ctx)
			assert.NoError(t, err, "should be able to retrieve logs after reopening")

			// Verify persistence behavior
			if tt.dbPath == "" {
				// In-memory store should be empty after reopening
				assert.Len(t, summaries, 0, "in-memory store should be empty after reopening")
				assert.Len(t, logs, 0, "in-memory store should be empty after reopening")
			} else {
				// Persistent store should retain data
				assert.Len(t, summaries, 2, "persistent store should retain traces after reopening")
				assert.Len(t, logs, 3, "persistent store should retain logs after reopening")
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
	err = s.AddSpans(ctx, []traces.SpanData{})
	assert.Error(t, err, "should get error when using closed store")
	assert.True(t, errors.Is(err, ErrStoreConnectionClosed), "error should be ErrStoreConnectionClosed")

	// Try to close an already closed store - should be a no-op
	err = s.Close()
	assert.NoError(t, err, "closing an already closed store should be a no-op")

	// Try some other operations on closed store
	_, err = s.GetTraceSummaries(ctx)
	assert.Error(t, err, "should get error when reading from closed store")
	assert.True(t, errors.Is(err, ErrStoreConnectionClosed), "error should be ErrStoreConnectionClosed")

	_, err = s.GetLogs(ctx)
	assert.Error(t, err, "should get error when reading from closed store")
	assert.True(t, errors.Is(err, ErrStoreConnectionClosed), "error should be ErrStoreConnectionClosed")
}

// TestSampleDataWorkflow tests the complete sample data workflow
func TestSampleDataWorkflow(t *testing.T) {
	helper, teardown := SetupTest(t)
	defer teardown()

	ctx := helper.Ctx
	s := helper.Store

	// Step 1: Check that no sample data exists initially
	exists, err := s.SampleDataExists(ctx)
	assert.NoError(t, err, "SampleDataExists should not return error")
	assert.False(t, exists, "Sample data should not exist initially")

	// Step 2: Load sample data
	sample := telemetry.NewSampleTelemetry()
	err = s.AddSpans(ctx, sample.Spans)
	assert.NoError(t, err, "AddSpans should not return error")

	err = s.AddLogs(ctx, sample.Logs)
	assert.NoError(t, err, "AddLogs should not return error")

	err = s.AddMetrics(ctx, sample.Metrics)
	assert.NoError(t, err, "AddMetrics should not return error")

	// Step 3: Check that sample data now exists
	exists, err = s.SampleDataExists(ctx)
	assert.NoError(t, err, "SampleDataExists should not return error")
	assert.True(t, exists, "Sample data should exist after loading")

	// Step 4: Clear sample data
	err = s.ClearSampleData(ctx)
	assert.NoError(t, err, "ClearSampleData should not return error")

	// Step 5: Check that sample data no longer exists
	exists, err = s.SampleDataExists(ctx)
	assert.NoError(t, err, "SampleDataExists should not return error")
	assert.False(t, exists, "Sample data should not exist after clearing")

	// Step 6: Verify all tables are empty
	spans, err := s.GetTraceSummaries(ctx)
	assert.NoError(t, err, "GetTraceSummaries should not return error")
	assert.Empty(t, spans, "Spans table should be empty after clearing sample data")

	logs, err := s.GetLogs(ctx)
	assert.NoError(t, err, "GetLogs should not return error")
	assert.Empty(t, logs, "Logs table should be empty after clearing sample data")

	metrics, err := s.GetMetrics(ctx)
	assert.NoError(t, err, "GetMetrics should not return error")
	assert.Empty(t, metrics, "Metrics table should be empty after clearing sample data")

	// Step 7: Try to clear sample data again (should be idempotent)
	err = s.ClearSampleData(ctx)
	assert.NoError(t, err, "ClearSampleData should be idempotent")
}
