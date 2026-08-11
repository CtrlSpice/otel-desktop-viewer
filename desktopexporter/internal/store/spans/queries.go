package spans

import (
	"embed"
	"strings"
	"text/template"
)

// Query bodies live in queries/*.sql rather than in Go string literals.
//
// They are the same text either way -- a golden test pins that -- but a .sql
// file can be opened in a SQL editor, syntax-highlighted, and pasted straight
// into a DuckDB shell against a live database when something is wrong. A
// several-hundred-line query assembled by positional fmt.Sprintf can do none of
// that, and reading it meant counting %s verbs against a trailing argument list
// to work out which fragment landed where.
//
// The cost is real and worth stating: these are invisible to Go tooling. A typo
// in a field name is a runtime error, not a compile error. text/template is
// used with Option("missingkey=error") so a misspelled field fails loudly at
// render time instead of silently rendering "<no value>" into SQL, and the
// golden tests catch the rest.
//
//go:embed queries/*.sql
var queryFS embed.FS

// searchSpansParams are the conditional fragments searchSpansSQL assembles.
//
// Named fields rather than positional arguments: the previous form threaded
// four %s through ~150 lines, so getting the order wrong swapped a join for an
// expression and produced SQL that still parsed.
type searchSpansParams struct {
	// CTEs is the search_params CTE, always present.
	CTEs string
	// MatchedCTE, MatchedExpr and MatchedJoin are empty, "true" and empty
	// respectively when there is no search predicate.
	MatchedCTE  string
	MatchedExpr string
	MatchedJoin string
}

var searchSpansTmpl = template.Must(
	template.New("search_spans.sql").
		Option("missingkey=error").
		Parse(mustQuery("search_spans.sql")),
)

func mustQuery(name string) string {
	b, err := queryFS.ReadFile("queries/" + name)
	if err != nil {
		// Unreachable: go:embed fails at compile time if the file is absent.
		panic("spans: missing embedded query " + name + ": " + err.Error())
	}
	return string(b)
}

func renderSearchSpans(p searchSpansParams) (string, error) {
	var sb strings.Builder
	if err := searchSpansTmpl.Execute(&sb, p); err != nil {
		return "", err
	}
	return sb.String(), nil
}
