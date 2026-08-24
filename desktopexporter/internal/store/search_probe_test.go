package store

// The content-hashed search fast path is only correct if the id it computes is
// the id ingest wrote. Nothing enforces that: the probe hashes a *search term*,
// ingest hashes a *stored attribute*, and the two arrive by completely
// different routes -- one from a user typing into a box, one from
// util.ValueToStringAndType. If they ever disagree, search returns nothing, with
// no error and no clue.
//
// So these compare the two directly, and then compare fast-path results against
// slow-path results on the same data.

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// The load-bearing equality: for every attribute actually in the dictionary,
// the id a search probe computes from (key, value, type, scope) must be the id
// stored on the row.
//
// What this catches, and what it cannot: both sides call AttributeID, so this
// detects the probe passing *wrong arguments* -- the wrong scope, a mangled
// value, the wrong type token -- which is the realistic failure, since the
// probe assembles them from a search field and ingest from pdata.
//
// It cannot detect AttributeID itself changing: mutate the hash and both sides
// move together, still agreeing. That is precisely why the SQL attr_id macro
// exists as an independent reimplementation; TestStoredIDsMatchTheSQLMacro is
// what fails when the encoding drifts. Mutation-checked in both directions --
// a wrong scope here fails this test and not that one, a changed hash fails
// that one and not this.
func TestSearchProbeMatchesIngestedID(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	defer s.Close()

	ingestAll(t, s, 1)

	type row struct{ key, value, typ, scope, id string }
	var rows []row
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		r, err := db.Query(`select key, value, type::varchar, scope, id::varchar from attributes`)
		if err != nil {
			return err
		}
		defer r.Close()
		for r.Next() {
			var x row
			if err := r.Scan(&x.key, &x.value, &x.typ, &x.scope, &x.id); err != nil {
				return err
			}
			rows = append(rows, x)
		}
		return r.Err()
	}))
	require.NotEmpty(t, rows, "fixture must produce attributes, or this proves nothing")

	checked := 0
	for _, x := range rows {
		// The type comes from the dictionary row, which is exactly what
		// discovery serves to the frontend -- so this walks every distinct
		// (key, value, type, scope) the fixture produced and asserts the
		// probe recomputes the id ingest stored for it.
		probe := ingest.IDProbe("ids",
			&search.FieldDefinition{Name: x.key, Type: x.typ},
			&search.Query{FieldOperator: "=", Value: x.value},
			x.scope)
		require.NotEmpty(t, probe, "probe should fire")
		assert.Contains(t, probe, x.id,
			"probe id disagrees with the stored id for %s=%s (%s)", x.key, x.value, x.scope)
		checked++
	}
	assert.Positive(t, checked, "no attributes were compared")
	t.Logf("verified %d attributes across %d dictionary rows", checked, len(rows))
}

// IDProbe must refuse everything it cannot answer byte-exactly, or the fast
// path silently returns wrong answers instead of falling back.
func TestIDProbeRefusesWhatItCannotGuarantee(t *testing.T) {
	str := &search.FieldDefinition{Name: "http.method", Type: "string"}
	for _, tc := range []struct {
		name  string
		field *search.FieldDefinition
		query *search.Query
		want  bool
	}{
		{"string equality", str, &search.Query{FieldOperator: "=", Value: "GET"}, true},
		{"value with spaces", str, &search.Query{FieldOperator: "=", Value: " GET "}, true},
		{"not-equals", str, &search.Query{FieldOperator: "!=", Value: "GET"}, false},
		{"contains", str, &search.Query{FieldOperator: "CONTAINS", Value: "GE"}, false},
		// The literal string "NULL" is an ordinary equality now. It used to
		// be refused here because the old wire format smuggled the null check
		// through as `= "NULL"`, and a content-derived id for that would have
		// matched attributes literally valued NULL. The null check has been
		// its own IS NULL operator since the grammar unification, so the
		// refusal would only slow down a legitimate search for the text.
		{"literal NULL string equality", str, &search.Query{FieldOperator: "=", Value: "NULL"}, true},
		// The declared type is trusted -- for an attribute field it is the
		// token ingest wrote, served back by discovery -- so a non-string
		// type takes the fast path under that type. An int64's stored text
		// is "200", exactly what is typed.
		{"int64 field", &search.FieldDefinition{Name: "http.status_code", Type: "int64"},
			&search.Query{FieldOperator: "=", Value: "200"}, true},
		// With no type there is no id to compute, so this must fall back to
		// the value-comparison form rather than guess a token. Guessing is
		// what the all-eight-types version did, and it conflated int64 200
		// with string "200".
		{"untyped field falls back", &search.FieldDefinition{Name: "x"},
			&search.Query{FieldOperator: "=", Value: "y"}, false},
		{"unknown type token falls back", &search.FieldDefinition{Name: "x", Type: "decimal"},
			&search.Query{FieldOperator: "=", Value: "y"}, false},
		{"nil query", str, nil, false},
	} {
		got := ingest.IDProbe("ids", tc.field, tc.query, ingest.ScopeSpan) != ""
		assert.Equal(t, tc.want, got, "%s", tc.name)
	}
}

// Fast and slow paths must return the same spans. The probe fires for string
// equality, so the comparison is run against the value-comparison form by
// asking the same question with an operator the probe refuses but that means
// the same thing for a single-valued attribute.
func TestFastPathAgreesWithValueComparison(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	defer s.Close()

	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, integrityTraces(1), s.FlushedIDs())
	}))

	count := func(t *testing.T, typ, op, value string) int {
		t.Helper()
		query := map[string]any{
			"type": "condition",
			"query": map[string]any{
				"field": map[string]any{
					"name": "http.method", "type": typ,
					"searchScope": "attribute", "attributeScope": "span",
				},
				"fieldOperator": op,
				"value":         value,
			},
		}
		var n int
		require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
			raw, err := spans.SearchTraces(ctx, db, 0, 1<<62, query)
			if err != nil {
				return err
			}
			var out []map[string]any
			if err := json.Unmarshal(raw, &out); err != nil {
				return err
			}
			n = len(out)
			return nil
		}))
		return n
	}

	// type "string" takes the probe; the empty type forces the value
	// comparison. Same question, two implementations.
	fast := count(t, "string", "=", "GET")
	slow := count(t, "", "=", "GET")
	assert.Equal(t, slow, fast, "fast path must find exactly what the value comparison finds")
	assert.Positive(t, fast, "fixture must match, or this compares two zeroes")

	// A value that does not exist must find nothing on both paths.
	assert.Zero(t, count(t, "string", "=", "POST"))
	assert.Zero(t, count(t, "", "=", "POST"))
}

// AttrTypes drives the fast path's hashing, so a type present in the schema
// enum but missing from that list would silently stop matching. Parsed from the
// enum DDL rather than restated, so the two cannot be edited apart.
func TestAttrTypesMatchSchemaEnum(t *testing.T) {
	var ddl string
	for _, stmt := range queries.Types() {
		if strings.Contains(stmt.SQL, "attr_type") {
			ddl = stmt.SQL
		}
	}
	require.NotEmpty(t, ddl, "attr_type enum not found in the type DDL")

	open := strings.Index(ddl, "(")
	close := strings.LastIndex(ddl, ")")
	require.Greater(t, close, open)
	var fromSchema []string
	for _, part := range strings.Split(ddl[open+1:close], ",") {
		fromSchema = append(fromSchema, strings.Trim(strings.TrimSpace(part), "'"))
	}

	assert.Equal(t, fromSchema, ingest.AttrTypes,
		"ingest.AttrTypes must list exactly the attr_type enum values, in order")
}
