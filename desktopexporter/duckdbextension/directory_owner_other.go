//go:build !unix

package duckdbextension

import "os"

// Non-Unix systems do not use Unix owner IDs.
func checkDirectoryOwner(string, os.FileInfo) error { return nil }
