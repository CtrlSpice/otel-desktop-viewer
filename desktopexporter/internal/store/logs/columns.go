package logs

import "fmt"

// Column identifies a received field or attribute by owner, never by a SQL identifier.
type Column struct {
	Source string `json:"source"`
	Kind   string `json:"kind"`
	Key    string `json:"key"`
}

var columnFields = map[string]map[string]bool{
	"log": {"timeUnixNano": true, "observedTimeUnixNano": true, "severityText": true,
		"severityNumber": true, "traceId": true, "spanId": true, "eventName": true,
		"body": true, "flags": true, "droppedAttributesCount": true},
	"resource": {"droppedAttributesCount": true, "schemaUrl": true},
	"scope":    {"name": true, "version": true, "droppedAttributesCount": true, "schemaUrl": true},
}

func ValidateColumns(columns []Column) error {
	seen := make(map[Column]bool, len(columns))
	for _, column := range columns {
		fields, ok := columnFields[column.Source]
		if !ok || (column.Kind != "field" && column.Kind != "attribute") ||
			(column.Kind == "field" && !fields[column.Key]) || seen[column] {
			return fmt.Errorf("invalid or duplicate log column %+v: %w", column, ErrInvalidLogQuery)
		}
		seen[column] = true
	}
	return nil
}
