package search

import (
	"errors"
	"fmt"
)

var ErrInvalidSort = errors.New("invalid search sort")

// Text fields use DuckDB's binary collation for bounded selection. The UI
// reorders the returned subset with localeCompare, so mixed-case or Unicode
// values may not select the same top rows across the server/client boundary.
type Sort struct {
	Field     string `json:"field"`
	Direction string `json:"direction"`
}

type ResultOptions struct {
	Limit *int64
	Sort  *Sort
}

func SortDirectionSQL(direction string) (string, error) {
	switch direction {
	case "asc", "desc":
		return direction, nil
	default:
		return "", fmt.Errorf("unsupported sort direction %q: %w", direction, ErrInvalidSort)
	}
}
