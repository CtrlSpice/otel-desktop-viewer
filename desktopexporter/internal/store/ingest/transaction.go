package ingest

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
)

// InTransaction runs fn inside a DuckDB transaction on conn, committing when it
// returns nil and rolling back otherwise.
//
// # Why ingest needs this rather than a transaction around the whole call
//
// The appenders flush every flushIntervalSpans rows, and every flush is a
// commit of its own unless something holds them together. So a batch bigger
// than one flush interval that failed later left everything before the last
// boundary durable while the call reported failure. The collector then retried
// the whole batch, whose early rows now collided with the copies already kept,
// and the retry failed for a reason the first attempt created -- a batch that
// can never succeed.
//
// Appenders honour a transaction opened on their own connection: flush inside
// one, roll back, and no rows remain.
//
// # Why the boundary lives here and not around the whole ingest
//
// A failed append pass is retried in narrowing halves to find the rows that
// caused it, and each attempt has to be able to fail without poisoning the
// next. DuckDB offers no way to do that inside an enclosing transaction --
// `begin` within a transaction is "cannot start a transaction within a
// transaction", and SAVEPOINT is not implemented at all -- so every attempt
// must be its own top-level transaction. Wrapping the whole ingest would make
// bisection impossible.
//
// # Why the dictionary flush stays outside
//
// Dictionary rows are written with `on conflict do nothing` and are reachable
// only through the owner rows that reference them, so committing them for a
// batch that later fails leaves nothing worse than rows SweepOrphans already
// exists to collect. Keeping them out of the transaction also keeps FlushedIDs
// honest: it marks ids as written the moment their insert succeeds, and a
// rollback that unmade those inserts would leave it claiming rows that no
// longer exist -- the next batch would skip re-inserting them and its owners
// would carry uuid[] references into nothing, which no foreign key catches.
func InTransaction(ctx context.Context, conn driver.Conn, fn func() error) (err error) {
	exec, ok := conn.(driver.ExecerContext)
	if !ok {
		return fmt.Errorf("InTransaction: %w: connection cannot execute statements", ErrIngestInternal)
	}

	if _, err := exec.ExecContext(ctx, "begin transaction", nil); err != nil {
		return fmt.Errorf("InTransaction: %w: begin: %w", ErrIngestInternal, err)
	}

	defer func() {
		if err != nil {
			// Rollback runs even when ctx is already done: the context that
			// cancelled the work must not also abandon the transaction, or the
			// connection stays wedged for every batch after this one.
			if _, rbErr := exec.ExecContext(context.WithoutCancel(ctx), "rollback", nil); rbErr != nil {
				err = errors.Join(err, fmt.Errorf("InTransaction: rollback: %w", rbErr))
			}
			return
		}
		if _, cErr := exec.ExecContext(ctx, "commit", nil); cErr != nil {
			err = fmt.Errorf("InTransaction: %w: commit: %w", ErrIngestInternal, cErr)
		}
	}()

	return fn()
}
