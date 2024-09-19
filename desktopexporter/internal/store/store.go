package store

import (
	"container/list"
	"context"
	"fmt"
	"sync"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
)

type Store struct {
	MaxQueueSize int
	mut          sync.Mutex
	TraceQueue   *list.List
	TraceMap     map[string]telemetry.TraceData
}

func NewStore(maxQueueSize int) *Store {
	return &Store{
		MaxQueueSize: maxQueueSize,
		mut:          sync.Mutex{},
		TraceQueue:   list.New(),
		TraceMap:     map[string]telemetry.TraceData{},
	}
}

func (store *Store) Add(_ context.Context, spanData telemetry.SpanData) {
	store.mut.Lock()
	defer store.mut.Unlock()

	// Enqueue, then append, as the enqueue process checks if the traceID is already in the map to keep the trace alive
	store.enqueueTrace(spanData.TraceID)
	traceData, traceExists := store.TraceMap[spanData.TraceID]
	if !traceExists {
		traceData = telemetry.TraceData{
			TraceID: spanData.TraceID,
			Spans:   []telemetry.SpanData{},
		}
	}
	traceData.Spans = append(traceData.Spans, spanData)
	store.TraceMap[spanData.TraceID] = traceData
}

func (store *Store) GetTrace(traceID string) (telemetry.TraceData, error) {
	store.mut.Lock()
	defer store.mut.Unlock()

	trace, traceExists := store.TraceMap[traceID]
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
		trace, traceExists := store.TraceMap[traceID]
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

	store.TraceQueue = list.New()
	store.TraceMap = map[string]telemetry.TraceData{}
}

func (store *Store) enqueueTrace(traceID string) {
	// If the traceID is already in the queue, move it to the front of the line
	_, traceIDExists := store.TraceMap[traceID]
	if traceIDExists {
		element := store.findQueueElement(traceID)
		if element == nil {
			fmt.Println(telemetry.ErrTraceIDMismatch)
		}

		store.TraceQueue.MoveToFront(element)
	} else {
		// If we have exceeded the maximum number of traces we plan to store
		// make room for the trace in the queue by deleting the oldest trace
		for store.TraceQueue.Len() >= store.MaxQueueSize {
			store.dequeueTrace()
		}
		// Add traceID to the front of the queue with the most recent traceIDs
		store.TraceQueue.PushFront(traceID)
	}
}

func (store *Store) dequeueTrace() {
	expiringTraceID := store.TraceQueue.Back().Value.(string)
	delete(store.TraceMap, expiringTraceID)
	store.TraceQueue.Remove(store.TraceQueue.Back())
}

func (store *Store) findQueueElement(traceID string) *list.Element {
	for element := store.TraceQueue.Front(); element != nil; element = element.Next() {
		if traceID == element.Value.(string) {
			return element
		}
	}
	return nil
}

func (store *Store) getRecentTraceIDs(traceCount int) []string {
	if traceCount > store.TraceQueue.Len() {
		traceCount = store.TraceQueue.Len()
	}

	recentTraceIDs := make([]string, 0, traceCount)
	element := store.TraceQueue.Front()

	for i := 0; i < traceCount; i++ {
		recentTraceIDs = append(recentTraceIDs, element.Value.(string))
		element = element.Next()
	}

	return recentTraceIDs
}
