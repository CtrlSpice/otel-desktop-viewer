package store

import (
	"container/list"
	"context"
	"fmt"
	"sync"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
)

type Store struct {
	maxQueueSize int
	mut          sync.Mutex
	traceQueue   *list.List
	traceMap     map[string]telemetry.TraceData
}

func NewStore(maxQueueSize int) *Store {
	return &Store{
		maxQueueSize: maxQueueSize,
		mut:          sync.Mutex{},
		traceQueue:   list.New(),
		traceMap:     map[string]telemetry.TraceData{},
	}
}

func (store *Store) Add(_ context.Context, spanData telemetry.SpanData) {
	store.mut.Lock()
	defer store.mut.Unlock()

	// Enqueue, then append, as the enqueue process checks if the traceID is already in the map to keep the trace alive
	store.enqueueTrace(spanData.TraceID)
	traceData, traceExists := store.traceMap[spanData.TraceID]
	if !traceExists {
		traceData = telemetry.TraceData{
			TraceID: spanData.TraceID,
			Spans:   []telemetry.SpanData{},
		}
	}
	traceData.Spans = append(traceData.Spans, spanData)
	store.traceMap[spanData.TraceID] = traceData
}

func (store *Store) GetTrace(traceID string) (telemetry.TraceData, error) {
	store.mut.Lock()
	defer store.mut.Unlock()

	trace, traceExists := store.traceMap[traceID]
	if !traceExists {
		return telemetry.TraceData{}, telemetry.ErrTraceIDNotFound
	}

	return trace, nil
}

func (store *Store) GetRecentTraces(traceCount int) []telemetry.TraceData {
	store.mut.Lock()
	defer store.mut.Unlock()

	recentIDs := store.getRecentTraceIDs(traceCount)
	recentTraces := make([]telemetry.TraceData, 0, len(recentIDs))

	for _, traceID := range recentIDs {
		trace, traceExists := store.traceMap[traceID]
		if !traceExists {
			fmt.Printf("error: %s\t traceID: %s\n", telemetry.ErrTraceIDNotFound, traceID)
		} else {
			recentTraces = append(recentTraces, trace)
		}
	}

	return recentTraces
}

func (store *Store) ClearTraces() {
	store.mut.Lock()
	defer store.mut.Unlock()

	store.traceQueue = list.New()
	store.traceMap = map[string]telemetry.TraceData{}
}

func (store *Store) enqueueTrace(traceID string) {
	// If the traceID is already in the queue, move it to the front of the line
	_, traceIDExists := store.traceMap[traceID]
	if traceIDExists {
		element := store.findQueueElement(traceID)
		if element == nil {
			fmt.Println(telemetry.ErrTraceIDMismatch)
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

func (store *Store) dequeueTrace() {
	expiringTraceID := store.traceQueue.Back().Value.(string)
	delete(store.traceMap, expiringTraceID)
	store.traceQueue.Remove(store.traceQueue.Back())
}

func (store *Store) findQueueElement(traceID string) *list.Element {
	for element := store.traceQueue.Front(); element != nil; element = element.Next() {
		if traceID == element.Value.(string) {
			return element
		}
	}
	return nil
}

func (store *Store) getRecentTraceIDs(traceCount int) []string {
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
