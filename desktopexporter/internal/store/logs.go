package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"

	"github.com/marcboeker/go-duckdb/v2"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/resource"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/scope"
)

// AddLogs appends a list of logs to the store.
func (s *Store) AddLogs(ctx context.Context, logs []logs.LogData) error {
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
			toDbLogBody(logData.Body),
			toDbAttributes(logData.Resource.Attributes),
			logData.Resource.DroppedAttributesCount,
			logData.Scope.Name,
			logData.Scope.Version,
			toDbAttributes(logData.Scope.Attributes),
			logData.Scope.DroppedAttributesCount,
			toDbAttributes(logData.Attributes),
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
	return nil
}

// GetLog retrieves a log by its ID.
func (s *Store) GetLog(ctx context.Context, logID string) (logs.LogData, error) {
	if err := s.checkConnection(); err != nil {
		return logs.LogData{}, fmt.Errorf(ErrGetLog, logID, err)
	}

	row := s.db.QueryRowContext(ctx, SelectLog, logID)
	logData, err := scanLogRow(row)
	if err != nil {
		return logData, fmt.Errorf(ErrGetLog, logID, err)
	}
	return logData, nil
}

// GetLogs retrieves all logs from the store.
func (s *Store) GetLogs(ctx context.Context) ([]logs.LogData, error) {
	if err := s.checkConnection(); err != nil {
		return nil, fmt.Errorf(ErrGetLogs, err)
	}

	logs := []logs.LogData{}

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
func (s *Store) GetLogsByTraceSpan(ctx context.Context, traceID string, spanID string) ([]logs.LogData, error) {
	if err := s.checkConnection(); err != nil {
		return nil, fmt.Errorf(ErrGetLogsByTraceSpan, traceID, spanID, err)
	}

	logs := []logs.LogData{}

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
func scanLogRow(scanner interface{ Scan(dest ...any) error }) (logs.LogData, error) {
	var (
		rawBody               duckdb.Union
		rawAttributes         duckdb.Composite[map[string]duckdb.Union]
		rawResourceAttributes duckdb.Composite[map[string]duckdb.Union]
		rawScopeAttributes    duckdb.Composite[map[string]duckdb.Union]
	)

	logData := logs.LogData{
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

	logData.Body = fromDbLogBody(rawBody)
	logData.Attributes = fromDbAttributes(rawAttributes.Get())
	logData.Resource.Attributes = fromDbAttributes(rawResourceAttributes.Get())
	logData.Scope.Attributes = fromDbAttributes(rawScopeAttributes.Get())

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
func (s *Store) GetLogsByTrace(ctx context.Context, traceID string) ([]logs.LogData, error) {
	if err := s.checkConnection(); err != nil {
		return nil, fmt.Errorf(ErrGetLogsByTrace, traceID, err)
	}

	logs := []logs.LogData{}

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

// BodyType supports all value types according to semantic conventions:
// - Scalar values: string, boolean, signed 64-bit integer, double
// - Byte array
// - Everything else (arrays, maps, etc.) as JSON

// toDbLogBody converts a log body value to a DuckDB Union type.
// For uint64 values, if they exceed math.MaxInt64, they are converted to strings.
// For complex types (arrays, maps, structs), the value is JSON marshaled.
func toDbLogBody(body any) duckdb.Union {
	switch t := body.(type) {
	case string:
		return duckdb.Union{Tag: "str", Value: t}
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32:
		return duckdb.Union{Tag: "bigint", Value: t}
	case uint64:
		value, hasOverflow := stringifyOnOverflow("body", t)
		if hasOverflow {
			return duckdb.Union{Tag: "str", Value: value}
		}
		return duckdb.Union{Tag: "bigint", Value: value}
	case float32, float64:
		return duckdb.Union{Tag: "double", Value: t}
	case bool:
		return duckdb.Union{Tag: "boolean", Value: t}
	case []byte:
		return duckdb.Union{Tag: "bytes", Value: t}
	default:
		// For complex types (arrays, maps, structs), convert to JSON string
		bodyJson, err := json.Marshal(body)
		if err != nil {
			log.Printf(WarnJSONMarshal, t, body)
			return duckdb.Union{Tag: "str", Value: fmt.Sprintf("%v", body)}
		}
		return duckdb.Union{Tag: "json", Value: string(bodyJson)}
	}
}

func fromDbLogBody(body duckdb.Union) any {
	if body.Tag == "json" {
		var result any
		strValue, ok := body.Value.(string)
		if !ok {
			log.Printf(WarnJSONUnmarshal, fmt.Sprintf(errJSONValueType, body.Value))
			return body.Value
		}

		if err := json.Unmarshal([]byte(strValue), &result); err != nil {
			log.Printf(WarnJSONUnmarshal, err)
			return body.Value
		}

		return result
	}
	return body.Value
}
