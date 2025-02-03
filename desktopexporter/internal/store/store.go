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

	"github.com/marcboeker/go-duckdb"

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

	if _, err = db.Exec(CREATE_ATTRIBUTE_TYPE); err != nil {
		log.Printf("could not create attribute type: %s", err.Error())
	}

	if _, err = db.Exec(CREATE_EVENT_TYPE); err != nil {
		log.Printf("could not create event type: %s", err.Error())
	}

	if _, err = db.Exec(CREATE_LINK_TYPE); err != nil {
		log.Printf("could not create link type: %s", err.Error())
	}

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

	for _, span := range spans {
		// Convert maps to DuckDB MAP format
		attributes := mapToString(span.Attributes)
		resourceAttrs := mapToString(span.Resource.Attributes)
		scopeAttrs := mapToString(span.Scope.Attributes)

		// Convert events to DuckDB ARRAY[STRUCT(...)] format
		events := eventToString(span.Events)

		// Convert links to DuckDB ARRAY[STRUCT(...)] format
		links := linkToString(span.Links)

		query := fmt.Sprintf(`INSERT INTO spans
			VALUES ('%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s', %s, %s, %s, %s, %d, '%s', '%s', %s, %d, %d, %d, %d, '%s', '%s')`,
			escapeString(span.TraceID),
			escapeString(span.TraceState),
			escapeString(span.SpanID),
			escapeString(span.ParentSpanID),
			escapeString(span.Name),
			escapeString(span.Kind),
			span.StartTime.Format(time.RFC3339Nano),
			span.EndTime.Format(time.RFC3339Nano),
			attributes,
			events,
			links,
			resourceAttrs,
			span.Resource.DroppedAttributesCount,
			escapeString(span.Scope.Name),
			escapeString(span.Scope.Version),
			scopeAttrs,
			span.Scope.DroppedAttributesCount,
			span.DroppedAttributesCount,
			span.DroppedEventsCount,
			span.DroppedLinksCount,
			escapeString(span.StatusCode),
			escapeString(span.StatusMessage),
		)

		if _, err := s.db.Exec(query); err != nil {
			return fmt.Errorf("could not append row to spans: %s/n%s", err.Error(), query)
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
		log.Fatalf("could not retrieve spans: %s", err.Error())
	}
	defer rows.Close()

	for rows.Next() {
		span := telemetry.SpanData{}
		span.Resource = &telemetry.ResourceData{
			Attributes:             map[string]interface{}{},
			DroppedAttributesCount: 0,
		}
		span.Scope = &telemetry.ScopeData{
			Name:                   "",
			Version:                "",
			Attributes:             map[string]interface{}{},
			DroppedAttributesCount: 0,
		}

		if err = rows.Scan(
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
		); err != nil {
			return trace, fmt.Errorf("could not scan spans: %s", err.Error())
		}

		trace.Spans = append(trace.Spans, span)
	}

	// Fun thing: db.QueryContext does not return sql.ErrNoRows,
	// but the first call to rows.Next() returns false,
	// so we have to check for traceID not found here.
	if len(trace.Spans) == 0 {
		return trace, telemetry.ErrTraceIDNotFound
	}

	return trace, nil
}

func (s *Store) GetTraceSummaries(ctx context.Context) (*[]telemetry.TraceSummary, error) {
	summaries := []telemetry.TraceSummary{}

	rows, err := s.db.QueryContext(ctx, SELECT_ORDERED_TRACES)
	if err == sql.ErrNoRows {
		return &summaries, nil
	} else if err != nil {
		return nil, fmt.Errorf("could not retrieve trace summaries: %s", err.Error())
	}
	defer rows.Close()

	for rows.Next() {
		summary := telemetry.TraceSummary{
			HasRootSpan:     false,
			RootServiceName: "",
			RootName:        "",
			RootStartTime:   time.Time{},
			RootEndTime:     time.Time{},
			SpanCount:       0,
			TraceID:         "",
		}

		if err = rows.Scan(&summary.TraceID); err != nil {
			return nil, fmt.Errorf("could not scan summary traceID: %s", err.Error())
		}

		spanCountRow := s.db.QueryRowContext(ctx, SELECT_SPAN_COUNT, summary.TraceID)
		if err = spanCountRow.Scan(&summary.SpanCount); err != nil {
			return nil, fmt.Errorf("could not scan summary spanCount: %s", err.Error())
		}

		rootSpanRow := s.db.QueryRowContext(ctx, SELECT_ROOT_SPAN, summary.TraceID)
		err = rootSpanRow.Scan(&summary.RootServiceName, &summary.RootName, &summary.RootStartTime, &summary.RootEndTime)
		if err == nil {
			summary.HasRootSpan = true
			summaries = append(summaries, summary)
		} else if err == sql.ErrNoRows {
			summaries = append(summaries, summary)
		} else {
			return nil, fmt.Errorf("could not retrieve trace summaries: %s", err.Error())
		}
	}
	return &summaries, nil
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

// Helper function to convert Events to DuckDB list of STRUCT string format
func eventToString(events []telemetry.EventData) string {
	eventStrings := []string{}

	for _, event := range events {
		attributes := mapToString(event.Attributes)
		eventStrings = append(eventStrings, fmt.Sprintf("{name: '%s', timestamp: '%v', attributes: %s, droppedAttributesCount: %d}",
			escapeString(event.Name),
			event.Timestamp.Format(time.RFC3339Nano),
			attributes,
			event.DroppedAttributesCount))
	}
	return fmt.Sprintf("[%s]", strings.Join(eventStrings, ", "))
}

// Helper function to convert Links to DuckDB list of STRUCT string format
func linkToString(links []telemetry.LinkData) string {
	linkStrings := []string{}

	for _, link := range links {
		attributes := mapToString(link.Attributes)
		linkStrings = append(linkStrings, fmt.Sprintf(
			"{traceID: '%s', spanID: '%s', traceState: '%s', attributes: %s, droppedAttributesCount: %d}",
			escapeString(link.TraceID),
			escapeString(link.SpanID),
			escapeString(link.TraceState),
			attributes,
			link.DroppedAttributesCount))
	}
	return fmt.Sprintf("[%s]", strings.Join(linkStrings, ", "))
}

// Helper function to convert map to DuckDB MAP string format
func mapToString(m map[string]interface{}) string {
	var pairs []string
	for k, v := range m {
		var valStr string
		switch v := v.(type) {
		case string:
			valStr = fmt.Sprintf("'%s'::attribute", escapeString(v))
		case int, int32, int64:
			valStr = fmt.Sprintf("%d::attribute", v)
		case float32, float64:
			valStr = fmt.Sprintf("%f::attribute", v)
		case bool:
			if v {
				valStr = "true::attribute"
			} else {
				valStr = "false::attribute"
			}
		case []string:
			elements := make([]string, len(v))
			for i, s := range v {
				elements[i] = fmt.Sprintf("'%s'::attribute", escapeString(s))
			}
			valStr = fmt.Sprintf("[%s]", strings.Join(elements, ", "))
		case []int64:
			elements := make([]string, len(v))
			for i, n := range v {
				elements[i] = fmt.Sprintf("%d::attribute", n)
			}
			valStr = fmt.Sprintf("[%s]", strings.Join(elements, ", "))
		case []float64:
			elements := make([]string, len(v))
			for i, f := range v {
				elements[i] = fmt.Sprintf("%f::attribute", f)
			}
			valStr = fmt.Sprintf("[%s]", strings.Join(elements, ", "))
		case []bool:
			elements := make([]string, len(v))
			for i, b := range v {
				if b {
					elements[i] = "true::attribute"
				} else {
					elements[i] = "false::attribute"
				}
			}
			valStr = fmt.Sprintf("[%s]", strings.Join(elements, ", "))
		default:
			valStr = fmt.Sprintf("union_value(str := '%v')", v)
		}
		pairs = append(pairs, fmt.Sprintf("'%s': %v", escapeString(k), valStr))
	}
	return fmt.Sprintf("MAP{%s}", strings.Join(pairs, ", "))
}

// Helper function to escape single quotes in strings for SQL
func escapeString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
