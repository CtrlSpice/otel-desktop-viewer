package ingest

import (
	"context"
	"database/sql/driver"
	"errors"

	"github.com/duckdb/duckdb-go/v2"
)

var ErrIngestInternal = errors.New("ingest internal error")

// InterruptedContextError restores cancellation identity lost by the driver.
func InterruptedContextError(ctx context.Context, err error) error {
	ctxErr := ctx.Err()
	if ctxErr == nil {
		return err
	}
	var duckErr *duckdb.Error
	if errors.As(err, &duckErr) && duckErr.Type == duckdb.ErrorTypeInterrupt && !errors.Is(err, ctxErr) {
		return errors.Join(ctxErr, err)
	}
	return err
}

func prepareNamedValues(conn *duckdb.Conn, args []any) ([]driver.NamedValue, error) {
	out := make([]driver.NamedValue, 0, len(args))
	for i, arg := range args {
		nv := driver.NamedValue{Ordinal: i + 1, Value: arg}
		if err := conn.CheckNamedValue(&nv); err != nil {
			converted, err := driver.DefaultParameterConverter.ConvertValue(arg)
			if err != nil {
				return nil, err
			}
			nv.Value = converted
		}
		out = append(out, nv)
	}
	return out, nil
}
