package spans_test

import (
	"database/sql"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

type rejectionRow struct {
	kind        string
	occurrences int64
	samples     int64
}

func readRejections(t *testing.T, s *store.Store) []rejectionRow {
	t.Helper()
	out, err := readStore(s, func(db *sql.DB) ([]rejectionRow, error) {
		rows, err := db.Query(`select kind, occurrences, len(samples)
			from ingest_rejections order by kind`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var got []rejectionRow
		for rows.Next() {
			var r rejectionRow
			if err := rows.Scan(&r.kind, &r.occurrences, &r.samples); err != nil {
				return nil, err
			}
			got = append(got, r)
		}
		return got, rows.Err()
	})
	require.NoError(t, err)
	return out
}

// A resend records one row saying how many spans it refused, with a span id to
// link to -- not one row per refused span.
func TestRejections_ResendRecordsOneAggregateRow(t *testing.T) {
	t.Parallel()
	s, _ := storetest.New(t)

	_, err := ingestReport(t, s, createTestTracePdata())
	require.NoError(t, err)
	assert.Empty(t, readRejections(t, s), "a clean batch records nothing")

	_, err = ingestReport(t, s, createTestTracePdata())
	require.NoError(t, err)

	got := readRejections(t, s)
	require.Len(t, got, 1, "one row per (signal, kind), not per span")
	assert.Equal(t, "span_already_stored", got[0].kind)
	assert.EqualValues(t, 9, got[0].occurrences)
	assert.EqualValues(t, 9, got[0].samples, "one sample per distinct refused span")
}

// Repeating the resend accumulates into the same row rather than growing the
// table, which is what keeps a replay loop from bloating the database.
func TestRejections_RepeatedResendsAccumulateInOneRow(t *testing.T) {
	t.Parallel()
	s, _ := storetest.New(t)

	for range 4 {
		_, err := ingestReport(t, s, createTestTracePdata())
		require.NoError(t, err)
	}

	got := readRejections(t, s)
	require.Len(t, got, 1, "a replay loop must not grow the table")
	assert.EqualValues(t, 27, got[0].occurrences, "3 resends of 9 spans")
	assert.EqualValues(t, 9, got[0].samples,
		"the same nine identities re-refused must dedupe, not fill the bound")
}

func TestRejections_EmptySpanIDIsRefusedWithoutLinkableSample(t *testing.T) {
	t.Parallel()
	s, _ := storetest.New(t)

	traces := ptrace.NewTraces()
	span := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetTraceID(mustDecodeTraceID("000000000000000000000000000000ac"))

	rep, err := ingestReport(t, s, traces)
	require.NoError(t, err)
	require.ErrorIs(t, rep.Reason(), spans.ErrInvalidSpanID)

	got := readRejections(t, s)
	require.Len(t, got, 1)
	assert.Equal(t, "span_refused", got[0].kind)
	assert.EqualValues(t, 1, got[0].occurrences)
	assert.Zero(t, got[0].samples, "an empty ID must not become an all-zero route")
}
