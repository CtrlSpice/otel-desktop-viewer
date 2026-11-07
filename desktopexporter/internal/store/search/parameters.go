package search

import (
	"strconv"
	"strings"
)

// RebaseParameters composes generated queries whose $N bindings start at one.
// Quoted SQL and comments are copied unchanged, including literal "$1" text.
func RebaseParameters(statement string, offset int) string {
	if offset == 0 {
		return statement
	}
	var out strings.Builder
	for i := 0; i < len(statement); {
		start := i
		switch {
		case statement[i] == '\'' || statement[i] == '"':
			quote := statement[i]
			i++
			for i < len(statement) {
				if statement[i] == quote {
					i++
					if i < len(statement) && statement[i] == quote {
						i++
						continue
					}
					break
				}
				i++
			}
		case strings.HasPrefix(statement[i:], "--"):
			for i < len(statement) && statement[i] != '\n' {
				i++
			}
		case strings.HasPrefix(statement[i:], "/*"):
			i += 2
			depth := 1
			for i < len(statement) && depth > 0 {
				switch {
				case strings.HasPrefix(statement[i:], "/*"):
					depth++
					i += 2
				case strings.HasPrefix(statement[i:], "*/"):
					depth--
					i += 2
				default:
					i++
				}
			}
		case statement[i] == '$':
			i++
			if i < len(statement) && statement[i] >= '1' && statement[i] <= '9' {
				for i < len(statement) && statement[i] >= '0' && statement[i] <= '9' {
					i++
				}
				position, err := strconv.Atoi(statement[start+1 : i])
				if err == nil {
					out.WriteByte('$')
					out.WriteString(strconv.Itoa(position + offset))
					continue
				}
			} else {
				end := i
				for end < len(statement) && (statement[end] == '_' || statement[end] >= 'a' && statement[end] <= 'z' || statement[end] >= 'A' && statement[end] <= 'Z' || statement[end] >= '0' && statement[end] <= '9') {
					end++
				}
				if end < len(statement) && statement[end] == '$' {
					delimiter := statement[start : end+1]
					if close := strings.Index(statement[end+1:], delimiter); close >= 0 {
						i = end + 1 + close + len(delimiter)
					}
				}
			}
		default:
			i++
		}
		out.WriteString(statement[start:i])
	}
	return out.String()
}
