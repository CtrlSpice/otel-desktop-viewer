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
}{owners: make(map[*fileOwner]struct{})}

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
	if err != nil || (exists && id != before) || (s.pin != nil && s.pin.fileIdentity != id) {
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
}
