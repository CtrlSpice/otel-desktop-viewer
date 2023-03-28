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
	traceMap     map[string]TraceData
}

func NewTraceStore(maxQueueSize int) *TraceStore {
	return &TraceStore{
		maxQueueSize: maxQueueSize,
		mut:          sync.Mutex{},
		traceQueue:   list.New(),
		traceMap:     map[string]TraceData{},
	}
}

func (store *TraceStore) Add(_ context.Context, spanData SpanData) {
	store.mut.Lock()
	defer store.mut.Unlock()

	// Enqueue, then append, as the enqueue process checks if the traceID is already in the map to keep the trace alive
	store.enqueueTrace(spanData.TraceID)
	traceData, traceExists := store.traceMap[spanData.TraceID]
	if !traceExists {
		traceData = TraceData{
			TraceID: spanData.TraceID,
			Spans: []SpanData{},
		}
	}
	traceData.Spans = append(traceData.Spans, spanData)
	store.traceMap[spanData.TraceID] = traceData
}

func (store *TraceStore) GetTrace(traceID string) (TraceData, error) {
	store.mut.Lock()
	defer store.mut.Unlock()

	trace, traceExists := store.traceMap[traceID]
	if !traceExists {
		return TraceData{}, ErrTraceIDNotFound
	}

	return trace, nil
}

func (store *TraceStore) GetRecentTraces(traceCount int) []TraceData {
	store.mut.Lock()
	defer store.mut.Unlock()

	recentIDs := store.getRecentTraceIDs(traceCount)
	recentTraces := make([]TraceData, 0, len(recentIDs))

	for _, traceID := range recentIDs {
		trace, traceExists := store.traceMap[traceID]
		if !traceExists {
			fmt.Printf("error: %s\t traceID: %s\n", ErrTraceIDNotFound, traceID)
		} else {
			recentTraces = append(recentTraces, trace)
		}
	}

	return recentTraces
}

func (store *TraceStore) ClearTraces(){
	store.mut.Lock()
	defer store.mut.Unlock()

	store.traceQueue = list.New()
	store.traceMap = map[string]TraceData{}
}

func (store *TraceStore) enqueueTrace(traceID string) {
	// If the traceID is already in the queue, move it to the front of the line
	_, traceIDExists := store.traceMap[traceID]
	if traceIDExists {
		element := store.findQueueElement(traceID)
		if element == nil {
			fmt.Println(ErrTraceIDMismatch)
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
