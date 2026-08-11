// Package queries holds every piece of SQL the store runs, as files.
//
// Two kinds live here, and the split is by what the SQL does rather than by
// which signal runs it:
//
//   - ddl/ is structure: types, tables, indexes and macros, run once when a
//     store is opened. Ordered, because tables reference each other.
//   - spans/, logs/, metrics/ are the read path: one file per query.
//
// Ingest stays in the signal packages. It is Go walking pdata and driving
// appenders, not SQL, and moving it here would separate it from the types it
// unwraps for no gain.
//
// # Why files rather than Go string literals
//
// The DDL was ~700 lines of backticked strings in four Go slices, and the read
// queries ran to hundreds of lines assembled by positional fmt.Sprintf. Neither
// could be syntax-highlighted, neither could be pasted into a DuckDB shell
// against a live database, and reading a Sprintf'd query meant counting %s
// verbs against a trailing argument list to work out which fragment landed
// where -- where getting the order wrong swapped a join for an expression and
// still produced SQL that parsed.
//
// As files they open in a SQL editor, they diff cleanly, and the conditional
// fragments are named template fields instead of positions. Each object's
// rationale travels with it as SQL comments rather than as Go comments one
// indirection away.
//
// # What is checked, and when
//
// Templates are invisible to Go tooling, so a misspelled field is a runtime
// error rather than a compile error. Four things push back. Every template is
// parsed at package init, so a syntax error is a startup panic, not a
// first-request surprise. Option("missingkey=error") makes a bad field fail
// loudly at render instead of writing "<no value>" into the SQL. Every file is
// checked against the registry in both directions, so a renamed file cannot
// leave a dangling reference and an orphaned file cannot sit unnoticed. And the
// golden tests in the signal packages pin the rendered text byte for byte,
// which is what makes editing these files safe.
package queries

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"text/template"
)

//go:embed ddl/types/*.sql ddl/tables/*.sql ddl/indexes/*.sql ddl/macros/*.sql
//go:embed spans/*.sql
var files embed.FS

// Statement is one DDL object: the SQL, plus the file it came from.
//
// The name exists for error messages. The previous form reported "failed to
// create table 4", which meant counting entries in a Go slice to find out what
// had actually failed.
type Statement struct {
	Name string
	SQL  string
}

// Types, Tables, Indexes and Macros return the DDL in creation order.
func Types() []Statement   { return ddl("types", typeFiles) }
func Tables() []Statement  { return ddl("tables", tableFiles) }
func Indexes() []Statement { return ddl("indexes", indexFiles) }
func Macros() []Statement  { return ddl("macros", macroFiles) }

func ddl(kind string, names []string) []Statement {
	out := make([]Statement, 0, len(names))
	for _, n := range names {
		out = append(out, Statement{
			Name: kind + "/" + n,
			SQL:  mustRead(path.Join("ddl", kind, n)),
		})
	}
	return out
}

// Name identifies one read-path query. Typed rather than a bare string so a
// caller cannot pass an arbitrary path, and so the set is enumerable for the
// registration check below.
type Name string

const (
	// SearchSpans fetches one whole trace: the recursive tree walk, its
	// payload, and the resource/scope maps the wire format references.
	SearchSpans Name = "spans/search_spans.sql"
)

// queryNames is every read-path query. Kept beside the constants so adding one
// without registering it is a visible omission rather than a silent one.
var queryNames = []Name{SearchSpans}

var templates = parseAll()

func parseAll() map[Name]*template.Template {
	parsed := make(map[Name]*template.Template, len(queryNames))
	for _, n := range queryNames {
		parsed[n] = template.Must(
			template.New(string(n)).Option("missingkey=error").Parse(mustRead(string(n))),
		)
	}
	checkRegistered(parsed)
	return parsed
}

// checkRegistered walks the embedded tree and fails init on anything the
// registry does not account for -- a .sql file nobody references would never
// run, and would rot unnoticed.
func checkRegistered(parsed map[Name]*template.Template) {
	known := make(map[string]bool, len(parsed)+64)
	for n := range parsed {
		known[string(n)] = true
	}
	for kind, names := range map[string][]string{
		"types": typeFiles, "tables": tableFiles,
		"indexes": indexFiles, "macros": macroFiles,
	} {
		for _, n := range names {
			known[path.Join("ddl", kind, n)] = true
		}
	}
	err := fs.WalkDir(files, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".sql") {
			return nil
		}
		if !known[p] {
			panic("queries: embedded file is not registered: " + p)
		}
		return nil
	})
	if err != nil {
		panic("queries: walking embedded files: " + err.Error())
	}
}

func mustRead(p string) string {
	b, err := files.ReadFile(p)
	if err != nil {
		// Unreachable for registered names: go:embed fails to compile if the
		// directory is missing, and the registration check covers the rest.
		panic("queries: missing embedded file " + p + ": " + err.Error())
	}
	return string(b)
}

// Render fills a query's template fields.
//
// data is normally a struct whose fields are the conditional fragments the
// query composes -- named, so that adding or reordering one cannot silently
// change which fragment lands where.
func Render(name Name, data any) (string, error) {
	tmpl, ok := templates[name]
	if !ok {
		// Unreachable through the Name constants; reachable if a caller
		// converts a string.
		return "", fmt.Errorf("queries: unknown query %q", name)
	}
	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		return "", fmt.Errorf("queries: rendering %s: %w", name, err)
	}
	return sb.String(), nil
}
