package spans

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Hostile inputs for the two fields a caller controls: the attribute key and
// the value compared against it. Each would change the meaning of the query if
// it reached the SQL text instead of a bound parameter.
var injectionPayloads = []string{
	`' or 1=1 --`,
	`'; drop table spans; --`,
	`" or ""="`,
	`\' or 1=1`,
	`x') or (select count(*) from attributes) > 0 --`,
	// Template syntax in data must remain inert after rendering.
	`{{.MatchedJoin}}`,
	`{{template "x"}}`,
}

// TestGetTraceViewSQLBindsHostileInput verifies that caller-controlled text
// becomes a bound argument rather than SQL text.
//
// text/template does not escape SQL. Safety depends on keeping values in bound
// arguments, and template syntax in a value must not trigger another expansion.
func TestGetTraceViewSQLBindsHostileInput(t *testing.T) {
	t.Parallel()
	for _, payload := range injectionPayloads {
		t.Run(payload, func(t *testing.T) {
			// A non-equality operator so the query keeps the value comparison
			// path. Under "=" the id probe hashes the value away entirely, so
			// it never reaches the SQL either -- safe, but it would prove
			// nothing about the path where the value is actually compared.
			criteria := map[string]any{
				"id":   "n1",
				"type": "condition",
				"query": map[string]any{
					"field": map[string]any{
						"name":           payload,
						"searchScope":    "attribute",
						"attributeScope": "span",
						"type":           "string",
					},
					"fieldOperator": "CONTAINS",
					"value":         payload,
				},
			}

			query, args, err := getTraceViewSQL("00000000000000000000000000000099", criteria)
			require.NoError(t, err)

			require.NotContains(t, query, payload,
				"caller input reached the SQL text; it must be a bound argument")

			var found bool
			for _, a := range args {
				if s, ok := a.(string); ok && strings.Contains(s, payload) {
					found = true
				}
			}
			require.True(t, found, "payload should appear among the bound args")

			// The rendered SQL must carry no unexpanded template syntax,
			// whatever the input contained.
			require.NotContains(t, query, "{{", "unrendered template directive in SQL")
			require.NotContains(t, query, "<no value>", "template rendered a missing field")
		})
	}
}
