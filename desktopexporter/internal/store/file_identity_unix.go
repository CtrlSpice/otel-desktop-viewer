//go:build unix

package store

import (
	"os"
	"syscall"
)

// stat does not open and close a descriptor: doing that for an alias of an
// active database could release the process's fcntl lock.
func identifyFile(path string) (fileIdentity, error) {
	info, err := os.Stat(path)
	if err != nil {
		return fileIdentity{}, err
	}
	return unixFileIdentity(info), nil
}

func unixFileIdentity(info os.FileInfo) fileIdentity {
	stat := info.Sys().(*syscall.Stat_t)
	return fileIdentity{uint64(stat.Dev), uint64(stat.Ino)}
}

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
	return &viewerFile{f, unixFileIdentity(info)}, nil
}
