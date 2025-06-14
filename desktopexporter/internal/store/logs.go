package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/marcboeker/go-duckdb/v2"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
)

// AddLogs appends a list of logs to the store.
func (s *Store) AddLogs(ctx context.Context, logs []telemetry.LogData) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf(ErrAddLogs, err)
	}

	appender, err := duckdb.NewAppender(s.conn, "", "", "logs")
	if err != nil {
		return fmt.Errorf(ErrCreateAppender, err)
	}
	defer appender.Close()

	for _, log := range logs {
		err := appender.AppendRow(
			log.ID(),
			log.Timestamp,
			log.ObservedTimestamp,
			log.TraceID,
			log.SpanID,
			log.SeverityText,
			log.SeverityNumber,
			toDbBody(log.Body),
			toDbMap(log.Resource.Attributes),
			log.Resource.DroppedAttributesCount,
			log.Scope.Name,
			log.Scope.Version,
			toDbMap(log.Scope.Attributes),
			log.Scope.DroppedAttributesCount,
			toDbMap(log.Attributes),
			log.DroppedAttributesCount,
			log.Flags,
			log.EventName,
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

// GetLog retrieves a log by its ID.
func (s *Store) GetLog(ctx context.Context, logID string) (telemetry.LogData, error) {
	if err := s.checkConnection(); err != nil {
		return telemetry.LogData{}, fmt.Errorf(ErrGetLog, logID, err)
	}

	row := s.db.QueryRowContext(ctx, SelectLog, logID)
	log, err := scanLogRow(row)
	if err != nil {
		return log, fmt.Errorf(ErrGetLog, logID, err)
	}
	return log, nil
}

// GetLogs retrieves all logs from the store.
func (s *Store) GetLogs(ctx context.Context) ([]telemetry.LogData, error) {
	if err := s.checkConnection(); err != nil {
		return nil, fmt.Errorf(ErrGetLogs, err)
	}

	logs := []telemetry.LogData{}

	rows, err := s.db.QueryContext(ctx, SelectLogs)
	if err != nil {
		return nil, fmt.Errorf(ErrGetLogs, err)
	}
	defer rows.Close()

	for rows.Next() {
		log, err := scanLogRow(rows)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}

	return logs, nil
}

// GetLogsByTraceSpan retrieves all logs for a given trace and span.
func (s *Store) GetLogsByTraceSpan(ctx context.Context, traceID string, spanID string) ([]telemetry.LogData, error) {
	if err := s.checkConnection(); err != nil {
		return nil, fmt.Errorf(ErrGetLogsByTraceSpan, traceID, spanID, err)
	}

	logs := []telemetry.LogData{}

	rows, err := s.db.QueryContext(ctx, SelectLogsByTraceSpan, traceID, spanID)
	if err != nil {
		return nil, fmt.Errorf(ErrGetLogsByTraceSpan, traceID, spanID, err)
	}
	defer rows.Close()

	for rows.Next() {
		log, err := scanLogRow(rows)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, nil
}

// scanLogRow converts a database row into a LogData struct
func scanLogRow(scanner interface{ Scan(dest ...any) error }) (telemetry.LogData, error) {
	var (
		rawBody               duckdb.Union
		rawAttributes         duckdb.Composite[map[string]duckdb.Union]
		rawResourceAttributes duckdb.Composite[map[string]duckdb.Union]
		rawScopeAttributes    duckdb.Composite[map[string]duckdb.Union]
	)

	log := telemetry.LogData{
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

	if err := scanner.Scan(
		&log.Timestamp,
		&log.ObservedTimestamp,
		&log.TraceID,
		&log.SpanID,
		&log.SeverityText,
		&log.SeverityNumber,
		&rawBody,
		&rawResourceAttributes,
		&log.Resource.DroppedAttributesCount,
		&log.Scope.Name,
		&log.Scope.Version,
		&rawScopeAttributes,
		&log.Scope.DroppedAttributesCount,
		&rawAttributes,
		&log.DroppedAttributesCount,
		&log.Flags,
		&log.EventName,
	); err != nil {
		if err == sql.ErrNoRows {
			return log, ErrLogIDNotFound
		}
		return log, fmt.Errorf(ErrScanLogRow, err)
	}

	log.Body = fromDbBody(rawBody)
	log.Attributes = fromDbMap(rawAttributes.Get())
	log.Resource.Attributes = fromDbMap(rawResourceAttributes.Get())
	log.Scope.Attributes = fromDbMap(rawScopeAttributes.Get())

	return log, nil
}

// ClearLogs truncates the logs table.
func (s *Store) ClearLogs(ctx context.Context) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf(ErrClearLogs, err)
	}

	if _, err := s.db.ExecContext(ctx, TruncateLogs); err != nil {
		return fmt.Errorf(ErrClearLogs, err)
	}
	return nil
}

// GetLogsByTrace retrieves all logs for a given trace.
func (s *Store) GetLogsByTrace(ctx context.Context, traceID string) ([]telemetry.LogData, error) {
	if err := s.checkConnection(); err != nil {
		return nil, fmt.Errorf(ErrGetLogsByTrace, traceID, err)
	}

	logs := []telemetry.LogData{}

	rows, err := s.db.QueryContext(ctx, SelectLogsByTrace, traceID)
	if err != nil {
		return nil, fmt.Errorf(ErrGetLogsByTrace, traceID, err)
	}
	defer rows.Close()

	for rows.Next() {
		log, err := scanLogRow(rows)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, nil
}

