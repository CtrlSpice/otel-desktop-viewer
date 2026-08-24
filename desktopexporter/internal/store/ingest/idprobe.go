package ingest

import (
	"fmt"
	"slices"

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
//   - Never the NULL sentinel, which means "lacks this key" -- the opposite of
//     a membership test.
//   - Any type token the schema enum does not contain, which can only mean the
//     caller and the store disagree about what types exist.
//
// # Why it trusts the declared type
//
// The id includes the type token, so the probe needs to know the type. For an
// attribute field that type is not client guesswork: GetTraceAttributes serves
// the field list as "select distinct a.key, a.scope, a.type from attributes",
// so it is the token ingest itself wrote, round-tripped back. A key carrying
// two types appears as two field definitions, each correct.
//
// An earlier version hashed under all eight types and tested overlap, on the
// belief that the declared type could not be trusted -- a conclusion drawn from
// f1.lap.number returning nothing when declared as a string. Whatever produced
// that "string" (a hand-built probe request, or the older schema it was
// observed against, which was mid-rewrite and at one point serving from a
// corrupt database), it was not the frontend: the dictionary holds the key as
// int64, and discovery reads the dictionary.
//
// Probing all eight was also less correct, not merely more expensive. The value
// string is identical across types, so int64 200 and string "200" hash to
// different ids but both land in the overlap set: searching an integer field
// for 200 would also match a string "200" written by another producer. No key
// in the reference capture carries two types, so nothing surfaced it -- it
// would have appeared the first time two producers disagreed, as a search
// quietly returning too much. One hash under the declared type is cheaper and
// says what the user meant.
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
	// Only exact equality has a content-derived id. The literal string
	// "NULL" is an ordinary value here: the null check arrives as its own
	// IS NULL operator since the sentinel wire format was removed, so it
	// can never reach this function disguised as an equality.
	if query.FieldOperator != "=" {
		return ""
	}
	if !slices.Contains(AttrTypes, field.Type) {
		return ""
	}
	id := formatUUID(AttributeID(field.Name, query.Value, field.Type, scope))
	return fmt.Sprintf("list_contains(%s, '%s'::uuid)", arrayExpr, id)
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
