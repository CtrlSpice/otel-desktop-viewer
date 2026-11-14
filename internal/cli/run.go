package cli

import "github.com/CtrlSpice/otel-desktop-viewer/internal/collector"

// Run dispatches to the interactive CLI or the Windows service handler.
func Run(version string) error {
	return run(collector.Settings(version))
}
