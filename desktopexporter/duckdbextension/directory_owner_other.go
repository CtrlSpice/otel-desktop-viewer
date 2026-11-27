//go:build windows

package duckdbextension

import "io/fs"

func checkDirectoryOwner(string, fs.FileInfo) error { return nil }
