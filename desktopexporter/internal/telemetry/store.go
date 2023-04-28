package telemetry

import (
	"container/list"
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
)

type Store struct {
	maxQueueSize int
	mut          sync.Mutex
	queue        *list.List
	telemetryMap map[string]TelemetryData
}

func NewTelemetryStore(maxQueueSize int) *Store {
	return &Store{
		maxQueueSize: maxQueueSize,
		mut:          sync.Mutex{},
		queue:        list.New(),
		telemetryMap: map[string]TelemetryData{},
	}
}

func (store *Store) AddMetric(_ context.Context, md MetricData) {
	store.mut.Lock()
	defer store.mut.Unlock()
	metricID := "1111111"
	store.enqueueTelemetry(metricID)
	store.telemetryMap[metricID] = TelemetryData{
		ID:     metricID,
		Type:   "metric",
		Metric: md,
	}
}

func generateLogID(log LogData) string {
	h := sha256.New()
	h.Write([]byte(log.Body + log.Timestamp.String() + log.ObservedTimestamp.String()))
	bs := h.Sum(nil)

	return fmt.Sprintf("%x", bs)
}

func (store *Store) AddLog(_ context.Context, ld LogData) {
	store.mut.Lock()
	defer store.mut.Unlock()

	logID := generateLogID(ld)
	store.enqueueTelemetry(logID)
	store.telemetryMap[logID] = TelemetryData{
		ID:   logID,
		Type: "log",
		Log:  ld,
	}
}

func (store *Store) AddSpan(_ context.Context, spanData SpanData) {
	store.mut.Lock()
	defer store.mut.Unlock()

	// Enqueue, then append, as the enqueue process checks if the traceID is already in the map to keep the trace alive
	store.enqueueTelemetry(spanData.TraceID)
	td, traceExists := store.telemetryMap[spanData.TraceID]
	if !traceExists {
		td = TelemetryData{
			ID:   spanData.TraceID,
			Type: "trace",
			Trace: TraceData{
				TraceID: spanData.TraceID,
				Spans:   []SpanData{},
			},
		}
	}
	td.Trace.Spans = append(td.Trace.Spans, spanData)
	store.telemetryMap[spanData.TraceID] = td
}

func (store *Store) GetTelemetry(id string) (TelemetryData, error) {
	store.mut.Lock()
	defer store.mut.Unlock()

	td, traceExists := store.telemetryMap[id]
	if !traceExists {
		return TelemetryData{}, ErrTraceIDNotFound // TODO: return telemetryIDNotFound
	}

	return td, nil
}

func (store *Store) GetRecentTelemetry(traceCount int) []TelemetryData {
	store.mut.Lock()
	defer store.mut.Unlock()

	recentIDs := store.getRecentTelemetryIDs(traceCount)
	recentTraces := make([]TelemetryData, 0, len(recentIDs))

	for _, id := range recentIDs {
		td, ok := store.telemetryMap[id]
		if !ok {
			fmt.Printf("error: %s\t traceID: %s\n", ErrTraceIDNotFound, id)
		} else {
			recentTraces = append(recentTraces, td)
		}
	}

	return recentTraces
}

func (store *Store) Clear() {
	store.mut.Lock()
	defer store.mut.Unlock()

	store.queue = list.New()
	store.telemetryMap = map[string]TelemetryData{}
}

func (store *Store) enqueueTelemetry(id string) {
	// If the traceID is already in the queue, move it to the front of the line
	_, telemetryIDExists := store.telemetryMap[id]
	if telemetryIDExists {
		element := store.findQueueElement(id)
		if element == nil {
			fmt.Println(ErrTraceIDMismatch)
		}

		store.queue.MoveToFront(element)
	} else {
		// If we have exceeded the maximum number of traces we plan to store
		// make room for the trace in the queue by deleting the oldest trace
		for store.queue.Len() >= store.maxQueueSize {
			store.dequeueTrace()
		}
		// Add traceID to the front of the queue with the most recent traceIDs
		store.queue.PushFront(id)
	}
}

func (store *Store) dequeueTrace() {
	expiringTraceID := store.queue.Back().Value.(string)
	delete(store.telemetryMap, expiringTraceID)
	store.queue.Remove(store.queue.Back())
}

func (store *Store) findQueueElement(traceID string) *list.Element {
	for element := store.queue.Front(); element != nil; element = element.Next() {
		if traceID == element.Value.(string) {
			return element
		}
	}
	return nil
}

func (store *Store) getRecentTelemetryIDs(traceCount int) []string {
	if traceCount > store.queue.Len() {
		traceCount = store.queue.Len()
	}

	recentTraceIDs := make([]string, 0, traceCount)
	element := store.queue.Front()

	for i := 0; i < traceCount; i++ {
		recentTraceIDs = append(recentTraceIDs, element.Value.(string))
		element = element.Next()
	}

	return recentTraceIDs
}
