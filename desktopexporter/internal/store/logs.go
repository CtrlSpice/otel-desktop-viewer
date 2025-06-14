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

	s.mu.Lock()
	defer s.mu.Unlock()

	appender, err := duckdb.NewAppender(s.conn, "", "", "logs")
	if err != nil {
		return fmt.Errorf(ErrCreateAppender, err)
	}
	defer appender.Close()

	for i, logData := range logs {
		err := appender.AppendRow(
			logData.ID(),
			logData.Timestamp,
			logData.ObservedTimestamp,
			logData.TraceID,
			logData.SpanID,
			logData.SeverityText,
			logData.SeverityNumber,
			toDbBody(logData.Body),
			toDbMap(logData.Resource.Attributes),
			logData.Resource.DroppedAttributesCount,
			logData.Scope.Name,
			logData.Scope.Version,
			toDbMap(logData.Scope.Attributes),
			logData.Scope.DroppedAttributesCount,
			toDbMap(logData.Attributes),
			logData.DroppedAttributesCount,
			logData.Flags,
			logData.EventName,
		)
		if err != nil {
			return fmt.Errorf(ErrAppendRow, err)
		}

		// Flush every 10 logs to prevent buffer overflow
		if (i+1)%10 == 0 {
			err = appender.Flush()
			if err != nil {
				return fmt.Errorf(ErrFlushAppender, err)
			}
		}
	}
	
	// Final flush for any remaining logs
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
	logData, err := scanLogRow(row)
	if err != nil {
		return logData, fmt.Errorf(ErrGetLog, logID, err)
	}
	return logData, nil
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
		logData, err := scanLogRow(rows)
		if err != nil {
			return nil, err
		}
		logs = append(logs, logData)
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
		logData, err := scanLogRow(rows)
		if err != nil {
			return nil, err
		}
		logs = append(logs, logData)
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

	logData := telemetry.LogData{
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
		&logData.Timestamp,
		&logData.ObservedTimestamp,
		&logData.TraceID,
		&logData.SpanID,
		&logData.SeverityText,
		&logData.SeverityNumber,
		&rawBody,
		&rawResourceAttributes,
		&logData.Resource.DroppedAttributesCount,
		&logData.Scope.Name,
		&logData.Scope.Version,
		&rawScopeAttributes,
		&logData.Scope.DroppedAttributesCount,
		&rawAttributes,
		&logData.DroppedAttributesCount,
		&logData.Flags,
		&logData.EventName,
	); err != nil {
		if err == sql.ErrNoRows {
			return logData, ErrLogIDNotFound
		}
		return logData, fmt.Errorf(ErrScanLogRow, err)
	}

	logData.Body = fromDbBody(rawBody)
	logData.Attributes = fromDbMap(rawAttributes.Get())
	logData.Resource.Attributes = fromDbMap(rawResourceAttributes.Get())
	logData.Scope.Attributes = fromDbMap(rawScopeAttributes.Get())

	return logData, nil
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
		logData, err := scanLogRow(rows)
		if err != nil {
			return nil, err
		}
		logs = append(logs, logData)
	}
	return logs, nil
}

