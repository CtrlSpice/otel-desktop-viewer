package searchindex

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
)

type connection struct{ driver.Conn }

func (c connection) ExecContext(ctx context.Context, q string, values ...any) (sql.Result, error) {
	args, err := arguments(c.Conn, values)
	if err != nil {
		return nil, err
	}
	e, ok := c.Conn.(driver.ExecerContext)
	if !ok {
		return nil, fmt.Errorf("catalogue connection cannot execute")
	}
	return e.ExecContext(ctx, q, args)
}

func arguments(conn driver.Conn, values []any) ([]driver.NamedValue, error) {
	args := make([]driver.NamedValue, len(values))
	for i, v := range values {
		args[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
		if c, ok := conn.(driver.NamedValueChecker); ok {
			err := c.CheckNamedValue(&args[i])
			if err == nil {
				continue
			}
			if !errors.Is(err, driver.ErrSkip) {
				return nil, err
			}
		}
		var err error
		args[i].Value, err = driver.DefaultParameterConverter.ConvertValue(v)
		if err != nil {
			return nil, err
		}
	}
	return args, nil
}
