package main

// These projections consume documents checked by validateLogDetail or
// validateMetricDetail and select only the case-sensitive fields used for display.
func detailFieldValues(document map[string]any, fields []string) []any {
	values := make([]any, len(fields))
	for i, field := range fields {
		values[i] = document[field]
	}
	return values
}

func detailValueDocument(value any) map[string]any {
	object := value.(map[string]any)
	payload := object["value"]
	switch object["kind"] {
	case "array":
		items := payload.([]any)
		values := make([]any, len(items))
		for i, item := range items {
			values[i] = detailValueDocument(item)
		}
		payload = values
	case "map":
		payload = detailAttributeDocuments(payload.([]any))
	}
	return map[string]any{"kind": object["kind"], "value": payload}
}

func detailAttributeDocuments(attributes []any) []any {
	documents := make([]any, len(attributes))
	for i, value := range attributes {
		attribute := value.(map[string]any)
		documents[i] = map[string]any{"key": attribute["key"], "value": detailValueDocument(attribute["value"])}
	}
	return documents
}

func formatDetailAttributes(attributes []any) string {
	rows := make([][]any, len(attributes))
	for i, value := range attributes {
		attribute := value.(map[string]any)
		tagged := detailValueDocument(attribute["value"])
		rows[i] = []any{attribute["key"], tagged["kind"], formatQueryValue(tagged["value"])}
	}
	return detailTable([]string{"key", "kind", "value"}, rows)
}

func detailExemplarDocuments(exemplars []any) []any {
	documents := make([]any, len(exemplars))
	for i, value := range exemplars {
		exemplar := value.(map[string]any)
		documents[i] = map[string]any{
			"timestamp":          exemplar["timestamp"],
			"traceID":            exemplar["traceID"],
			"spanID":             exemplar["spanID"],
			"valueType":          exemplar["valueType"],
			"intValue":           exemplar["intValue"],
			"doubleValue":        exemplar["doubleValue"],
			"filteredAttributes": detailAttributeDocuments(exemplar["filteredAttributes"].([]any)),
		}
	}
	return documents
}
