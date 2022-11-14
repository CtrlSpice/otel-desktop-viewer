package desktopexporter

import (
	"context"
	"strconv"
	"testing"

	"github.com/CtrlSpice/desktop-collector/desktop-exporter/testdata"
	"github.com/stretchr/testify/assert"
)

func TestNewTraceStore(t *testing.T) {
	traceStore := NewTraceStore(10)

	assert.Equal(t, 10, traceStore.maxQueueSize)
	assert.Equal(t, 0, traceStore.traceQueue.Len())
}

func TestAdd(t *testing.T) {
	maxQueueLength := 3
	spansPerTrace := 3

	traces := testdata.GenerateOTLPPayload(1, 1, maxQueueLength*spansPerTrace)

	store := NewTraceStore(maxQueueLength)
	spans := extractSpans(context.Background(), traces)
	ctx := context.Background()

	// Assign each span a TraceID derived from its index before adding it to the store
	// This TraceID is used to validate indexing in the store's traceMap
	for i, span := range spans {
		span.TraceID = strconv.Itoa(i % spansPerTrace)
		store.Add(ctx, span)

		// Verify that the node with the most recently added TraceID
		// Is moved to the front of the queue during the Add operation
		assert.Equal(t, span.TraceID, store.traceQueue.Front().Value)
	}

	// Verify that 3 unique TraceIDs are indexed in the traceMap
	assert.Equal(t, maxQueueLength, len(store.traceMap))

	for traceID, trace := range store.traceMap {
		// Verify that three spans are associaded with each TraceID
		assert.Len(t, trace.Spans, spansPerTrace)

		// Verify that each span has the correct traceID
		for _, span := range trace.Spans {
			assert.Equal(t, traceID, span.TraceID)
		}
	}
}

func TestAddExceedingTraceLimits(t *testing.T) {
	maxQueueLength := 5
	queueOffset := 2

	traces := testdata.GenerateOTLPPayload(1, 1, maxQueueLength+queueOffset)

	store := NewTraceStore(maxQueueLength)
	spans := extractSpans(context.Background(), traces)
	ctx := context.Background()

	// Assign each span a TraceID derived from its index before adding it to the store
	// This TraceID is used to validate queue and dequeue functionality
	for i, span := range spans {
		span.TraceID = strconv.Itoa(i)
		store.Add(ctx, span)

		// Verify that the node with the most recently added TraceID
		// Is moved to the front of the queue during the Add operation
		assert.Equal(t, span.TraceID, store.traceQueue.Front().Value)
	}

	// Verify that the maximum number of unique TraceIDs have been indexed in the traceMap
	assert.Equal(t, maxQueueLength, len(store.traceMap))

	// Verify that the correct number of elements have dropped off the queue
	assert.Equal(t, strconv.Itoa(queueOffset), store.traceQueue.Back().Value)

	// Verify that the traceID values dropped from the traceQueue
	// Are no longer present as indices in the traceMap
	for i := 0; i < queueOffset; i++ {
		assert.NotContains(t, store.traceMap, strconv.Itoa(i))
	}

	// Verify that all the remaining traceIDs are still present
	for i := queueOffset; i < maxQueueLength; i++ {
		assert.Contains(t, store.traceMap, strconv.Itoa(i))
	}
}

func TestGetRecentTraces(t *testing.T) {
	totalTraces := 10
	numRecent := 5
	tracePayload := testdata.GenerateOTLPPayload(1, 1, totalTraces)

	store := NewTraceStore(totalTraces)
	ctx := context.Background()
	spans := extractSpans(ctx, tracePayload)

	// Assign each span a TraceID derived from its index before adding it to the store
	// This TraceID is used to validate the ordering of the slice returned by *TraceStore.GetRecentTraceIDs
	for i, span := range spans {
		span.TraceID = strconv.Itoa(i)
		store.Add(ctx, span)
	}

	recentTraces := store.GetRecentTraces(numRecent)

	// Validate that the number of IDs returned is equal to the lesser value of:
	// - The number of IDs requested or
	// - The number traces available in the store
	if totalTraces < numRecent {
		assert.Len(t, recentTraces, totalTraces)
	} else {
		assert.Len(t, recentTraces, numRecent)
	}

	// Validate the order of the traces based of their ID
	for i, trace := range recentTraces {
		expectedTraceID := strconv.Itoa(totalTraces - (i + 1))
		assert.Equal(t, expectedTraceID, trace.Spans[0].TraceID)
	}
}

func TestGetTrace(t *testing.T) {
	totalTraces := 10
	traces := testdata.GenerateOTLPPayload(1, 1, totalTraces)

	store := NewTraceStore(totalTraces)
	ctx := context.Background()
	spans := extractSpans(ctx, traces)

	// Assign each span a TraceID derived from its index before adding it to the store
	// This TraceID is passed as an argument to test *TraceStore.GetTrace
	for i, span := range spans {
		span.TraceID = strconv.Itoa(i)
		store.Add(ctx, span)
	}

	// Verify that we are able to retrieve every trace in the store by its TraceID
	for i := 0; i < totalTraces; i++ {
		trace, _ := store.GetTrace(strconv.Itoa(i))
		assert.Equal(t, strconv.Itoa(i), trace.Spans[0].TraceID)
	}

	// Verify that looking up an invalid TraceID returns the appropriate error
	_, err := store.GetTrace(strconv.Itoa(-1))
	assert.EqualError(t, err, "traceID not found")
}
