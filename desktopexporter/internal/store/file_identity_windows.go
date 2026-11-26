package store

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func identifyFile(path string) (fileIdentity, error) {
	f, err := pinViewerFile(path)
	if err != nil {
		return fileIdentity{}, err
	}
	return f.fileIdentity, f.Close()
}

// The store supplies absolute paths. Use the extended Win32 spelling at the
// system call boundary so long paths do not depend on host policy.
func windowsFilename(path string) (*uint16, error) {
	if len(path) >= 248 && filepath.IsAbs(path) && !strings.HasPrefix(path, `\\?\`) && !strings.HasPrefix(path, `\\.\`) {
		path = filepath.Clean(path)
		if strings.HasPrefix(path, `\\`) {
			path = `\\?\UNC\` + path[2:]
		} else {
			path = `\\?\` + path
		}
	}
	return windows.UTF16PtrFromString(path)
}

func publishNewDatabase(staged, path string) error {
	from, err := windowsFilename(staged)
	if err != nil {
		return err
	}
	to, err := windowsFilename(path)
	if err != nil {
		return err
	}
	// No REPLACE_EXISTING or COPY_ALLOWED: same-volume rename only.
	return windows.MoveFileEx(from, to, 0)
}

func pinViewerFile(path string) (*viewerFile, error) {
	name, err := windowsFilename(path)
	if err != nil {
		return nil, err
	}
	// Metadata-only access pins the file ID without conflicting with DuckDB's
	// exclusive read/write handle. Keep renames possible during inspection.
	h, err := windows.CreateFile(name, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		windows.CloseHandle(h)
		return nil, &os.PathError{Op: "stat", Path: path, Err: err}
	}
	return &viewerFile{os.NewFile(uintptr(h), path), fileIdentity{uint64(info.VolumeSerialNumber),
		uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow)}}, nil
}
