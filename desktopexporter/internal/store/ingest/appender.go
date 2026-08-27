package ingest

import (
	"database/sql/driver"
	"errors"
	"fmt"

	"github.com/duckdb/duckdb-go/v2"
)

var (
	ErrIngestInternal = errors.New("ingest internal error")
)

// NewAppenders opens one appender per table, keyed by name. Caller must
// CloseAppenders.
func NewAppenders(conn driver.Conn, tables []string) (map[string]*duckdb.Appender, error) {
	out := make(map[string]*duckdb.Appender, len(tables))
	for _, table := range tables {
		a, err := duckdb.NewAppender(conn, "", "", table)
		if err != nil {
			CloseAppenders(out, tables)
			return nil, fmt.Errorf("NewAppenders: %w: %w", ErrIngestInternal, err)
		}
		out[table] = a
	}
	return out, nil
}

// FlushAppenders flushes in reverse table order, so parents land before the
// rows referencing them.
func FlushAppenders(appenders map[string]*duckdb.Appender, tables []string) error {
	for i := len(tables) - 1; i >= 0; i-- {
		if a := appenders[tables[i]]; a != nil {
			if err := a.Flush(); err != nil {
				return fmt.Errorf("FlushAppenders: %w: %w", ErrIngestInternal, err)
			}
		}
	}
	return nil
}

// CloseAppenders closes every appender in reverse table order and returns the
// first error. It keeps closing after a failure because Close releases each
// buffered chunk; later errors only report that the transaction is aborted.
func CloseAppenders(appenders map[string]*duckdb.Appender, tables []string) error {
	var firstErr error
	for i := len(tables) - 1; i >= 0; i-- {
		if a := appenders[tables[i]]; a != nil {
			if err := a.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}
