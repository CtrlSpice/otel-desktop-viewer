package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/marcboeker/go-duckdb/v2"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/resource"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/scope"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/traces"
)

type dbEvent struct {
	Name                   string     `db:"name"`
	Timestamp              int64      `db:"timestamp"`
	Attributes             duckdb.Map `db:"attributes"`
	DroppedAttributesCount uint32     `db:"droppedAttributesCount"`
}

type dbLink struct {
	TraceID                string     `db:"traceID"`
	SpanID                 string     `db:"spanID"`
	TraceState             string     `db:"traceState"`
	Attributes             duckdb.Map `db:"attributes"`
	DroppedAttributesCount uint32     `db:"droppedAttributesCount"`
}

// AddSpans appends a list of spans to the store.
func (s *Store) AddSpans(ctx context.Context, spans []traces.SpanData) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf(ErrAddSpans, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	appender, err := duckdb.NewAppender(s.conn, "", "", "spans")
	if err != nil {
		return fmt.Errorf(ErrCreateAppender, err)
	}
	defer appender.Close()

	for i, span := range spans {
		err := appender.AppendRow(
			span.TraceID,
			span.TraceState,
			span.SpanID,
			span.ParentSpanID,
			span.Name,
			span.Kind,
			span.StartTime,
			span.EndTime,
			toDbAttributes(span.Attributes),
			toDbEvents(span.Events),
			toDbLinks(span.Links),
			toDbAttributes(span.Resource.Attributes),
			span.Resource.DroppedAttributesCount,
			span.Scope.Name,
			span.Scope.Version,
			toDbAttributes(span.Scope.Attributes),
			span.Scope.DroppedAttributesCount,
			span.DroppedAttributesCount,
			span.DroppedEventsCount,
			span.DroppedLinksCount,
			span.StatusCode,
			span.StatusMessage,
		)
		if err != nil {
			return fmt.Errorf(ErrAppendRow, err)
		}

		// Flush every 10 spans to prevent buffer overflow
		if (i+1)%10 == 0 {
			err = appender.Flush()
			if err != nil {
				return fmt.Errorf(ErrFlushAppender, err)
			}
		}
	}

	// Final flush for any remaining spans
	err = appender.Flush()
	if err != nil {
		return fmt.Errorf(ErrFlushAppender, err)
	}

	return nil
}

// GetTraceSummaries retrieves a summary for each trace from the store.
func (s *Store) GetTraceSummaries(ctx context.Context) ([]traces.TraceSummary, error) {
	if err := s.checkConnection(); err != nil {
		return nil, fmt.Errorf(ErrGetTraceSummaries, err)
	}

	summaries := []traces.TraceSummary{}

	rows, err := s.db.QueryContext(ctx, SelectTraceSummaries)
	if err != nil {
		return nil, fmt.Errorf(ErrGetTraceSummaries, err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			traceID     string
			serviceName sql.NullString
			rootName    sql.NullString
			startTime   sql.NullInt64
			endTime     sql.NullInt64
			spanCount   int
		)

		if err = rows.Scan(
			&traceID,
			&serviceName,
			&rootName,
			&startTime,
			&endTime,
			&spanCount,
		); err != nil {
			return nil, fmt.Errorf(ErrGetTraceSummaries, err)
		}

		var rootSpan *traces.RootSpan
		if serviceName.Valid && rootName.Valid && startTime.Valid && endTime.Valid {
			rootSpan = &traces.RootSpan{
				ServiceName: serviceName.String,
				Name:        rootName.String,
				StartTime:   startTime.Int64,
				EndTime:     endTime.Int64,
			}
		}

		summaries = append(summaries, traces.TraceSummary{
			TraceID:   traceID,
			RootSpan:  rootSpan,
			SpanCount: uint32(spanCount),
		})
	}
	return summaries, nil
}

// GetTrace retrieves a trace from the store using the traceID.
func (s *Store) GetTrace(ctx context.Context, traceID string) (traces.TraceData, error) {
	if err := s.checkConnection(); err != nil {
		return traces.TraceData{}, fmt.Errorf(ErrGetTrace, traceID, err)
	}

	trace := traces.TraceData{
		TraceID: traceID,
		Spans:   []traces.SpanData{},
	}

	rows, err := s.db.QueryContext(ctx, SelectTrace, traceID)
	if err != nil {
		return trace, fmt.Errorf(ErrGetTrace, traceID, err)
	}
	defer rows.Close()

	for rows.Next() {
		span := traces.SpanData{
			Resource: &resource.ResourceData{
				Attributes:             map[string]any{},
				DroppedAttributesCount: 0,
			},
			Scope: &scope.ScopeData{
				Name:                   "",
				Version:                "",
				Attributes:             map[string]any{},
				DroppedAttributesCount: 0,
			},
		}

		var (
			rawAttributes         duckdb.Composite[map[string]duckdb.Union]
			rawResourceAttributes duckdb.Composite[map[string]duckdb.Union]
			rawScopeAttributes    duckdb.Composite[map[string]duckdb.Union]

			rawEvents duckdb.Composite[[]dbEvent]
			rawLinks  duckdb.Composite[[]dbLink]
		)

		if err = rows.Scan(
			&span.TraceID,
			&span.TraceState,
			&span.SpanID,
			&span.ParentSpanID,
			&span.Name,
			&span.Kind,
			&span.StartTime,
			&span.EndTime,
			&rawAttributes,
			&rawEvents,
			&rawLinks,
			&rawResourceAttributes,
			&span.Resource.DroppedAttributesCount,
			&span.Scope.Name,
			&span.Scope.Version,
			&rawScopeAttributes,
			&span.Scope.DroppedAttributesCount,
			&span.DroppedAttributesCount,
			&span.DroppedEventsCount,
			&span.DroppedLinksCount,
			&span.StatusCode,
			&span.StatusMessage,
		); err != nil {
			return trace, fmt.Errorf(ErrGetTrace, traceID, err)
		}

		span.Attributes = fromDbAttributes(rawAttributes.Get())
		span.Resource.Attributes = fromDbAttributes(rawResourceAttributes.Get())
		span.Scope.Attributes = fromDbAttributes(rawScopeAttributes.Get())

		span.Events = fromDbEvents(rawEvents.Get())
		span.Links = fromDbLinks(rawLinks.Get())

		trace.Spans = append(trace.Spans, span)
	}

	// Fun thing: db.QueryContext does not return sql.ErrNoRows,
	// but the first call to rows.Next() returns false,
	// so we have to check for traceID not found here.
	if len(trace.Spans) == 0 {
		return trace, fmt.Errorf(ErrGetTrace, traceID, ErrTraceIDNotFound)
	}

	return trace, nil
}

// ClearTraces truncates the spans table.
func (s *Store) ClearTraces(ctx context.Context) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf(ErrClearTraces, err)
	}

	if _, err := s.db.ExecContext(ctx, TruncateSpans); err != nil {
		return fmt.Errorf(ErrClearTraces, err)
	}
	return nil
}

// toDbEvents is a helper function that converts a list of EventData
// to a list of go-duckdb Appender compatible dbEvent structs.
func toDbEvents(events []traces.EventData) []dbEvent {
	dbEvents := make([]dbEvent, len(events))

	for i, event := range events {
		dbEvents[i] = dbEvent{
			Name:                   event.Name,
			Timestamp:              event.Timestamp,
			Attributes:             toDbAttributes(event.Attributes),
			DroppedAttributesCount: event.DroppedAttributesCount,
		}
	}
	return dbEvents
}

// toDbLinks is a helper function that converts a list of LinkData
// to a list of go-duckdb Appender compatible dbLink structs.
func toDbLinks(links []traces.LinkData) []dbLink {
	dbLinks := make([]dbLink, len(links))

	for i, link := range links {
		dbLinks[i] = dbLink{
			TraceID:                link.TraceID,
			SpanID:                 link.SpanID,
			TraceState:             link.TraceState,
			Attributes:             toDbAttributes(link.Attributes),
			DroppedAttributesCount: link.DroppedAttributesCount,
		}
	}
	return dbLinks
}

func fromDbEvents(dbEvents []dbEvent) []traces.EventData {
	events := []traces.EventData{}

	for _, dbEvent := range dbEvents {
		attributes := map[string]any{}
		for k, v := range dbEvent.Attributes {
			if name, ok := k.(string); ok {
				if union, ok := v.(duckdb.Union); ok {
					attributes[name] = union.Value
				}
			}
		}

		event := traces.EventData{
			Name:                   dbEvent.Name,
			Timestamp:              dbEvent.Timestamp,
			Attributes:             attributes,
			DroppedAttributesCount: dbEvent.DroppedAttributesCount,
		}
		events = append(events, event)
	}
	return events
}

func fromDbLinks(dbLinks []dbLink) []traces.LinkData {
	links := []traces.LinkData{}

	for _, dbLink := range dbLinks {
		attributes := map[string]any{}
		for k, v := range dbLink.Attributes {
			if name, ok := k.(string); ok {
				if union, ok := v.(duckdb.Union); ok {
					attributes[name] = union.Value
				}
			}
		}

		link := traces.LinkData{
			TraceID:                dbLink.TraceID,
			SpanID:                 dbLink.SpanID,
			TraceState:             dbLink.TraceState,
			Attributes:             attributes,
			DroppedAttributesCount: dbLink.DroppedAttributesCount,
		}
		links = append(links, link)
	}

	return links
}
