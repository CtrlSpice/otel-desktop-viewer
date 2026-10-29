package logs

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/arrowread"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/timerange"
)

func SearchArrow(ctx context.Context, db *sql.DB, timeRange timerange.TimeRange, criteria any, options search.ResultOptions) (*arrowread.Result, error) {
	query, err := logResultsSQL(timeRange, criteria, options, nil, nil, true)
	if err != nil {
		return nil, err
	}
	return arrowread.Read(ctx, db, query.sql, query.args...)
}

// Each attribute key is a bound constant: DuckDB Variant extraction does not
// accept a different key for each input row. Field expressions come only from
// this closed set, never from a requested SQL identifier.
func columnPreviews(columns []Column, args []any) (string, []any, error) {
	if err := ValidateColumns(columns); err != nil {
		return "", nil, err
	}
	var expressions []string
	for _, c := range columns {
		prefix := "l"
		if c.Source != "log" {
			prefix += "." + c.Source
		}
		var value, present string
		if c.Kind == "attribute" {
			present = "variant_exists(" + prefix + ".attributes, ?::varchar)"
			value = prefix + ".attributes[?::varchar]"
		} else {
			switch c.Key {
			case "timeUnixNano":
				value = "l.timestamp::varchar"
			case "observedTimeUnixNano":
				value = "l.observed_timestamp::varchar"
			case "severityText":
				value = "l.severity_text"
			case "severityNumber":
				value = "l.severity_number"
			case "traceId":
				value = "trace_id_wire(l.trace_id)"
			case "spanId":
				value = "span_id_wire(l.span_id)"
			case "eventName":
				value = "l.event_name"
			case "body":
				value = "l.body"
			case "flags":
				value = "l.flags::bigint"
			case "droppedAttributesCount":
				value = prefix + ".dropped_attributes_count::bigint"
			case "schemaUrl":
				value = prefix + ".schema_url"
			case "name":
				value = "l.scope.name"
			case "version":
				value = "l.scope.version"
			default:
				return "", nil, fmt.Errorf("unsupported log field: %w", ErrInvalidLogQuery)
			}
			present = "(" + value + ") is not null"
			value = "(" + value + ")::variant"
		}
		expressions = append(expressions, "struct_pack(source := ?::varchar, kind := ?::varchar, key := ?::varchar, present := "+present+", valueType := otel_value_kind("+value+"), preview := native_column_preview("+value+"))")
		args = append(args, c.Source, c.Kind, c.Key)
		if c.Kind == "attribute" {
			args = append(args, c.Key, c.Key, c.Key)
		}
	}
	return strings.Join(expressions, ",\n"), args, nil
}

func PageArrow(ctx context.Context, db *sql.DB, timeRange timerange.TimeRange, criteria any, options search.ResultOptions, page PageOptions) (*arrowread.Result, error) {
	if page.Size < 1 || page.Size > 250 || page.Offset < 0 {
		return nil, fmt.Errorf("page size must be 1..250 and offset must be non-negative: %w", ErrInvalidLogLimit)
	}
	if err := ValidateColumns(page.Columns); err != nil {
		return nil, err
	}
	query, err := logResultsSQL(timeRange, criteria, options, &page, nil, true)
	if err != nil {
		return nil, err
	}
	return arrowread.Read(ctx, db, query.sql, query.args...)
}

func GetArrow(ctx context.Context, db *sql.DB, logRef string) (*arrowread.Result, error) {
	query, err := queries.RenderArrow(queries.GetLog, nil)
	if err != nil {
		return nil, err
	}
	result, err := arrowread.Read(ctx, db, query, logRef)
	if err == nil && result.NumRows() == 0 {
		result.Close()
		return nil, ErrLogRefNotFound
	}
	return result, err
}

func TraceSummariesArrow(ctx context.Context, db *sql.DB, traceID string) (*arrowread.Result, error) {
	query, err := queries.RenderArrow(queries.GetTraceLogSummaries, nil)
	if err != nil {
		return nil, err
	}
	return arrowread.Read(ctx, db, query, traceID)
}

func SpanLogsArrow(ctx context.Context, db *sql.DB, traceID string, spanID uint64) (*arrowread.Result, error) {
	query, err := queries.RenderArrow(queries.GetSpanLogs, nil)
	if err != nil {
		return nil, err
	}
	return arrowread.Read(ctx, db, query, traceID, []uint64{spanID})
}
