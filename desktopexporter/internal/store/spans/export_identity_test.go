package spans_test

import (
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/require"
)

func TestExportIdentity_RetainedSpanIsImmutableButDeletionAllowsReingestion(t *testing.T) {
	s, ctx := storetest.New(t)
	const traceHex = "000000000000000000000000000000a1"
	const traceID = "00000000-0000-0000-0000-0000000000a1"
	read := func() json.RawMessage {
		t.Helper()
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.GetTraceSelectionOTLP(ctx, db, traceID, "1")
		})
		require.NoError(t, err)
		return raw
	}
	_, err := ingestReport(t, s, traceReusingSpanIDs(traceHex, "original"))
	require.NoError(t, err)
	original := read()
	changed := traceReusingSpanIDs(traceHex, "replacement")
	rejected, err := ingestReport(t, s, changed)
	require.NoError(t, err)
	require.Equal(t, 2, rejected.Count())
	require.Equal(t, string(original), string(read()), "resends cannot change retained export bytes")

	// This is the public delete operation, not a direct SQL mutation. The
	// composite identity has no persistent incarnation/version after deletion.
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		return spans.DeleteSpansByTraceIDs(ctx, db, []any{traceID})
	}))
	rejected, err = ingestReport(t, s, changed)
	require.NoError(t, err)
	require.Zero(t, rejected.Count())
	require.NotEqual(t, string(original), string(read()))
	require.Contains(t, string(read()), "replacement-event")
}
