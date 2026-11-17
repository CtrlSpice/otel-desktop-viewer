package ingest

import "strings"

// OwnedAppendQuery decodes each row's complete binary payloads into owned values.
// Table and columns must be application constants, never received telemetry.
func OwnedAppendQuery(table string, columns []string) string {
	var replacements []string
	for _, column := range columns {
		root := "variant_bytes_to_variant(" + column + ")"
		switch column {
		case "resource.attributes":
			replacements = append(replacements, "struct_update(resource, attributes := "+root+")::otel_resource as resource")
		case "scope.attributes":
			replacements = append(replacements, "struct_update(scope, attributes := "+root+")::otel_scope as scope")
		default:
			replacements = append(replacements, root+" as "+column)
		}
	}
	return "insert into " + table + " select * replace (" + strings.Join(replacements, ", ") + ") from appended_data"
}
