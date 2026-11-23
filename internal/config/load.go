// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// MainFile and DropInDir locate the configuration in the file system given
// to Load, whose root is the root of the system.
const (
	MainFile  = "etc/emergup/config.toml"
	DropInDir = "etc/emergup/config.d"
)

// dropInSuffix is the extension a file of DropInDir needs to be read.
const dropInSuffix = ".toml"

// Load reads the configuration from fsys, the root file system: the generic
// defaults, changed by MainFile and then by every *.toml file of DropInDir
// in lexical order. A file changes only the values it sets, and the lists of
// entries add up as Entries describes. A missing MainFile or DropInDir is
// not an error: without either, the configuration is the generic defaults.
func Load(ctx context.Context, fsys fs.FS) (Config, error) {
	files, err := configurationFiles(fsys)
	if err != nil {
		return Config{}, err
	}

	cfg := Default()

	for _, name := range files {
		err := ctx.Err()
		if err != nil {
			return Config{}, fmt.Errorf("reading the configuration: %w", err)
		}

		err = apply(fsys, name, &cfg)
		if err != nil {
			return Config{}, err
		}
	}

	return cfg, nil
}

// configurationFiles lists the files Load reads, in the order it reads them.
func configurationFiles(fsys fs.FS) ([]string, error) {
	var files []string

	_, err := fs.Stat(fsys, MainFile)

	switch {
	case err == nil:
		files = append(files, MainFile)
	case !errors.Is(err, fs.ErrNotExist):
		return nil, fmt.Errorf("%s: %w", displayPath(MainFile), err)
	}

	// ReadDir returns the entries sorted by name, the lexical order.
	dropIns, err := fs.ReadDir(fsys, DropInDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%s: %w", displayPath(DropInDir), err)
	}

	for _, dropIn := range dropIns {
		if dropIn.IsDir() || !strings.HasSuffix(dropIn.Name(), dropInSuffix) {
			continue
		}

		files = append(files, path.Join(DropInDir, dropIn.Name()))
	}

	return files, nil
}

// apply decodes the file name of fsys over cfg, then adds up the lists of
// entries the file sets to the ones cfg had before.
func apply(fsys fs.FS, name string, cfg *Config) error {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return fmt.Errorf("%s: %w", displayPath(name), err)
	}

	lists := cfg.entryLists()
	earlier := make([]Entries, len(lists))

	for i, list := range lists {
		earlier[i] = slices.Clone(*list)
	}

	err = toml.NewDecoder(bytes.NewReader(data)).Decode(cfg)
	if err != nil {
		return fmt.Errorf("%s: %w", displayPath(name), err)
	}

	// A list the file does not set still holds the earlier entries, and
	// adding them to themselves changes nothing.
	for i, list := range lists {
		*list = list.appliedTo(earlier[i])
	}

	return nil
}

// entryLists returns the lists of c that add up across files.
func (c *Config) entryLists() []*Entries {
	return []*Entries{
		&c.Steps.Sync.AlwaysSync,
		&c.Steps.Update.Exclude,
	}
}

// appliedTo returns earlier changed by the entries e of a later file: an
// entry is added unless it is already there, "-entry" removes it and "-*"
// removes them all.
func (e Entries) appliedTo(earlier Entries) Entries {
	result := slices.Clone(earlier)

	for _, entry := range e {
		removed, isRemoval := strings.CutPrefix(entry, "-")

		switch {
		case entry == "-*":
			result = result[:0]
		case isRemoval:
			result = slices.DeleteFunc(result, func(candidate string) bool { return candidate == removed })
		case !slices.Contains(result, entry):
			result = append(result, entry)
		}
	}

	return result
}

// displayPath is how a path of the root file system is shown to the user.
func displayPath(name string) string {
	return "/" + name
}
