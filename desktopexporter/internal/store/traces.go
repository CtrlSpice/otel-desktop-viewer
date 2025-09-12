package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/resource"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/scope"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/traces"
)

// AddSpans appends a list of spans to the store.
func (s *Store) AddSpans(ctx context.Context, spans []traces.SpanData) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf(ErrAddSpans, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	appender, err := NewAppenderWrapper(s.conn, "", "", "spans")
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
			span.Attributes,
			span.Events,
			span.Links,
			span.Resource.Attributes,
			span.Resource.DroppedAttributesCount,
			span.Scope.Name,
			span.Scope.Version,
			span.Scope.Attributes,
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

func (s *Store) GetTrace(ctx context.Context, traceID string) (*traces.Trace, error) {
	rows, err := s.db.QueryContext(ctx, SelectTrace, traceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get trace: %w", err)
	}
	defer rows.Close()

	var spanNodes []traces.SpanNode
	for rows.Next() {
		spanNode, err := scanTraceRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan trace row: %w", err)
		}
		spanNodes = append(spanNodes, spanNode)
	}

	if len(spanNodes) == 0 {
		return nil, fmt.Errorf(ErrGetTrace, traceID, ErrTraceIDNotFound)
	}

	return &traces.Trace{
		TraceID: traceID,
		Spans:   spanNodes,
	}, nil
}

// scanTraceRow converts a database row into a SpanNode struct
func scanTraceRow(scanner interface{ Scan(dest ...any) error }) (traces.SpanNode, error) {
	span := traces.SpanData{
		Resource: &resource.ResourceData{},
		Scope:    &scope.ScopeData{},
	}

	var depth int

	if err := scanner.Scan(
		&span.TraceID,
		&span.TraceState,
		&span.SpanID,
		&span.ParentSpanID,
		&span.Name,
		&span.Kind,
		&span.StartTime,
		&span.EndTime,
		&span.Attributes,
		&span.Events,
		&span.Links,
		&span.Resource.Attributes,
		&span.Resource.DroppedAttributesCount,
		&span.Scope.Name,
		&span.Scope.Version,
		&span.Scope.Attributes,
		&span.Scope.DroppedAttributesCount,
		&span.DroppedAttributesCount,
		&span.DroppedEventsCount,
		&span.DroppedLinksCount,
		&span.StatusCode,
		&span.StatusMessage,
		&depth,
	); err != nil {
		return traces.SpanNode{}, fmt.Errorf(ErrScanTraceRow, err)
	}

	return traces.SpanNode{
		SpanData: span,
		Depth:    depth,
	}, nil
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

// DeleteSpansByTraceID deletes all spans for a specific trace.
func (s *Store) DeleteSpansByTraceID(ctx context.Context, traceID string) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf(ErrDeleteSpansByTraceID, err)
	}

	_, err := s.db.ExecContext(ctx, DeleteSpansByTraceID, traceID)
	if err != nil {
		return fmt.Errorf(ErrDeleteSpansByTraceID, err)
	}

	return nil
}

// DeleteSpanByID deletes a specific span by its ID.
func (s *Store) DeleteSpanByID(ctx context.Context, spanID string) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf(ErrDeleteSpanByID, err)
	}

	_, err := s.db.ExecContext(ctx, DeleteSpanByID, spanID)
	if err != nil {
		return fmt.Errorf(ErrDeleteSpanByID, err)
	}

	return nil
}

// DeleteSpansByTraceIDs deletes all spans for multiple traces.
func (s *Store) DeleteSpansByTraceIDs(ctx context.Context, traceIDs []any) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf(ErrDeleteSpansByTraceID, err)
	}

	if len(traceIDs) == 0 {
		return nil // Nothing to delete
	}

	placeholders := buildPlaceholders(len(traceIDs))
	query := fmt.Sprintf(DeleteSpansByTraceIDs, placeholders)

	_, err := s.db.ExecContext(ctx, query, traceIDs...)
	if err != nil {
		return fmt.Errorf(ErrDeleteSpansByTraceID, err)
	}

	return nil
}

// DeleteSpansByIDs deletes multiple spans by their IDs.
func (s *Store) DeleteSpansByIDs(ctx context.Context, spanIDs []any) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf(ErrDeleteSpanByID, err)
	}

	if len(spanIDs) == 0 {
		return nil // Nothing to delete
	}

	placeholders := buildPlaceholders(len(spanIDs))
	query := fmt.Sprintf(DeleteSpansByIDs, placeholders)

	_, err := s.db.ExecContext(ctx, query, spanIDs...)
	if err != nil {
		return fmt.Errorf(ErrDeleteSpanByID, err)
	}

	return nil
}
