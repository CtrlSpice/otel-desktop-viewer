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
	// text/template renders data as text and never re-parses it, so this is
	// inert -- but assert it rather than trust it, since the query bodies now
	// go through a template where they previously went through Sprintf.
	`{{.MatchedJoin}}`,
	`{{template "x"}}`,
}

// TestSearchSpansSQLBindsHostileInput is the regression test for the property
// that makes this query safe: caller-controlled text becomes a bound argument,
// never SQL text.
//
// Worth pinning explicitly now that the query body is rendered by
// text/template. text/template does not escape anything -- that is html
// /template -- so it offers no protection of its own and never did; the
// protection is that values travel in args. The template change is neutral to
// that, and this test is what says so out loud rather than leaving the next
// reader to work it out.
//
// The `{{...}}` payloads cover the one genuinely new question: template data is
// rendered, not re-parsed, so a fragment containing template syntax cannot
// cause a second round of expansion.
func TestSearchSpansSQLBindsHostileInput(t *testing.T) {
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

			query, args, err := searchSpansSQL("00000000000000000000000000000099", criteria)
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
