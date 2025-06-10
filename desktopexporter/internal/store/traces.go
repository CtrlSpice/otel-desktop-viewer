package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/marcboeker/go-duckdb/v2"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
)

// AddSpans appends a list of spans to the store.
func (s *Store) AddSpans(ctx context.Context, spans []telemetry.SpanData) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf(ErrAddSpans, err)
	}

	appender, err := duckdb.NewAppender(s.conn, "", "", "spans")
	if err != nil {
		return fmt.Errorf(ErrCreateAppender, err)
	}
	defer appender.Close()

	for _, span := range spans {
		err := appender.AppendRow(
			span.TraceID,
			span.TraceState,
			span.SpanID,
			span.ParentSpanID,
			span.Name,
			span.Kind,
			span.StartTime,
			span.EndTime,
			toDbMap(span.Attributes),
			toDbEvents(span.Events),
			toDbLinks(span.Links),
			toDbMap(span.Resource.Attributes),
			span.Resource.DroppedAttributesCount,
			span.Scope.Name,
			span.Scope.Version,
			toDbMap(span.Scope.Attributes),
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
	}
	err = appender.Flush()
	if err != nil {
		return fmt.Errorf(ErrFlushAppender, err)
	}
	return nil
}

// GetTraceSummaries retrieves a summary for each trace from the store.
func (s *Store) GetTraceSummaries(ctx context.Context) ([]telemetry.TraceSummary, error) {
	if err := s.checkConnection(); err != nil {
		return nil, fmt.Errorf(ErrGetTraceSummaries, err)
	}

	summaries := []telemetry.TraceSummary{}

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

		var rootSpan *telemetry.RootSpan
		if serviceName.Valid && rootName.Valid && startTime.Valid && endTime.Valid {
			rootSpan = &telemetry.RootSpan{
				ServiceName: serviceName.String,
				Name:        rootName.String,
				StartTime:   startTime.Int64,
				EndTime:     endTime.Int64,
			}
		}

		summaries = append(summaries, telemetry.TraceSummary{
			TraceID:   traceID,
			RootSpan:  rootSpan,
			SpanCount: uint32(spanCount),
		})
	}
	return summaries, nil
}

// GetTrace retrieves a trace from the store using the traceID.
func (s *Store) GetTrace(ctx context.Context, traceID string) (telemetry.TraceData, error) {
	if err := s.checkConnection(); err != nil {
		return telemetry.TraceData{}, fmt.Errorf(ErrGetTrace, traceID, err)
	}

	trace := telemetry.TraceData{
		TraceID: traceID,
		Spans:   []telemetry.SpanData{},
	}

	rows, err := s.db.QueryContext(ctx, SelectTrace, traceID)
	if err != nil {
		return trace, fmt.Errorf(ErrGetTrace, traceID, err)
	}
	defer rows.Close()

	for rows.Next() {
		span := telemetry.SpanData{
			Resource: &telemetry.ResourceData{
				Attributes:             map[string]any{},
				DroppedAttributesCount: 0,
			},
			Scope: &telemetry.ScopeData{
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

		span.Attributes = fromDbMap(rawAttributes.Get())
		span.Resource.Attributes = fromDbMap(rawResourceAttributes.Get())
		span.Scope.Attributes = fromDbMap(rawScopeAttributes.Get())

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