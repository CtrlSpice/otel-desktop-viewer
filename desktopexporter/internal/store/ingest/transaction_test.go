package ingest_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInTransaction_CancelDuringCommitDoesNotWedgeTheConnection verifies that
// commit closes the transaction after its work context is cancelled.
func TestInTransaction_CancelDuringCommitDoesNotWedgeTheConnection(t *testing.T) {
	t.Parallel()
	s, _ := storetest.New(t)

	err := s.WithConn(func(conn driver.Conn) error {
		ctx, cancel := context.WithCancel(context.Background())

		// Cancel after the work succeeds but before the deferred commit runs.
		txErr := ingest.InTransaction(ctx, conn, func() error {
			cancel()
			return nil
		})
		require.NoError(t, txErr, "a cancel after the work is done must not fail the commit")

		// A second transaction proves that the first one closed.
		return ingest.InTransaction(context.Background(), conn, func() error { return nil })
	})
	require.NoError(t, err, "the connection must still be usable")
}

// TestInTransaction_CancelledWorkStillClosesTheTransaction verifies that failed
// work rolls back after its context is cancelled.
func TestInTransaction_CancelledWorkStillClosesTheTransaction(t *testing.T) {
	t.Parallel()
	s, _ := storetest.New(t)

	sentinel := errors.New("work failed")

	err := s.WithConn(func(conn driver.Conn) error {
		ctx, cancel := context.WithCancel(context.Background())

		// Cancel while the transaction is open.
		txErr := ingest.InTransaction(ctx, conn, func() error {
			cancel()
			return sentinel
		})
		require.ErrorIs(t, txErr, sentinel)

		return ingest.InTransaction(context.Background(), conn, func() error { return nil })
	})
	require.NoError(t, err)
}

// TestInTransaction_RollbackDiscardsWrites verifies rollback visibility.
func TestInTransaction_RollbackDiscardsWrites(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	sentinel := errors.New("boom")
	err := s.WithConn(func(conn driver.Conn) error {
		txErr := ingest.InTransaction(ctx, conn, func() error {
			exec := conn.(driver.ExecerContext)
			if _, e := exec.ExecContext(ctx,
				`insert into resources (id, payload_id, attribute_ids)
					 values (gen_random_uuid(), gen_random_uuid(), [])`, nil); e != nil {
				return e
			}
			return sentinel
		})
		require.ErrorIs(t, txErr, sentinel)
		return nil
	})
	require.NoError(t, err)

	var n int
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		return db.QueryRowContext(ctx, "select count(*) from resources").Scan(&n)
	}))
	assert.Zero(t, n, "a rolled-back write must leave nothing behind")
}
