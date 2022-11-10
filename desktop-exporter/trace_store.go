package desktopexporter

import (
	"container/list"
	"context"
	"errors"
	"sync"
)

type TraceStore struct {
	maxQueueSize int
	mut          sync.Mutex
	traceQueue   *list.List
	traceMap     map[string][]SpanData
}

func NewTraceStore(maxQueueSize int) *TraceStore {
	return &TraceStore{
		maxQueueSize: maxQueueSize,
		mut:          sync.Mutex{},
		traceQueue:   list.New(),
		traceMap:     map[string][]SpanData{},
	}
}

func (store *TraceStore) Add(_ context.Context, spanData SpanData) {
	store.mut.Lock()
	defer store.mut.Unlock()

	// Enqueue, then append, as the enqueue process checks if the traceID is already in the map to keep the trace alive
	store.enqueueTrace(spanData.TraceID)
	store.traceMap[spanData.TraceID] = append(store.traceMap[spanData.TraceID], spanData)
}

func (store *TraceStore) GetSpansByTraceID(traceID string) ([]SpanData, error) {
	store.mut.Lock()
	defer store.mut.Unlock()

	spans, traceIDExists := store.traceMap[traceID]
	if !traceIDExists {
		return nil, errors.New("traceID not found: " + traceID)
	}

	return spans, nil
}

func (store *TraceStore) getRecentTraceIDs(traceCount int) []string {
	if traceCount > store.traceQueue.Len() {
		traceCount = store.traceQueue.Len()
	}

	recentTraceIDs := make([]string, 0, traceCount)
	element := store.traceQueue.Front()

	for i := 0; i < traceCount; i++ {
		recentTraceIDs = append(recentTraceIDs, element.Value.(string))
		element = element.Next()
	}

	return recentTraceIDs
}

func (store *TraceStore) enqueueTrace(traceID string) {
	// If the traceID is already in the queue, move it to the front of the line
	_, traceIDExists := store.traceMap[traceID]
	if traceIDExists {
		element := store.findQueueElement(traceID)
		if element == nil {
			panic(errors.New("traceID mismatch between TraceStore.traceMap and TraceStore.traceQueue"))
		}

		store.traceQueue.MoveToFront(element)
	} else {
		// If we have exceeded the maximum number of traces we plan to store
		// make room for the trace in the queue by deleting the oldest trace
		for store.traceQueue.Len() >= store.maxQueueSize {
			store.dequeueTrace()
		}
		// Add traceID to the front of the queue with the most recent traceIDs
		store.traceQueue.PushFront(traceID)
	}
}

func (store *TraceStore) dequeueTrace() {
	expiringTraceID := store.traceQueue.Back().Value.(string)
	delete(store.traceMap, expiringTraceID)
	store.traceQueue.Remove(store.traceQueue.Back())
}

func (store *TraceStore) findQueueElement(traceID string) *list.Element {
	for element := store.traceQueue.Front(); element != nil; element = element.Next() {
		if traceID == element.Value.(string) {
			return element
		}
	}
	return nil
}

func getTraceDuration(spans []SpanData) (int64, error) {
	if len(spans) < 1 {
		return 0, errors.New("can't calculate trace duration - spans slice is empty")
	}

	// Determine the total duration of the trace
	traceStartTime := spans[0].StartTime
	traceEndTime := spans[0].EndTime
	for i := 1; i < len(spans); i++ {
		if spans[i].StartTime.Before(traceStartTime) {
			traceStartTime = spans[i].StartTime
		}

		if spans[i].EndTime.After(traceEndTime) {
			traceEndTime = spans[i].EndTime
		}
	}
	return traceEndTime.Sub(traceStartTime).Milliseconds(), nil
}
