// Package migrations embeds the forward SQL migrations so the API image can
// apply them at startup. Keeping the files embedded means the deployed image
// never needs a copy of the repository or a separate migration CLI.
package migrations

import (
	"embed"
	"io/fs"
	"sort"
	"strings"
)

//go:embed *.up.sql
var files embed.FS

// Migration is one forward migration.
type Migration struct {
	// Version is the file name without the ".up.sql" suffix, for example
	// "0001_identity". It is stored in the schema_migrations ledger.
	Version string
	SQL     string
}

// Up returns the forward migrations in file name order, which is also the
// order they must be applied.
func Up() ([]Migration, error) {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return nil, err
	}

	migrations := make([]Migration, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		body, err := files.ReadFile(name)
		if err != nil {
			return nil, err
		}
		migrations = append(migrations, Migration{
			Version: strings.TrimSuffix(name, ".up.sql"),
			SQL:     string(body),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})
	return migrations, nil
}
