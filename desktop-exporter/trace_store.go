package desktopexporter

import (
	"container/list"
	"context"
	"errors"
	"sync"
)

const (
	MAX_QUEUE_SIZE = 10000
)

type TraceStore struct {
	mut        sync.Mutex
	traceQueue *list.List
	traceMap   map[string][]SpanData
}

func NewTraceStore() *TraceStore {
	return &TraceStore{
		mut:        sync.Mutex{},
		traceQueue: list.New(),
		traceMap:   make(map[string][]SpanData),
	}
}

func (store *TraceStore) Add(_ context.Context, spanData SpanData) {
	store.mut.Lock()
	defer store.mut.Unlock()

	// Enqueue, then append, as the enqueue process checks if the traceID is already in the map to keep the trace alive
	store.enqueueTrace(spanData.TraceID)
	store.traceMap[spanData.TraceID] = append(store.traceMap[spanData.TraceID], spanData)
}

func (store *TraceStore) enqueueTrace(traceID string) {
	// Make room for the trace in the queue if need be
	for store.traceQueue.Len() >= MAX_QUEUE_SIZE {
		store.dequeueTrace()
	}

	// If the traceID is already in the queue, move it to the back of the line
	// Note to future self, who will absolutely forget this:
	// Fifo implementation here means the FRONT element is set to expire first
	_, traceIDExists := store.traceMap[traceID]
	if traceIDExists {
		e := store.findQueueElement(traceID)
		if e == nil {
			panic(errors.New("traceID mismatch between TraceStore.traceMap and TraceStore.traceQueue"))
		}

		store.traceQueue.MoveToBack(e)
	} else {
		//Enqueue traceID
		store.traceQueue.PushBack(traceID)
	}
}

func (store *TraceStore) dequeueTrace() {
	expiringTraceID := store.traceQueue.Front().Value.(string)
	delete(store.traceMap, expiringTraceID)
	store.traceQueue.Remove(store.traceQueue.Front())
}

func (store *TraceStore) findQueueElement(traceID string) *list.Element {
	for e := store.traceQueue.Front(); e != nil; e = e.Next() {
		if traceID == e.Value.(string) {
			return e
		}
	}
	return nil
}
