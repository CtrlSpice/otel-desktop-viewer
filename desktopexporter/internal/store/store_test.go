package store

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
	"github.com/stretchr/testify/assert"
)

type storeTest struct {
	name     string
	dbPath   string
	cleanup  func()
}

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
			store := NewStore(ctx, tt.dbPath)
			assert.NotNil(t, store, "store should not be nil")
			assert.NotNil(t, store.db, "database connection should not be nil")
			assert.NotNil(t, store.conn, "duckdb connection should not be nil")

			// For file-based stores, verify file creation
			if tt.dbPath != "" {
				fileInfo, err := os.Stat(tt.dbPath)
				assert.NoErrorf(t, err, "database file does not exist: %v", err)
				assert.Greater(t, fileInfo.Size(), int64(0), "database file should not be empty")
			}

			// Create sample telemetry
			sample := telemetry.NewSampleTelemetry()

			// Verify tables exist by inserting sample data
			err := store.AddSpans(ctx, sample.Spans)
			assert.NoError(t, err, "spans table should exist and accept sample data")
			
			err = store.AddLogs(ctx, sample.Logs)
			assert.NoError(t, err, "logs table should exist and accept sample data")

			// Verify data was inserted correctly
			summaries, err := store.GetTraceSummaries(ctx)
			assert.NoError(t, err, "should be able to retrieve trace summaries")
			assert.Len(t, summaries, 2, "should have two traces from sample data")

			logs, err := store.GetLogs(ctx)
			assert.NoError(t, err, "should be able to retrieve logs")
			assert.Len(t, logs, 3, "should have three logs from sample data")

			// Test store closure
			err = store.Close()
			assert.NoError(t, err, "store should close without error")

			// Test store reopening
			store = NewStore(ctx, tt.dbPath)
			assert.NotNil(t, store, "store should be reopened successfully")
			assert.NotNil(t, store.db, "database connection should be reestablished")
			assert.NotNil(t, store.conn, "duckdb connection should be reestablished")

			// Verify data after reopening
			summaries, err = store.GetTraceSummaries(ctx)
			assert.NoError(t, err, "should be able to retrieve trace summaries after reopening")
			
			logs, err = store.GetLogs(ctx)
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
			err = store.Close()
			assert.NoErrorf(t, err, "could not close database: %v", err)

			if tt.cleanup != nil {
				tt.cleanup()
			}
		})
	}
}

func TestStoreLifecycleErrors(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ctx, "")
	assert.NotNil(t, store)

	// Test using store after close
	err := store.Close()
	assert.NoError(t, err, "first close should succeed")

	// Try to use the store after closing
	err = store.AddSpans(ctx, []telemetry.SpanData{})
	assert.Error(t, err, "should get error when using closed store")
	assert.True(t, errors.Is(err, ErrStoreConnectionClosed), "error should be ErrStoreConnectionClosed")
	
	// Try to close an already closed store - should be a no-op
	err = store.Close()
	assert.NoError(t, err, "closing an already closed store should be a no-op")

	// Try some other operations on closed store
	_, err = store.GetTraceSummaries(ctx)
	assert.Error(t, err, "should get error when reading from closed store")
	assert.True(t, errors.Is(err, ErrStoreConnectionClosed), "error should be ErrStoreConnectionClosed")

	_, err = store.GetLogs(ctx)
	assert.Error(t, err, "should get error when reading from closed store")
	assert.True(t, errors.Is(err, ErrStoreConnectionClosed), "error should be ErrStoreConnectionClosed")
}



