package store

import (
	"context"
	"os"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
	"github.com/stretchr/testify/assert"
)

func TestPersistence(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ctx, "./quack.db")

	// Check that db file is created properly
	_, err := os.Stat("./quack.db")
	assert.NoErrorf(t, err, "database file does not exist: %v", err)

	// Add sample spans to the store
	err = store.AddSpans(ctx, telemetry.NewSampleTelemetry().Spans)
	assert.NoErrorf(t, err, "could not add spans to the database: %v", err)

	// Get trace summaries and check length
	summaries, err := store.GetTraceSummaries(ctx)
	if assert.NoErrorf(t, err, "could not get trace summaries: %v", err) {
		assert.Len(t, *summaries, 2)
	}

	// Close store
	err = store.Close()
	assert.NoErrorf(t, err, "could not close database: %v", err)

	// Reopen store from the database file
	store = NewStore(ctx, "./quack.db")

	// Get a trace by ID and check ID of root span
	trace, err := store.GetTrace(ctx, "42957c7c2fca940a0d32a0cdd38c06a4")
	if assert.NoErrorf(t, err, "could not get trace: %v", err) {
		assert.Equal(t, "37fd1349bf83d330", trace.Spans[0].SpanID)
	}

	// Clean up
	err = store.Close()
	assert.NoErrorf(t, err, "could not close database: %v", err)

	err = os.Remove("./quack.db")
	assert.NoError(t, err, "could not remove database file: %v", err)
}
