package ingest

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"

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
		var a *duckdb.Appender
		var err error
		switch table {
		case "logs":
			a, err = NewTypedQueryAppender(conn, OwnedAppendQuery(table, []string{"body", "attributes", "resource.attributes", "scope.attributes"}),
				`select * replace(null::blob as body,
					null::otel_resource_input as resource, null::otel_scope_input as scope,
					null::blob as attributes) from logs where false`)
		case "spans":
			a, err = NewTypedQueryAppender(conn, OwnedAppendQuery(table, []string{"attributes", "resource.attributes", "scope.attributes"}),
				`select * replace(null::otel_resource_input as resource,
					null::otel_scope_input as scope, null::blob as attributes)
				from spans where false`)
		case "events", "links", "exemplars":
			a, err = NewTypedQueryAppender(conn, OwnedAppendQuery(table, []string{"attributes"}),
				`select * replace(null::blob as attributes) from `+table+` where false`)
		case "metric_datapoints":
			a, err = NewTypedQueryAppender(conn, OwnedAppendQuery(table, []string{"attributes", "resource.attributes", "scope.attributes"}),
				`select * replace(null::blob as attributes,
					null::otel_resource_input as resource, null::otel_scope_input as scope)
				from metric_datapoints where false`)
		default:
			a, err = duckdb.NewAppender(conn, "", "", table)
		}
		if err != nil {
			CloseAppenders(out, tables)
			return nil, fmt.Errorf("NewAppenders: %w: %w", ErrIngestInternal, err)
		}
		out[table] = a
	}
	return out, nil
}

// OwnedAppendQuery decodes each row's complete binary payloads into owned values.
// Table and columns must be application constants, never received telemetry.
func OwnedAppendQuery(table string, columns []string) string {
	var replacements []string
	for _, column := range columns {
		root := "variant_bytes_to_variant(" + column + ")"
		switch column {
		case "resource.attributes":
			replacements = append(replacements, "struct_update(resource, attributes := "+root+")::otel_resource as resource")
		case "scope.attributes":
			replacements = append(replacements, "struct_update(scope, attributes := "+root+")::otel_scope as scope")
		default:
			replacements = append(replacements, root+" as "+column)
		}
	}
	return "insert into " + table + " select * replace (" + strings.Join(replacements, ", ") + ") from appended_data"
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
