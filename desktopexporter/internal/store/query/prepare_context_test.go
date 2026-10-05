package query

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestRequireReadOnlyCancellationLeavesConnectionReusable(t *testing.T) {
	viewerStore, err := store.NewStore(context.Background(), "", zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, viewerStore.Close()) })

	require.NoError(t, viewerStore.WithDBRead(func(db *sql.DB) error {
		conn, err := db.Conn(context.Background())
		require.NoError(t, err)
		defer func() { require.NoError(t, conn.Close()) }()

		statement := "SELECT " + strings.Repeat("1,", 100_000) + "1"
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		defer cancel()
		_, err = requireReadOnly(ctx, conn, statement)
		require.Error(t, err)
		assert.True(t, errors.Is(err, context.DeadlineExceeded), "%v", err)
		assert.NotErrorIs(t, err, ErrReadOnly)

		var value int
		require.NoError(t, conn.QueryRowContext(context.Background(), "SELECT 1").Scan(&value))
		assert.Equal(t, 1, value)
		return nil
	}))
}
