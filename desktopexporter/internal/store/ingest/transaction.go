package ingest

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
)

// InTransaction runs fn inside a DuckDB transaction on conn, committing on nil
// and rolling back otherwise.
//
// Appenders commit at every flush unless something holds them together, so
// without this a batch that failed after its first flush left earlier rows
// durable -- and the sender's retry then collided with them. Appenders honour a
// transaction on their own connection.
//
// The boundary is per attempt rather than around the whole ingest because
// bisection retries need to fail independently, and DuckDB has no nested
// transactions and no SAVEPOINT.
//
// The dictionary flush stays outside: its rows are written on-conflict-do-
// nothing and swept if orphaned, and rolling them back would leave FlushedIDs
// claiming rows that no longer exist.
func InTransaction(ctx context.Context, conn driver.Conn, fn func() error) (err error) {
	exec, ok := conn.(driver.ExecerContext)
	if !ok {
		return fmt.Errorf("InTransaction: %w: connection cannot execute statements", ErrIngestInternal)
	}

	if _, err := exec.ExecContext(ctx, "begin transaction", nil); err != nil {
		return fmt.Errorf("InTransaction: %w: begin: %w", ErrIngestInternal, err)
	}

	// Closing ignores cancellation at both ends. Ingest owns one connection for
	// the life of the process, so a transaction left open fails every later
	// batch with "cannot start a transaction within a transaction" -- and that
	// begin returns before reaching any rollback, so nothing recovers it.
	closeCtx := context.WithoutCancel(ctx)

	defer func() {
		if err != nil {
			if _, rbErr := exec.ExecContext(closeCtx, "rollback", nil); rbErr != nil {
				err = errors.Join(err, fmt.Errorf("InTransaction: rollback: %w", rbErr))
			}
			return
		}
		if _, cErr := exec.ExecContext(closeCtx, "commit", nil); cErr != nil {
			err = fmt.Errorf("InTransaction: %w: commit: %w", ErrIngestInternal, cErr)
			// Harmless if the failed commit already closed it, essential if not.
			if _, rbErr := exec.ExecContext(closeCtx, "rollback", nil); rbErr != nil {
				err = errors.Join(err, fmt.Errorf("InTransaction: rollback after failed commit: %w", rbErr))
			}
		}
	}()

	return fn()
}
