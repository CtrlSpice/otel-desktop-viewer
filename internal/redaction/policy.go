// Package redaction shares the viewer's Collector policy with record lookups.
package redaction

import _ "embed"

//go:embed policy.yaml
var Policy string
