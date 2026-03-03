package ingest

import (
	"database/sql/driver"
	"errors"
	"fmt"

	"github.com/marcboeker/go-duckdb/v2"
)

var (
	ErrIngestInternal = errors.New("ingest internal error")
)

// NewAppenders creates one appender per table name, keyed by table name.
// Caller must call CloseAppenders(appenders, tables) when done so appenders are closed in creation order.
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

// FlushAppenders flushes appenders in reverse order of tables (parents before dependents)
// so FK references exist when rows are written. Safe to call with nil map or nil/empty tables.
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

// CloseAppenders closes appenders in the order of tables, so close order is deterministic.
// Safe to call with nil map or nil/empty tables.
func CloseAppenders(appenders map[string]*duckdb.Appender, tables []string) {
	for i := len(tables) - 1; i >= 0; i-- {
		if a := appenders[tables[i]]; a != nil {
			a.Close()
		}
	}
}
