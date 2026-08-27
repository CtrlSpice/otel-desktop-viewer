package spans

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"

	"github.com/duckdb/duckdb-go/v2"
	"github.com/google/uuid"
)

// ErrSpanAlreadyStored is the reason a span was skipped because its id is
// already in the store, or repeated earlier in the same batch. Span ids are
// required to be unique, so this means the sender is reusing them -- most often
// a replayed capture.
var ErrSpanAlreadyStored = errors.New("span id already stored")

// spanUUID is the span's 8-byte id widened into a uuid, zero-padded high.
func spanUUID(id [8]byte) duckdb.UUID {
	var padded [16]byte
	copy(padded[8:], id[:])
	return duckdb.UUID(padded)
}

// skipAlreadyStored returns the ordinals whose span id the store already holds,
// or which repeat an earlier ordinal in this batch. The first occurrence of a
// repeated id is kept.
//
// Bisection would find these anyway, but only by failing: every row bad means
// 2n-1 transactions, measured at ~1ms per span, so replaying a large capture
// spends minutes discovering one row at a time what one indexed lookup answers
// at once. It also gives a truer reason than a constraint violation surfaced
// from a failed flush.
func skipAlreadyStored(ctx context.Context, conn driver.Conn, ids []duckdb.UUID) (map[int]error, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	stored, err := storedSpanIDs(ctx, conn, ids)
	if err != nil {
		return nil, err
	}

	skip := map[int]error{}
	seen := make(map[duckdb.UUID]struct{}, len(ids))
	for ordinal, id := range ids {
		_, repeated := seen[id]
		if _, ok := stored[id]; ok || repeated {
			skip[ordinal] = ErrSpanAlreadyStored
			continue
		}
		seen[id] = struct{}{}
	}
	return skip, nil
}

// storedSpanIDs returns which of ids the spans table already holds. One bound
// array argument, so the statement text is the same whatever the batch holds,
// and span_id is the primary key so each probe is an index lookup.
//
// Runs on conn rather than the pool: ingest already holds the store's locks,
// and reaching back through a locking Store method from inside WithConn would
// deadlock.
func storedSpanIDs(ctx context.Context, conn driver.Conn, ids []duckdb.UUID) (map[duckdb.UUID]struct{}, error) {
	queryer, ok := conn.(driver.QueryerContext)
	if !ok {
		return nil, fmt.Errorf("storedSpanIDs: %w: connection cannot query", ErrSpansStoreInternal)
	}

	text := make([]string, len(ids))
	for i, id := range ids {
		text[i] = id.String()
	}

	const q = `select span_id::varchar from spans
		where span_id in (select unnest(?::varchar[])::uuid)`

	arg, err := prepareSpanArg(conn, text)
	if err != nil {
		return nil, fmt.Errorf("storedSpanIDs: %w: %w", ErrSpansStoreInternal, err)
	}

	rows, err := queryer.QueryContext(ctx, q, []driver.NamedValue{{Ordinal: 1, Value: arg}})
	if err != nil {
		return nil, fmt.Errorf("storedSpanIDs: %w: %w", ErrSpansStoreInternal, err)
	}
	defer rows.Close()

	out := make(map[duckdb.UUID]struct{})
	dest := make([]driver.Value, 1)
	for {
		if err := rows.Next(dest); err != nil {
			break
		}
		s, ok := dest[0].(string)
		if !ok {
			continue
		}
		parsed, err := uuid.Parse(s)
		if err != nil {
			return nil, fmt.Errorf("storedSpanIDs: %w: %w", ErrSpansStoreInternal, err)
		}
		out[duckdb.UUID(parsed)] = struct{}{}
	}
	return out, nil
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
