package logs_test

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/require"
)

func TestExportIdentity_LogReingestionDoesNotReplaceNativeReference(t *testing.T) {
	s, ctx := storetest.New(t)
	received := otlpLogFixture()
	ingest := func() {
		t.Helper()
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return logs.Ingest(ctx, conn, received)
		}))
	}
	read := func(ref string) json.RawMessage {
		t.Helper()
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return logs.GetLogOTLP(ctx, db, ref)
		})
		require.NoError(t, err)
		return raw
	}
	ingest()
	originalRef := lookupLogRef(t, s, ctx, "primary.event")
	original := read(originalRef)
	replacement := received.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)
	replacement.SetEventName("replacement.event")
	replacement.Body().SetStr("replacement body")
	ingest()
	replacementRef := lookupLogRef(t, s, ctx, "replacement.event")
	require.NotEqual(t, originalRef, replacementRef)
	require.Equal(t, string(original), string(read(originalRef)), "received changes create a new log, not new bytes for the old reference")
	require.Contains(t, string(read(replacementRef)), "replacement body")
}
