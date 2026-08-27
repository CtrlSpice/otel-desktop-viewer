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

// maxSamples bounds how many refused-span identities one rejection row keeps.
// Enough for the shape to read -- one pair repeating, one trace dominating,
// varied traces from one service -- without a replay loop growing the row.
// The merge_samples macro carries the same bound; they change together.
const maxSamples = 10

// SamplePair is one refused span's identity in OTLP wire form. Both halves,
// because a span id only identifies a span within its trace.
type SamplePair struct {
	TraceID string
	SpanID  string
}

// RejectionRecord is one (signal, kind) tally to write.
type RejectionRecord struct {
	Signal string
	Kind   string
	Count  int
	// The most recently refused spans, newest first, deduped by pair, at most
	// maxSamples. Empty when the refused rows are not in the store.
	Samples []SamplePair
}

// rejectionID is sha256(signal, kind) truncated to a uuid, so the upsert finds
// its row by primary key and the same fault from two senders shares one row.
func rejectionID(signal, kind string) duckdb.UUID {
	sum := sha256.Sum256([]byte(signal + "\x00" + kind))
	var id [16]byte
	copy(id[:], sum[:16])
	return duckdb.UUID(id)
}

// appendSample prepends p, drops an existing copy of it, and trims to
// maxSamples -- newest first, one entry per identity, bounded. Rejected walks
// the batch in order, so the last call holds the most recent occurrence.
func appendSample(samples []SamplePair, p SamplePair) []SamplePair {
	out := make([]SamplePair, 0, min(len(samples)+1, maxSamples))
	out = append(out, p)
	for _, s := range samples {
		if s == p {
			continue
		}
		if len(out) == maxSamples {
			break
		}
		out = append(out, s)
	}
	return out
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

	// The zip and the merge live beside the table as macros
	// (samples_from_arrays, merge_samples), created with the rest of the DDL.
	const q = `insert into ingest_rejections
			(id, signal, kind, samples, first_seen, last_seen, occurrences)
		values (?::uuid, ?, ?, samples_from_arrays(?::varchar[], ?::varchar[]), ?, ?, ?)
		on conflict (id) do update set
			last_seen   = excluded.last_seen,
			occurrences = ingest_rejections.occurrences + excluded.occurrences,
			samples     = merge_samples(excluded.samples, ingest_rejections.samples)`

	now := time.Now().UnixNano()
	for _, r := range records {
		if r.Count <= 0 {
			continue
		}
		traces := make([]string, len(r.Samples))
		spans := make([]string, len(r.Samples))
		for i, p := range r.Samples {
			traces[i], spans[i] = p.TraceID, p.SpanID
		}
		id := rejectionID(r.Signal, r.Kind)
		args := []any{
			id.String(),
			r.Signal, r.Kind, traces, spans, now, now, int64(r.Count),
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

// Tally folds a Rejected into one record per kind, keeping the most recent
// maxSamples refused identities for each, newest first and deduped by pair.
// sampleFor turns an ordinal into a linkable trace and span id in wire form,
// and may return empty strings when the refused row is not in the store.
func Tally(signal string, r Rejected, kindFor func(error) string, sampleFor func(ordinal int) (traceID, spanID string)) []RejectionRecord {
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
		if traceID, spanID := sampleFor(item.Ordinal); spanID != "" {
			rec.Samples = appendSample(rec.Samples, SamplePair{TraceID: traceID, SpanID: spanID})
		}
	}
	out := make([]RejectionRecord, 0, len(order))
	for _, kind := range order {
		out = append(out, *byKind[kind])
	}
	return out
}
