//go:build !darwin && !linux && !windows

package store

import "os"

func publishNewDatabase(staged, path string) error {
	return os.Link(staged, path)
}
