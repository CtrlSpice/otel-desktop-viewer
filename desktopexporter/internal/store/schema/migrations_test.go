package schema

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

func TestShippedMigrationChain(t *testing.T) {
	migrations, err := Migrations()
	require.NoError(t, err)
	_, err = Plan(Baseline, Version, migrations)
	require.NoError(t, err)
}

func TestMigrationPlan(t *testing.T) {
	chain := []Migration{{28, "028_first.sql", "select 1"}, {29, "029_second.sql", "select 2"}}
	for _, current := range []int{0, 27, 28, 29} {
		steps, err := Plan(current, 29, chain)
		require.NoError(t, err)
		if current == 0 {
			require.Empty(t, steps)
		} else {
			require.Equal(t, chain[current-27:], steps)
		}
	}
	for _, current := range []int{-1, 26, 30} {
		_, err := Plan(current, 29, chain)
		require.Error(t, err)
	}
	for name, chain := range map[string][]Migration{
		"missing":   {{28, "first", "select 1"}},
		"duplicate": {{28, "first", "select 1"}, {28, "second", "select 2"}},
		"unordered": {{29, "second", "select 2"}, {28, "first", "select 1"}},
		"empty":     {{28, "first", " "}, {29, "second", "select 2"}},
		"unnamed":   {{28, "", "select 1"}, {29, "second", "select 2"}},
	} {
		t.Run(name, func(t *testing.T) {
			// Broken releases must fail even for a fresh or current database.
			for _, current := range []int{0, 27, 29} {
				_, err := Plan(current, 29, chain)
				require.Error(t, err)
			}
		})
	}
}

func TestLoadMigrations(t *testing.T) {
	files := fstest.MapFS{
		"migrations/README.md":      {Data: []byte("instructions")},
		"migrations/029_second.sql": {Data: []byte("select 2")},
		"migrations/028_first.sql":  {Data: []byte("select 1")},
	}
	chain, err := loadMigrations(files)
	require.NoError(t, err)
	require.Equal(t, []Migration{{28, "028_first.sql", "select 1"}, {29, "029_second.sql", "select 2"}}, chain)
	for _, name := range []string{"28_first.sql", "028.sql", "028_.sql", "abc_first.sql", "028_first.txt"} {
		_, err := loadMigrations(fstest.MapFS{"migrations/" + name: {Data: []byte("select 1")}})
		require.Error(t, err, name)
	}
}
