package desktopexporter

import (
	"container/list"
	"context"
	"fmt"
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
		return nil, ErrTraceIDNotFound
	}

	return spans, nil
}

func (store *TraceStore) GetRecentTraces(traceCount int) map[string][]SpanData {
	store.mut.Lock()
	defer store.mut.Unlock()

	recentIDs := store.getRecentTraceIDs(traceCount)
	recentTraces := map[string][]SpanData{}

	for _, traceID := range recentIDs {
		spans, err := store.GetSpansByTraceID(traceID)
		if err != nil {
			fmt.Printf("error: %s\t traceID: %s\n", err, traceID)
		} else {
			recentTraces[traceID] = append(recentTraces[traceID], spans...)
		}
	}

	return recentTraces
}

func (store *TraceStore) getRecentTraceIDs(traceCount int) []string {
	store.mut.Lock()
	defer store.mut.Unlock()

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
			panic(ErrTraceIDMismatch)
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
