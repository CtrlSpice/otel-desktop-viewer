package store

import (
	"os"

	"golang.org/x/sys/windows"
)

func identifyFile(path string) (fileIdentity, error) {
	f, err := pinViewerFile(path)
	if err != nil {
		return fileIdentity{}, err
	}
	return f.fileIdentity, f.Close()
}

func pinViewerFile(path string) (*viewerFile, error) {
	name, err := windows.UTF16PtrFromString(path)
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
