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

	appender, err := duckdb.NewAppender(s.conn, "", "", "spans")
	if err != nil {
		return fmt.Errorf("could not create appender: %w", err)
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
			return fmt.Errorf("could not append row: %w", err)
		}
	}
	err = appender.Flush()
	if err != nil {
		return fmt.Errorf("could not flush appender: %w", err)
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
			return trace, fmt.Errorf("could not scan spans: %v", err)
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
