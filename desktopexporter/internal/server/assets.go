//go:build !evals

package server

import (
	"embed"
	"io/fs"
)

//go:embed static
var assets embed.FS

func staticFS() (fs.FS, error) { return fs.Sub(assets, "static") }
