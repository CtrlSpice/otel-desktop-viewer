package server

import (
	"bufio"
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestChangesSnapshotsReconnectAndShutdown(t *testing.T) {
	db, err := store.NewStore(t.Context(), "", nil)
	require.NoError(t, err)
	defer db.Close()
	s, err := NewServer("localhost:0", 0, db, zap.NewNop(), nil)
	require.NoError(t, err)
	httpServer := httptest.NewServer(s.server.Handler)
	defer httpServer.Close()
	defer s.Shutdown(context.Background())
	client := &http.Client{Timeout: 5 * time.Second}
	connect := func() (*http.Response, *bufio.Scanner) {
		t.Helper()
		response, err := client.Get(httpServer.URL + "/api/changes")
		require.NoError(t, err)
		require.Equal(t, "text/event-stream", response.Header.Get("Content-Type"))
		return response, bufio.NewScanner(response.Body)
	}
	read := func(scanner *bufio.Scanner) string {
		t.Helper()
		for scanner.Scan() {
			if strings.HasPrefix(scanner.Text(), "data: ") {
				return scanner.Text()
			}
		}
		t.Fatalf("stream ended: %v", scanner.Err())
		return ""
	}
	response, scanner := connect()
	first := read(scanner)
	require.NoError(t, db.WithDBWrite(func(db *sql.DB) error {
		_, err := db.Exec("CREATE TABLE notification_test (id INTEGER)")
		return err
	}))
	second := read(scanner)
	require.NotEqual(t, first, second)
	require.NoError(t, response.Body.Close())
	response, scanner = connect()
	defer response.Body.Close()
	require.Equal(t, second, read(scanner), "reconnect must send a snapshot even without another write")
	require.NoError(t, s.Shutdown(t.Context()))
	for scanner.Scan() {
	}
	require.NoError(t, scanner.Err(), "shutdown must close the SSE handler")
}
