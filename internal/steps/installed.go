// SPDX-License-Identifier: GPL-3.0-or-later

package steps

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/OrbintSoft/emergup/internal/runner"
)

// InstalledPackages returns the installed packages of the system on root,
// each as its category and full name with its version ("app-misc/foo-1.2"),
// in order: those of the package database Portage names, a package being
// merged left out.
func InstalledPackages(ctx context.Context, run runner.Runner, root fs.FS) ([]string, error) {
	database, err := packageDatabase(ctx, run)
	if err != nil {
		return nil, err
	}

	return databaseEntries(root, database)
}

// databaseEntries returns the name of each package of the database at
// database on root, its category and full name, in order; a package being
// merged has no entry yet. A database that does not exist has none.
func databaseEntries(root fs.FS, database string) ([]string, error) {
	dir := strings.TrimPrefix(path.Clean(database), "/")

	categories, err := fs.ReadDir(root, dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("could not read the installed packages: %w", err)
	}

	var names []string

	for _, category := range categories {
		if !category.IsDir() {
			continue
		}

		entries, err := fs.ReadDir(root, path.Join(dir, category.Name()))
		if err != nil {
			return nil, fmt.Errorf("could not read the installed packages: %w", err)
		}

		for _, entry := range entries {
			if entry.IsDir() && !strings.HasPrefix(entry.Name(), mergingPrefix) {
				names = append(names, path.Join(category.Name(), entry.Name()))
			}
		}
	}

	return names, nil
}
