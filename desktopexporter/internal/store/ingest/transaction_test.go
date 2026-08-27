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

// A transaction must close even when the context that carried it is already
// cancelled, at either end.
//
// Ingest owns one connection for the life of the process, so a transaction left
// open does not cost one batch -- it costs every batch after it, because the
// next `begin` fails with "cannot start a transaction within a transaction" and
// returns before reaching any rollback. Nothing recovers that short of a
// restart, which is why both the commit and the rollback ignore cancellation.
func TestInTransaction_CancelDuringCommitDoesNotWedgeTheConnection(t *testing.T) {
	t.Parallel()
	s, _ := storetest.New(t)

	err := s.WithConn(func(conn driver.Conn) error {
		ctx, cancel := context.WithCancel(context.Background())

		// fn succeeds, then the context dies before the deferred commit runs --
		// the window where a commit bound to ctx would refuse to close the
		// transaction it had already earned.
		txErr := ingest.InTransaction(ctx, conn, func() error {
			cancel()
			return nil
		})
		require.NoError(t, txErr, "a cancel after the work is done must not fail the commit")

		// The proof: another transaction can still be opened on this
		// connection. Before the fix this failed permanently.
		return ingest.InTransaction(context.Background(), conn, func() error { return nil })
	})
	require.NoError(t, err, "the connection must still be usable")
}

// The same at the other end: a cancelled batch rolls back rather than leaving
// its transaction open.
func TestInTransaction_CancelledWorkStillClosesTheTransaction(t *testing.T) {
	t.Parallel()
	s, _ := storetest.New(t)

	sentinel := errors.New("work failed")

	err := s.WithConn(func(conn driver.Conn) error {
		ctx, cancel := context.WithCancel(context.Background())

		// Cancelled while the work is in flight, which is where a real
		// shutdown lands: begin has already succeeded, so there is an open
		// transaction that the cancelled context must not walk away from.
		txErr := ingest.InTransaction(ctx, conn, func() error {
			cancel()
			return sentinel
		})
		require.ErrorIs(t, txErr, sentinel)

		return ingest.InTransaction(context.Background(), conn, func() error { return nil })
	})
	require.NoError(t, err)
}

// Rolling back really does discard the work, rather than the transaction
// having been closed early by something else.
func TestInTransaction_RollbackDiscardsWrites(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	sentinel := errors.New("boom")
	err := s.WithConn(func(conn driver.Conn) error {
		txErr := ingest.InTransaction(ctx, conn, func() error {
			exec := conn.(driver.ExecerContext)
			if _, e := exec.ExecContext(ctx,
				`insert into resources (id, attribute_ids) values (gen_random_uuid(), [])`, nil); e != nil {
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
