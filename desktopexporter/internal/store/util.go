package store

import "strings"

// buildPlaceholders returns a comma-separated list of ? placeholders for SQL IN clauses.
func buildPlaceholders(count int) string {
	if count == 0 {
		return ""
	}
	marks := make([]string, count)
	for i := range count {
		marks[i] = "?"
	}
	return strings.Join(marks, ",")
}
