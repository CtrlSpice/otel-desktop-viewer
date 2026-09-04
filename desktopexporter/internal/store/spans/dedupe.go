package spans

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
	"github.com/duckdb/duckdb-go/v2"
	"github.com/google/uuid"
)

// ErrSpanAlreadyStored is the reason a span was skipped because the store
// already holds it, or because an earlier span in the same batch claimed the
// same identity.
var ErrSpanAlreadyStored = errors.New("span already stored")

// spanKey identifies one span. A span id is only required to be unique within
// its trace, so the trace is part of the identity rather than context.
type spanKey struct {
	trace duckdb.UUID
	span  uint64
}

// skipAlreadyStored returns the ordinals the store already holds, or which
// repeat an earlier ordinal in this batch. The first occurrence is kept.
//
// Bisection would find these anyway, but only by failing: every row bad means
// 2n-1 transactions, measured at ~1ms per span, so replaying a large capture
// spends minutes discovering one row at a time what one indexed lookup answers
// at once.
func skipAlreadyStored(ctx context.Context, conn driver.Conn, keys []spanKey) (map[int]error, error) {
	if len(keys) == 0 {
		return nil, nil
	}

	stored, err := storedSpans(ctx, conn, keys)
	if err != nil {
		return nil, err
	}

	skip := map[int]error{}
	seen := make(map[spanKey]struct{}, len(keys))
	for ordinal, key := range keys {
		_, repeated := seen[key]
		if _, ok := stored[key]; ok || repeated {
			skip[ordinal] = ErrSpanAlreadyStored
			continue
		}
		seen[key] = struct{}{}
	}
	return skip, nil
}

// storedSpans returns which of keys the spans table already holds. Two bound
// arrays, so the statement text is the same whatever the batch holds, and
// (trace_id, span_id) is the primary key so each probe is an index lookup.
//
// Runs on conn rather than the pool: ingest already holds the store's locks,
// and reaching back through a locking Store method from inside WithConn would
// deadlock.
func storedSpans(ctx context.Context, conn driver.Conn, keys []spanKey) (map[spanKey]struct{}, error) {
	queryer, ok := conn.(driver.QueryerContext)
	if !ok {
		return nil, fmt.Errorf("storedSpans: %w: connection cannot query", ErrSpansStoreInternal)
	}

	traces := make([]string, len(keys))
	spanIDs := make([]uint64, len(keys))
	for i, k := range keys {
		traces[i] = k.trace.String()
		spanIDs[i] = k.span
	}

	// The two arrays are unnested together so they stay paired: a span id is
	// only looked for under the trace it arrived with.
	const q = `select s.trace_id::varchar, s.span_id
		from spans s
		join (select unnest(?::varchar[])::uuid as trace_id,
		             unnest(?::ubigint[]) as span_id) w
			on w.trace_id = s.trace_id and w.span_id = s.span_id`

	traceArg, err := prepareSpanArg(conn, traces)
	if err != nil {
		return nil, fmt.Errorf("storedSpans: %w: %w", ErrSpansStoreInternal, err)
	}
	spanArg, err := prepareSpanArg(conn, spanIDs)
	if err != nil {
		return nil, fmt.Errorf("storedSpans: %w: %w", ErrSpansStoreInternal, err)
	}

	rows, err := queryer.QueryContext(ctx, q, []driver.NamedValue{
		{Ordinal: 1, Value: traceArg},
		{Ordinal: 2, Value: spanArg},
	})
	if err != nil {
		return nil, fmt.Errorf("storedSpans: %w: %w", ErrSpansStoreInternal, err)
	}
	defer rows.Close()

	out := make(map[spanKey]struct{})
	dest := make([]driver.Value, 2)
	for {
		if err := rows.Next(dest); err != nil {
			break
		}
		traceText, ok1 := dest[0].(string)
		spanID, ok2 := dest[1].(uint64)
		if !ok1 || !ok2 {
			continue
		}
		trace, err := parseUUIDText(traceText)
		if err != nil {
			return nil, fmt.Errorf("storedSpans: %w: %w", ErrSpansStoreInternal, err)
		}
		out[spanKey{trace: trace, span: spanID}] = struct{}{}
	}
	return out, nil
}

func parseUUIDText(s string) (duckdb.UUID, error) {
	parsed, err := uuid.Parse(s)
	if err != nil {
		return duckdb.UUID{}, err
	}
	return duckdb.UUID(parsed), nil
}

// prepareSpanArg converts a Go value into something the duckdb driver will bind,
// matching the conversion metrics.Ingest already uses for its array arguments.
func prepareSpanArg(conn driver.Conn, v any) (driver.Value, error) {
	dconn, ok := conn.(*duckdb.Conn)
	if !ok {
		return driver.DefaultParameterConverter.ConvertValue(v)
	}
	nv := driver.NamedValue{Value: v}
	err := dconn.CheckNamedValue(&nv)
	if err == nil {
		return nv.Value, nil
	}
	if !errors.Is(err, driver.ErrSkip) {
		return nil, err
	}
	return driver.DefaultParameterConverter.ConvertValue(v)
}

// traceIDWire renders stored trace UUIDs the way OTLP and the UI do.
func traceIDWire(id duckdb.UUID) string {
	return strings.ReplaceAll(id.String(), "-", "")
}

// recordSpanRejections tallies what this batch was refused, one row per kind.
//
// The sample is the refused span's own id. For an already-stored rejection that
// row is in the store, so it is a working link rather than a copy of something
// we already have.
func recordSpanRejections(ctx context.Context, conn driver.Conn, r ingest.Rejected, keys []spanKey) error {
	if r.Count() == 0 {
		return nil
	}
	records := ingest.Tally("traces", r,
		func(reason error) string {
			if errors.Is(reason, ErrSpanAlreadyStored) {
				return ingest.KindAlreadyStored
			}
			return ingest.KindRefused
		},
		func(ordinal int) (string, string) {
			if ordinal < 0 || ordinal >= len(keys) {
				return "", ""
			}
			// Wire form, matching trace_id_wire / span_id_wire: dash-less
			// hex, the low 16 for a span, which is what the routes expect.
			return traceIDWire(keys[ordinal].trace), util.SpanIDWire(keys[ordinal].span)
		})
	return ingest.RecordRejections(ctx, conn, records)
}
