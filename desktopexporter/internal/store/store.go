package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"log"
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

func NewStore(ctx context.Context) *Store {
	connector, err := duckdb.NewConnector("", nil)
	if err != nil {
		log.Fatalf("could not initialize new connector: %s", err.Error())
	}

	conn, err := connector.Connect(ctx)
	if err != nil {
		log.Fatalf("could not connect to the database: %s", err.Error())
	}

	db := sql.OpenDB(connector)
	_, err = db.Exec(ENABLE_JSON)
	if err != nil {
		log.Fatalf("could not enable json: %s", err.Error())
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

func (s *Store) AddSpans(ctx context.Context, spans []telemetry.SpanData) {
	s.mut.Lock()
	defer s.mut.Unlock()

	appender, err := duckdb.NewAppenderFromConn(s.conn, "", "spans")
	if err != nil {
		log.Fatalf("could not create new appender for spans: %s", err.Error())
	}
	defer appender.Close()

	for _, span := range spans {
		attributes, err := json.Marshal(span.Attributes)
		if err != nil {
			log.Fatalf("could not marshal span attributes: %s", err.Error())
		}

		events, err := json.Marshal(span.Events)
		if err != nil {
			log.Fatalf("could not marshal span events: %s", err.Error())
		}

		links, err := json.Marshal(span.Links)
		if err != nil {
			log.Fatalf("could not marshal span links: %s", err.Error())
		}

		resourceAttributes, err := json.Marshal(span.Resource.Attributes)
		if err != nil {
			log.Fatalf("could not marshal resource attributes: %s", err.Error())
		}

		scopeAttributes, err := json.Marshal(span.Scope.Attributes)
		if err != nil {
			log.Fatalf("could not marshal scope attributes: %s", err.Error())
		}

		if err := appender.AppendRow(
			span.TraceID,
			span.TraceState,
			span.SpanID,
			span.ParentSpanID,
			span.Name,
			span.Kind,
			span.StartTime,
			span.EndTime,
			string(attributes),
			string(events),
			string(links),
			string(resourceAttributes),
			span.Resource.DroppedAttributesCount,
			span.Scope.Name,
			span.Scope.Version,
			string(scopeAttributes),
			span.Scope.DroppedAttributesCount,
			span.DroppedAttributesCount,
			span.DroppedEventsCount,
			span.DroppedLinksCount,
			span.StatusCode,
			span.StatusMessage,
		); err != nil {
			log.Fatalf("could not append row to spans: %s", err.Error())
		}
	}
}

func (s *Store) GetTrace(ctx context.Context, traceID string) (telemetry.TraceData, error) {
	trace := telemetry.TraceData{
		TraceID: traceID,
		Spans:   []telemetry.SpanData{},
	}

	rows, err := s.db.QueryContext(ctx, SELECT_TRACE, traceID)
	if err == sql.ErrNoRows {
		return trace, telemetry.ErrTraceIDNotFound
	} else if err != nil {
		log.Fatalf("could not retrieve spans: %s", err.Error())
	}

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

		// Placeholders for JSON
		attrBytes := []byte{}
		evntBytes := []byte{}
		linkBytes := []byte{}
		rAttrBytes := []byte{}
		sAttrBytes := []byte{}

		if err = rows.Scan(
			&span.TraceID,
			&span.TraceState,
			&span.SpanID,
			&span.ParentSpanID,
			&span.Name,
			&span.Kind,
			&span.StartTime,
			&span.EndTime,
			&attrBytes,
			&evntBytes,
			&linkBytes,
			&rAttrBytes,
			&span.Resource.DroppedAttributesCount,
			&span.Scope.Name,
			&span.Scope.Version,
			&sAttrBytes,
			&span.Scope.DroppedAttributesCount,
			&span.DroppedAttributesCount,
			&span.DroppedEventsCount,
			&span.DroppedLinksCount,
			&span.StatusCode,
			&span.StatusMessage,
		); err != nil {
			return trace, fmt.Errorf("could not scan spans: %s", err.Error())
		}

		if err = json.Unmarshal(attrBytes, &span.Attributes); err != nil {
			return trace, fmt.Errorf("could not unmarshal span attributes: %s", err.Error())
		}

		if err = json.Unmarshal(evntBytes, &span.Events); err != nil {
			return trace, fmt.Errorf("could not unmarshal span events: %s", err.Error())
		}

		if err = json.Unmarshal(linkBytes, &span.Links); err != nil {
			return trace, fmt.Errorf("could not unmarshal span links: %s", err.Error())
		}

		if err = json.Unmarshal(rAttrBytes, &span.Resource.Attributes); err != nil {
			return trace, fmt.Errorf("could not unmarshal resource attributes: %s", err.Error())
		}

		if err = json.Unmarshal(sAttrBytes, &span.Scope.Attributes); err != nil {
			return trace, fmt.Errorf("could not unmarshal scope attributes: %s", err.Error())
		}

		trace.Spans = append(trace.Spans, span)
	}
	rows.Close()
	return trace, nil
}

func (s *Store) GetTraceSummaries(ctx context.Context) telemetry.TraceSummaries {
	output := telemetry.TraceSummaries{
		TraceSummaries: []telemetry.TraceSummary{},
	}

	rows, err := s.db.QueryContext(ctx, SELECT_ORDERED_TRACES)
	if err == sql.ErrNoRows {
		rows.Close()
		return output
	} else if err != nil {
		log.Fatalf("could not retrieve trace summaries: %s", err.Error())
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
			log.Fatalf("could not scan summary traceID: %s", err.Error())
		}

		spanCountRow := s.db.QueryRowContext(ctx, SELECT_SPAN_COUNT, summary.TraceID)
		if err = spanCountRow.Scan(&summary.SpanCount); err != nil {
			log.Fatalf("could not scan summary spanCount: %s", err.Error())
		}

		rootSpanRow := s.db.QueryRowContext(ctx, SELECT_ROOT_SPAN, summary.TraceID)
		err = rootSpanRow.Scan(&summary.RootServiceName, &summary.RootName, &summary.RootStartTime, &summary.RootEndTime)
		if err == nil {
			summary.HasRootSpan = true
			output.TraceSummaries = append(output.TraceSummaries, summary)
		} else if err == sql.ErrNoRows {
			output.TraceSummaries = append(output.TraceSummaries, summary)
		} else {
			log.Fatalf("could not retrieve trace summaries: %s", err.Error())
		}
	}
	return output
}

func (s *Store) ClearTraces(ctx context.Context) {
	s.mut.Lock()
	defer s.mut.Unlock()

	if _, err := s.db.ExecContext(ctx, TRUNCATE_SPANS); err != nil {
		log.Fatalf("could not clear traces: %s", err.Error())
	}
}

func (s *Store) Close() error {
	s.conn.Close()
	return s.db.Close()
}
