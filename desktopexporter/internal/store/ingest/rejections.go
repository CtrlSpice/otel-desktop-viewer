package ingest

import (
	"context"
	"crypto/sha256"
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/duckdb/duckdb-go/v2"
)

// Kinds of rejection. The string is stored and shown, so it is part of the
// contract rather than an internal label.
const (
	KindAlreadyStored = "span_already_stored"
	KindRefused       = "span_refused"
)

// RejectionRecord is one (signal, kind) tally to write.
type RejectionRecord struct {
	Signal string
	Kind   string
	Count  int
	// Sample is an id from the most recent occurrence, for the UI to link to.
	// Empty when the refused row is not in the store and nothing can be linked.
	Sample string
}

// rejectionID is sha256(signal, kind) truncated to a uuid, so the upsert finds
// its row by primary key and the same fault from two senders shares one row.
func rejectionID(signal, kind string) duckdb.UUID {
	sum := sha256.Sum256([]byte(signal + "\x00" + kind))
	var id [16]byte
	copy(id[:], sum[:16])
	return duckdb.UUID(id)
}

// RecordRejections tallies refused telemetry, one row per (signal, kind).
//
// Called after the append pass has committed, never inside it: a rejection is a
// fact about a batch that succeeded, and rolling it back with a failed attempt
// would lose the only record that anything was refused.
func RecordRejections(ctx context.Context, conn driver.Conn, records []RejectionRecord) error {
	if len(records) == 0 {
		return nil
	}

	dconn, ok := conn.(*duckdb.Conn)
	if !ok {
		return fmt.Errorf("RecordRejections: %w: connection is not a *duckdb.Conn", ErrIngestInternal)
	}

	const q = `insert into ingest_rejections
			(id, signal, kind, sample, first_seen, last_seen, occurrences)
		values (?::uuid, ?, ?, ?, ?, ?, ?)
		on conflict (id) do update set
			last_seen   = excluded.last_seen,
			occurrences = ingest_rejections.occurrences + excluded.occurrences,
			sample      = coalesce(excluded.sample, ingest_rejections.sample)`

	now := time.Now().UnixNano()
	for _, r := range records {
		if r.Count <= 0 {
			continue
		}
		var sample any
		if r.Sample != "" {
			sample = r.Sample
		}
		id := rejectionID(r.Signal, r.Kind)
		args := []any{
			id.String(),
			r.Signal, r.Kind, sample, now, now, int64(r.Count),
		}
		named, err := prepareNamedValues(dconn, args)
		if err != nil {
			return fmt.Errorf("RecordRejections: %w: %w", ErrIngestInternal, err)
		}
		if _, err := dconn.ExecContext(ctx, q, named); err != nil {
			return fmt.Errorf("RecordRejections: %w: %w", ErrIngestInternal, err)
		}
	}
	return nil
}

// Tally folds a Rejected into one record per kind, carrying the last sample
// seen for each. sampleFor turns an ordinal into a linkable id, and may return
// "" when the refused row is not in the store.
func Tally(signal string, r Rejected, kindFor func(error) string, sampleFor func(ordinal int) string) []RejectionRecord {
	byKind := map[string]*RejectionRecord{}
	order := []string{}
	for _, item := range r.Items {
		kind := kindFor(item.Reason)
		rec, ok := byKind[kind]
		if !ok {
			rec = &RejectionRecord{Signal: signal, Kind: kind}
			byKind[kind] = rec
			order = append(order, kind)
		}
		rec.Count++
		if s := sampleFor(item.Ordinal); s != "" {
			rec.Sample = s
		}
	}
	out := make([]RejectionRecord, 0, len(order))
	for _, kind := range order {
		out = append(out, *byKind[kind])
	}
	return out
}
