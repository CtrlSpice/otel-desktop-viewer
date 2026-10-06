package ingest

import (
	"fmt"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
)

// IDProbe returns a bare membership test against a content-derived attribute
// id, or "" when the predicate cannot be answered that way. Callers embed it
// where the owner requires and wrap the result in search.Complete.
//
// An attribute id is a pure function of (key, value, type, scope), so an
// equality search can test membership without joining the dictionary. The
// probe accepts only exact scalar encodings under the discovered type;
// everything else uses the value-comparison path. The independent attr_id SQL
// macro remains an integrity check rather than part of this lookup.
func IDProbe(arrayExpr string, field *search.FieldDefinition, query *search.Query, scope string) string {
	if query == nil || field == nil {
		return ""
	}
	// Only exact equality has a content-derived id. Null checks use IS NULL.
	if query.FieldOperator != "=" {
		return ""
	}
	value, ok := searchValue(field.Type, query.Value)
	if !ok {
		return ""
	}
	id := formatUUID(AttributeID(field.Name, value))
	return fmt.Sprintf("list_contains(%s, '%s'::uuid)", arrayExpr, id)
}

func searchValue(kind, value string) (string, bool) {
	// Discovery reports the encoded kind; scalar values can be reconstructed
	// exactly enough for the equality fast path. Everything else uses SQL.
	switch kind {
	case "string":
		return `{"kind":"string","value":` + quoteJSON(value) + `}`, true
	case "int64":
		return `{"kind":"int64","value":` + quoteJSON(value) + `}`, true
	case "bool":
		if value == "true" || value == "false" {
			return `{"kind":"bool","value":` + value + `}`, true
		}
	}
	return "", false
}

func quoteJSON(s string) string {
	return fmt.Sprintf("%q", s)
}
