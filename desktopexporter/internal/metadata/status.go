package metadata

import (
	"go.opentelemetry.io/collector/component"
)

var (
	Type      = component.MustNewType("desktop")
	ScopeName = "github.com/CtrlSpice/otel-desktop-viewer/desktopexporter"
)

const (
	TracesStability  = component.StabilityLevelDevelopment
	MetricsStability = component.StabilityLevelDevelopment
	LogsStability    = component.StabilityLevelDevelopment
)
