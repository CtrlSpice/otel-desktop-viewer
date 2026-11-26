package store

import (
	"errors"
	"fmt"
	"os"
	"sync"
)

type fileIdentity struct{ device, id uint64 }

type fileOwner struct {
	path     string
	identity fileIdentity
	cached   bool
}

// Attached databases have independent DuckDB instances. Only direct opens of
// the same filename use duckdb-go's shared instance cache. Serialize file
// opening (including first creation), not queries or ingestion.
var openFiles = struct {
	sync.Mutex
	owners map[*fileOwner]struct{}
	// A Unix pin that unexpectedly opened an owned inode cannot close until
	// its last writer does: closing any descriptor releases process locks.
	extraPins map[fileIdentity][]*os.File
}{owners: make(map[*fileOwner]struct{}), extraPins: make(map[fileIdentity][]*os.File)}

var ErrFileInUse = errors.New("database file already open in this process")

func openFileStore(path string, cached bool, open func() (*Store, error)) (*Store, error) {
	openFiles.Lock()
	defer openFiles.Unlock()
	before, err := identifyFile(path)
	exists := err == nil
	if !exists && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for owner := range openFiles.owners {
		sameName := owner.path == path
		sameFile := exists && owner.identity == before
		if (sameName || sameFile) && !(sameName && sameFile && cached && owner.cached) {
			return nil, fmt.Errorf("%w: %s (active as %s)", ErrFileInUse, path, owner.path)
		}
	}
	s, err := open()
	if err != nil {
		return nil, err
	}
	id, err := identifyFile(path)
	if err != nil || (exists && id != before) || (s.pin != nil && s.pin.viewerFile != nil && s.pin.fileIdentity != id) {
		closeErr := s.Close() // Not registered yet.
		return nil, errors.Join(fmt.Errorf("database file changed during open: %s", path), err, closeErr)
	}
	owner := &fileOwner{path, id, cached}
	openFiles.owners[owner] = struct{}{}
	s.owner = owner
	return s, nil
}

func (o *fileOwner) release() {
	openFiles.Lock()
	defer openFiles.Unlock()
	delete(openFiles.owners, o)
	for other := range openFiles.owners {
		if other.identity == o.identity {
			return
		}
	}
	for _, f := range openFiles.extraPins[o.identity] {
		f.Close()
	}
	delete(openFiles.extraPins, o.identity)
}

// The caller holds openFiles during the pathname check and open. A rename by
// another process can still change which file this descriptor actually pins.
func pinUnownedFile(path string) (*viewerFile, error) {
	f, err := pinViewerFile(path)
	if err != nil {
		return nil, err
	}
	for owner := range openFiles.owners {
		if owner.identity == f.fileIdentity {
			openFiles.extraPins[f.fileIdentity] = append(openFiles.extraPins[f.fileIdentity], f.File)
			return nil, fmt.Errorf("%w: %s (active as %s)", ErrFileInUse, path, owner.path)
		}
	}
	return f, nil
}
