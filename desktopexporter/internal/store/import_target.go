package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/nested"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
	"github.com/duckdb/duckdb-go/v2"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

var ErrInvalidImportTarget = errors.New("invalid import target")

type ImportTarget struct {
	ID     string `json:"id"`
	SpanID string `json:"spanID,omitempty"`
}

// FindImportTarget looks up one received record without changing the database.
// A nil result means it is not stored yet. An identical existing log can match.
func (s *Store) FindImportTarget(ctx context.Context, signal string, body []byte,
	normalizeLogs func(context.Context, plog.Logs) error,
	normalizeMetrics func(context.Context, pmetric.Metrics) error,
) (*ImportTarget, error) {
	var query string
	var args []any
	target := &ImportTarget{}
	switch signal {
	case "traces":
		data, err := (&ptrace.JSONUnmarshaler{}).UnmarshalTraces(body)
		if err != nil || data.SpanCount() != 1 {
			return nil, fmt.Errorf("%w: expected one span", ErrInvalidImportTarget)
		}
		for _, resource := range data.ResourceSpans().All() {
			for _, scope := range resource.ScopeSpans().All() {
				for _, span := range scope.Spans().All() {
					if span.TraceID().IsEmpty() || span.SpanID().IsEmpty() {
						return nil, fmt.Errorf("%w: missing trace or span ID", ErrInvalidImportTarget)
					}
					target.ID, target.SpanID = span.TraceID().String(), span.SpanID().String()
					query = "SELECT EXISTS(SELECT 1 FROM spans WHERE trace_id = ?::UUID AND span_id = ?::UBIGINT)"
					args = []any{target.ID, strconv.FormatUint(util.SpanIDUint64(span.SpanID()), 10)}
				}
			}
		}
	case "metrics":
		data, err := (&pmetric.JSONUnmarshaler{}).UnmarshalMetrics(body)
		if err != nil || data.MetricCount() != 1 {
			return nil, fmt.Errorf("%w: expected one metric", ErrInvalidImportTarget)
		}
		if err := normalizeMetrics(ctx, data); err != nil {
			return nil, err
		}
		for _, rm := range data.ResourceMetrics().All() {
			resource, err := ingest.ResourceInput(rm.Resource(), rm.SchemaUrl())
			if err != nil {
				return nil, err
			}
			for _, sm := range rm.ScopeMetrics().All() {
				scope, err := ingest.ScopeInput(sm.Scope(), sm.SchemaUrl())
				if err != nil {
					return nil, err
				}
				for _, metric := range sm.Metrics().All() {
					switch metric.Type() {
					case pmetric.MetricTypeGauge, pmetric.MetricTypeSum, pmetric.MetricTypeHistogram, pmetric.MetricTypeExponentialHistogram:
					default:
						return nil, fmt.Errorf("%w: unsupported metric type", ErrInvalidImportTarget)
					}
					id, _, _ := nested.MetricIdentity(metric, resource, scope)
					target.ID = ingest.FormatUUID(id)
					query = "SELECT EXISTS(SELECT 1 FROM metrics WHERE id = ?::UUID)"
					args = []any{target.ID}
				}
			}
		}
	case "logs":
		data, err := (&plog.JSONUnmarshaler{}).UnmarshalLogs(body)
		if err != nil || data.LogRecordCount() != 1 {
			return nil, fmt.Errorf("%w: expected one log", ErrInvalidImportTarget)
		}
		if err := normalizeLogs(ctx, data); err != nil {
			return nil, err
		}
		for _, rl := range data.ResourceLogs().All() {
			resource, err := ingest.ResourceInput(rl.Resource(), rl.SchemaUrl())
			if err != nil {
				return nil, err
			}
			for _, sl := range rl.ScopeLogs().All() {
				scope, err := ingest.ScopeInput(sl.Scope(), sl.SchemaUrl())
				if err != nil {
					return nil, err
				}
				for _, log := range sl.LogRecords().All() {
					body, err := ingest.ValueBytes(log.Body())
					if err != nil {
						return nil, err
					}
					attributes, err := ingest.AttributeBytes(log.Attributes())
					if err != nil {
						return nil, err
					}
					var traceID, spanID any
					if !log.TraceID().IsEmpty() {
						traceID = log.TraceID().String()
					}
					if !log.SpanID().IsEmpty() {
						spanID = strconv.FormatUint(util.SpanIDUint64(log.SpanID()), 10)
					}
					query = `SELECT id::VARCHAR FROM logs
						WHERE timestamp = ?::UBIGINT AND observed_timestamp = ?::UBIGINT
						AND trace_id IS NOT DISTINCT FROM ?::UUID AND span_id IS NOT DISTINCT FROM ?::UBIGINT
						AND severity_number = ? AND severity_text = ? AND event_name = ?
						AND resource.id = ?::UUID AND scope.id = ?::UUID
						AND body IS NOT DISTINCT FROM variant_bytes_to_variant(?)
						AND attributes = variant_bytes_to_variant(?)
						AND dropped_attributes_count = ? AND flags = ? ORDER BY id LIMIT 1`
					args = []any{strconv.FormatUint(uint64(log.Timestamp()), 10), strconv.FormatUint(uint64(log.ObservedTimestamp()), 10), traceID, spanID,
						int32(log.SeverityNumber()), log.SeverityText(), log.EventName(),
						ingest.FormatUUID(resource["id"].(duckdb.UUID)), ingest.FormatUUID(scope["id"].(duckdb.UUID)),
						body, attributes, log.DroppedAttributesCount(), uint32(log.Flags())}
				}
			}
		}
	default:
		return nil, fmt.Errorf("%w: unsupported signal", ErrInvalidImportTarget)
	}
	err := s.WithDBRead(func(db *sql.DB) error {
		if signal == "logs" {
			return db.QueryRowContext(ctx, query, args...).Scan(&target.ID)
		}
		var found bool
		if err := db.QueryRowContext(ctx, query, args...).Scan(&found); err != nil {
			return err
		}
		if !found {
			return sql.ErrNoRows
		}
		return nil
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return target, err
}
