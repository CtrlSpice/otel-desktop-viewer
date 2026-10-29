package spans

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/arrowread"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
)

// WaterfallColumn identifies a received attribute, not an SQL expression.
// Scalar fields already travel with the tree and do not need preview requests.
type WaterfallColumn struct {
	Source string `json:"source"`
	Key    string `json:"key"`
}

func ValidateWaterfallColumns(columns []WaterfallColumn) error {
	for _, c := range columns {
		if c.Source != "span" && c.Source != "resource" && c.Source != "scope" {
			return fmt.Errorf("invalid waterfall column source: %w", ErrInvalidTraceQuery)
		}
	}
	return nil
}

func waterfallColumnExpressions(columns []WaterfallColumn, args []any, owner string) (string, []any, error) {
	if err := ValidateWaterfallColumns(columns); err != nil {
		return "", nil, err
	}
	var parts []string
	for _, c := range columns {
		prefix := owner
		if c.Source != "span" {
			prefix += "." + c.Source
		}
		source, key := len(args)+1, len(args)+2
		args = append(args, c.Source, c.Key)
		parts = append(parts, fmt.Sprintf(`struct_pack(source := $%d::varchar, key := $%d::varchar,
		"values" := otel_native_attribute_previews((variant_to_parquet_variant(variant_normalize(%s.attributes))::struct(metadata blob, value blob)).metadata, (variant_to_parquet_variant(variant_normalize(%s.attributes))::struct(metadata blob, value blob)).value, $%d::varchar))`, source, key, prefix, prefix, key))
	}
	return strings.Join(parts, ","), args, nil
}

// ColumnPreviewsArrow evaluates every requested column for the complete trace in
// one query. Unselected keys remain in WaterfallArrow's independent catalogue.
func ColumnPreviewsArrow(ctx context.Context, db *sql.DB, traceID string, columns []WaterfallColumn) (*arrowread.Result, error) {
	if err := ValidateWaterfallColumns(columns); err != nil {
		return nil, err
	}
	args := []any{traceID}
	parts := make([]string, 0, len(columns))
	for _, c := range columns {
		prefix := "s"
		if c.Source != "span" {
			prefix += "." + c.Source
		}
		source, key := len(args)+1, len(args)+2
		args = append(args, c.Source, c.Key)
		parts = append(parts, fmt.Sprintf(`select span_id_wire(s.span_id) as spanID, $%d::varchar as source, $%d::varchar as key,
		otel_native_attribute_previews((variant_to_parquet_variant(variant_normalize(%s.attributes))::struct(metadata blob, value blob)).metadata, (variant_to_parquet_variant(variant_normalize(%s.attributes))::struct(metadata blob, value blob)).value, $%d::varchar) as "values"
		from spans s where s.trace_id = try_cast($1 as uuid)`, source, key, prefix, prefix, key))
	}
	if len(parts) == 0 {
		parts = append(parts, `select ''::varchar as spanID, ''::varchar as source, ''::varchar as key,
		[]::struct(valueType varchar, preview varchar)[] as "values" where false and try_cast($1 as uuid) is not null`)
	}
	return arrowread.Read(ctx, db, "select * from ("+strings.Join(parts, " union all ")+") order by spanID, source, key", args...)
}

// GetArrow never resolves a span ID across traces.
func GetArrow(ctx context.Context, db *sql.DB, traceID string, spanID uint64) (*arrowread.Result, error) {
	query, err := queries.RenderArrow(queries.GetSpan, getTraceViewParams{MatchedExpr: "true"})
	if err != nil {
		return nil, err
	}
	return arrowread.Read(ctx, db, query, traceID, []uint64{spanID})
}
