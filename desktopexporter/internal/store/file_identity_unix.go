//go:build unix

package store

import (
	"os"
	"syscall"
)

func pinViewerFile(path string) (*viewerFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	stat := info.Sys().(*syscall.Stat_t)
	return &viewerFile{f, uint64(stat.Dev), uint64(stat.Ino)}, nil
}
