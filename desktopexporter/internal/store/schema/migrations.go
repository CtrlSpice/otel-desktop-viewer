package schema

import (
	"embed"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
)

// Baseline is the oldest schema with a supported forward migration path.
const Baseline = 27

// Migration upgrades the preceding version to Version.
type Migration struct {
	Version int
	Name    string
	SQL     string
}

//go:embed migrations
var migrationFiles embed.FS

// Migrations loads the shared, build-independent migration chain.
func Migrations() ([]Migration, error) {
	return loadMigrations(migrationFiles)
}

func loadMigrations(files fs.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(files, "migrations")
	if err != nil {
		return nil, err
	}
	var migrations []Migration
	for _, entry := range entries {
		if entry.Name() == "README.md" {
			continue
		}
		prefix, description, ok := strings.Cut(entry.Name(), "_")
		version, err := strconv.Atoi(prefix)
		if entry.IsDir() || !ok || err != nil || len(prefix) != 3 || !strings.HasSuffix(description, ".sql") || len(description) <= 4 {
			return nil, fmt.Errorf("invalid migration filename %q: expected NNN_description.sql", entry.Name())
		}
		body, err := fs.ReadFile(files, "migrations/"+entry.Name())
		if err != nil {
			return nil, err
		}
		migrations = append(migrations, Migration{Version: version, Name: entry.Name(), SQL: string(body)})
	}
	return migrations, nil
}

// Plan validates the complete chain and selects pending steps without I/O.
// A zero current version means a fresh database, created directly at target.
func Plan(current, target int, migrations []Migration) ([]Migration, error) {
	if target < Baseline || len(migrations) != target-Baseline {
		return nil, fmt.Errorf("incomplete migration chain from schema %d to %d", Baseline, target)
	}
	for i, migration := range migrations {
		if migration.Version != Baseline+i+1 || migration.Name == "" || strings.TrimSpace(migration.SQL) == "" {
			return nil, fmt.Errorf("invalid migration at schema %d: expected a named, nonempty step to %d", migration.Version, Baseline+i+1)
		}
	}
	if current == 0 {
		return nil, nil
	}
	if current < Baseline || current > target {
		return nil, fmt.Errorf("schema %d is outside supported range %d–%d", current, Baseline, target)
	}
	return migrations[current-Baseline:], nil
}
