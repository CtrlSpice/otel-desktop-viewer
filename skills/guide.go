package skills

import _ "embed"

//go:embed otel-desktop-viewer/SKILL.md
var guide string

func Guide() string {
	return guide
}
