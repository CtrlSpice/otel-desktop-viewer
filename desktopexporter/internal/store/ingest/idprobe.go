package ingest

import (
	"fmt"
	"strings"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
)

// IDProbe returns a bare membership test against a content-derived attribute
// id, or "" when the predicate cannot be answered that way. Callers embed it
// where the owner requires and wrap the result in search.Complete.
//
// # Why
//
// An attribute id is a pure function of (key, value, type, scope), so an
// equality search knows the id it is looking for before the query runs. The
// predicate collapses from "unnest every owner's array, join the dictionary,
// compare the value" to "does this array contain this uuid". Measured on the
// reference capture, exact match on a span attribute:
//
//	attr_value(ids, key) = value                2.67 ms
//	dictionary-first: && (subquery)             0.76 ms
//	list_contains(ids, <literal>)               0.13 ms
//
// # Why it is narrow on purpose
//
// The id only matches if the value string is byte-identical to what ingest
// wrote through util.ValueToStringAndType, and the type token matches too. So
// this refuses everything it cannot guarantee:
//
//   - Only "=". "!=" means "carries this key with a different value", which is
//     not the negation of "carries this id" -- an owner lacking the key
//     entirely satisfies one and not the other.
//   - Floats, in practice. Ingest renders doubles with FormatFloat('f',-1,64),
//     so a user typing 0.333 never matches the stored 0.3333333333333333. That
//     is equally true of today's text comparison, so it is not a regression --
//     just not a case this rescues.
//
// # Why it probes every type rather than trusting the declared one
//
// The id includes the type token, so hashing under the wrong type silently
// matches nothing. An earlier version gated on field.Type == "string" and
// trusted the caller; measuring against real data caught it returning zero
// results for f1.lap.number, an int64 the request had declared as a string.
// Silent zero results from a type disagreement is exactly the failure this
// whole design is supposed to avoid.
//
// So it hashes the value under all eight types and tests membership against
// any of them. The value string is identical in every case -- only the token
// differs -- so this costs eight cheap hashes and one array-overlap test
// instead of one, removes the dependency on client-supplied type metadata
// entirely, and extends the fast path to int64 and bool, whose stored text
// ("200", "true") is exactly what a user types.
//   - Never the NULL sentinel, which means "lacks this key" -- the opposite of
//     a membership test.
//
// Everything refused falls back to the value-comparison form, which is correct
// for all of it, just slower.
//
// Deliberately Go rather than the attr_id SQL macro: the macro is an
// independent reimplementation kept for auditing, and putting it on the
// correctness path would turn a Go/SQL divergence into search silently
// returning nothing. One implementation writes and reads; the other checks.
// TestSearchProbeMatchesIngestedID pins that this agrees with what ingest
// actually stored.
func IDProbe(arrayExpr string, field *search.FieldDefinition, query *search.Query, scope string) string {
	if query == nil || field == nil {
		return ""
	}
	if query.FieldOperator != "=" || query.Value == "NULL" {
		return ""
	}
	ids := make([]string, 0, len(AttrTypes))
	for _, typ := range AttrTypes {
		ids = append(ids, "'"+formatUUID(AttributeID(field.Name, query.Value, typ, scope))+"'::uuid")
	}
	return fmt.Sprintf("%s && [%s]", arrayExpr, strings.Join(ids, ", "))
}

// AttrTypes is every value of the attr_type enum, in schema order.
//
// Must stay in step with TypeCreationQueries in the schema package;
// TestAttrTypesMatchSchemaEnum fails if it drifts. A type missing here means
// attributes of that type silently stop matching the search fast path.
var AttrTypes = []string{
	"string", "int64", "float64", "bool",
	"string[]", "int64[]", "float64[]", "boolean[]",
}
