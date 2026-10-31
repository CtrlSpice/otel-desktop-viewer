package discovery

import (
	"strconv"
	"strings"
)

// Builder keeps discovery values separate from SQL. Offset reserves positional
// parameters already owned by a surrounding query, such as Home's search CTEs.
type Builder struct {
	Offset int
	values []string
}

func (b *Builder) Bind(value string) string {
	b.values = append(b.values, value)
	return b.parameter(len(b.values) - 1)
}

func (b *Builder) parameter(index int) string {
	return "$" + strconv.Itoa(b.Offset+index+1) + "::VARCHAR"
}

func (b *Builder) Args() []any {
	args := make([]any, len(b.values))
	for i, value := range b.values {
		args[i] = value
	}
	return args
}

// Inline renders a standalone statement for the CLI's existing read-only SQL
// endpoint. Server execution uses Args instead. Replace once, so parameter-like
// text inside a received value is never interpreted as another parameter.
func (b *Builder) Inline(statement string) string {
	replacements := make([]string, 0, 2*len(b.values))
	for i, value := range b.values {
		replacements = append(replacements, b.parameter(i), Literal(value))
	}
	return strings.NewReplacer(replacements...).Replace(statement)
}
