package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/marcboeker/go-duckdb/v2"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
)

type Store struct {
	mut  sync.Mutex
	db   *sql.DB
	conn driver.Conn
}

func NewStore(ctx context.Context, dbPath string) *Store {
	if dbPath != "" {
		dbPath = filepath.Clean(dbPath)
	}
	connector, err := duckdb.NewConnector(dbPath, nil)
	if err != nil {
		log.Fatalf("could not initialize new connector: %s", err.Error())
	}

	conn, err := connector.Connect(ctx)
	if err != nil {
		log.Fatalf("could not connect to the database: %s", err.Error())
	}

	db := sql.OpenDB(connector)

	// 1) Create types - ignore "already exists" errors
	if _, err = db.Exec(CREATE_ATTRIBUTE_TYPE); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			log.Fatalf("could not create attribute type: %s", err.Error())
		}
	}

	if _, err = db.Exec(CREATE_EVENT_TYPE); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			log.Printf("could not create event type: %s", err.Error())
		}
	}

	if _, err = db.Exec(CREATE_LINK_TYPE); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			log.Fatalf("could not create link type: %s", err.Error())
		}
	}

	// 2) Create the spans table
	if _, err = db.Exec(CREATE_SPANS_TABLE); err != nil {
		log.Fatalf("could not create table spans: %s", err.Error())
	}

	return &Store{
		mut:  sync.Mutex{},
		db:   db,
		conn: conn,
	}
}

func (s *Store) AddSpans(ctx context.Context, spans []telemetry.SpanData) error {
	s.mut.Lock()
	defer s.mut.Unlock()

	// Prepare the statement once for all spans
	stmt, err := s.db.PrepareContext(ctx, INSERT_SPANS)
	if err != nil {
		return fmt.Errorf("could not prepare statement: %v", err)
	}
	defer stmt.Close()

	for _, span := range spans {
		// Convert attributes and Structs that contain attributes 
		// to JSON format compatible with DUCKDB's UNION type
		attributes := MarshalAttributes(span.Attributes)
		resourceAttrs := MarshalAttributes(span.Resource.Attributes)
		scopeAttrs := MarshalAttributes(span.Scope.Attributes)
		events := MarshalEvents(span.Events)
		links := MarshalLinks(span.Links)

		// log.Printf("attributes: %v", attributes)
		// log.Printf("events: %v", events)
		// log.Printf("links: %v", links)

		_, err := stmt.ExecContext(ctx,
			span.TraceID,
			span.TraceState,
			span.SpanID,
			span.ParentSpanID,
			span.Name,
			span.Kind,
			span.StartTime.Format(time.RFC3339Nano),
			span.EndTime.Format(time.RFC3339Nano),
			attributes,
			events,
			links,
			resourceAttrs,
			span.Resource.DroppedAttributesCount,
			span.Scope.Name,
			span.Scope.Version,
			scopeAttrs,
			span.Scope.DroppedAttributesCount,
			span.DroppedAttributesCount,
			span.DroppedEventsCount,
			span.DroppedLinksCount,
			span.StatusCode,
			span.StatusMessage,
		)
		if err != nil {
			return fmt.Errorf("could not append row to spans: %v", err)
		}
	}
	return nil
}

func (s *Store) GetTrace(ctx context.Context, traceID string) (telemetry.TraceData, error) {
	trace := telemetry.TraceData{
		TraceID: traceID,
		Spans:   []telemetry.SpanData{},
	}

	rows, err := s.db.QueryContext(ctx, SELECT_TRACE, traceID)
	if err != nil {
		return trace, fmt.Errorf("could not retrieve spans: %v", err)
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

		// DuckDB's Go bindings have no support for UNIONs
		// So we are leveraging duckdb's json functionality to fool it into doing the right thing
		var (
			rawAttributes duckdb.Composite[map[string]any]
			rawResourceAttributes duckdb.Composite[map[string]any]
			rawScopeAttributes duckdb.Composite[map[string]any]

			rawEvents duckdb.Composite[[]any]
			rawLinks duckdb.Composite[[]any]
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
			return trace, fmt.Errorf("could not scan spans: %v", err)
		}

		span.Attributes = parseRawAttributes(rawAttributes.Get())
		span.Resource.Attributes = parseRawAttributes(rawResourceAttributes.Get())
		span.Scope.Attributes = parseRawAttributes(rawScopeAttributes.Get())

		span.Events = parseRawEvents(rawEvents.Get())
		span.Links = parseRawLinks(rawLinks.Get())

		trace.Spans = append(trace.Spans, span)
	}

	// Fun thing: db.QueryContext does not return sql.ErrNoRows,
	// but the first call to rows.Next() returns false,
	// so we have to check for traceID not found here.
	if len(trace.Spans) == 0 {
		log.Printf("No spans found for traceID: %s", traceID)
		return trace, telemetry.ErrTraceIDNotFound
	}

	return trace, nil
}

func (s *Store) GetTraceSummaries(ctx context.Context) ([]telemetry.TraceSummary, error) {
	summaries := []telemetry.TraceSummary{}

	rows, err := s.db.QueryContext(ctx, SELECT_TRACE_SUMMARIES)
	if err == sql.ErrNoRows {
		return summaries, nil
	} else if err != nil {
		return nil, fmt.Errorf("could not retrieve trace summaries: %s", err.Error())
	}
	defer rows.Close()

	for rows.Next() {
		var (
			traceID     string
			serviceName sql.NullString
			rootName    sql.NullString
			startTime   sql.NullTime
			endTime     sql.NullTime
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
			return nil, fmt.Errorf("could not scan summary: %s", err.Error())
		}

		var rootSpan *telemetry.RootSpan
		if serviceName.Valid && rootName.Valid && startTime.Valid && endTime.Valid {
			rootSpan = &telemetry.RootSpan{
				ServiceName: serviceName.String,
				Name:        rootName.String,
				StartTime:   startTime.Time,
				EndTime:     endTime.Time,
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

func (s *Store) ClearTraces(ctx context.Context) error {
	s.mut.Lock()
	defer s.mut.Unlock()

	if _, err := s.db.ExecContext(ctx, TRUNCATE_SPANS); err != nil {
		return fmt.Errorf("could not clear traces: %s", err.Error())
	}
	return nil
}

func (s *Store) Close() error {
	s.conn.Close()
	return s.db.Close()
}