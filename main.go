// Program otel-desktop-viewer is an OpenTelemetry Collector binary.

package main

import (
	"log"
	"runtime/debug"
	"strings"

	"github.com/CtrlSpice/otel-desktop-viewer/internal/cli"
)

var version = "dev" // overridden by ldflags in release builds

func buildVersion() string {
	if version != "dev" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return version
	}
	modVersion := info.Main.Version
	if modVersion == "" || modVersion == "(devel)" {
		return version
	}
	return strings.TrimPrefix(modVersion, "v")
}

func main() {
	if err := cli.Run(buildVersion()); err != nil {
		log.Fatal(err)
	}
}
