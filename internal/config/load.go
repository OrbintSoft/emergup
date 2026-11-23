// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"bytes"
	"cmp"
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
//
// Every file is checked before it is applied. Load reports the problems of
// all the files, each one an *Error joined with errors.Join, and returns a
// configuration only when there are none.
func Load(ctx context.Context, fsys fs.FS) (Config, error) {
	files, err := configurationFiles(fsys)
	if err != nil {
		return Config{}, err
	}

	cfg := Default()

	var problems []error

	for _, name := range files {
		err := ctx.Err()
		if err != nil {
			return Config{}, fmt.Errorf("reading the configuration: %w", err)
		}

		for _, problem := range apply(fsys, name, &cfg) {
			problems = append(problems, problem)
		}
	}

	if len(problems) > 0 {
		return Config{}, errors.Join(problems...)
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
		return nil, fileError(displayPath(MainFile), err)
	}

	// ReadDir returns the entries sorted by name, the lexical order.
	dropIns, err := fs.ReadDir(fsys, DropInDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fileError(displayPath(DropInDir), err)
	}

	for _, dropIn := range dropIns {
		if dropIn.IsDir() || !strings.HasSuffix(dropIn.Name(), dropInSuffix) {
			continue
		}

		files = append(files, path.Join(DropInDir, dropIn.Name()))
	}

	return files, nil
}

// apply checks the file name of fsys and returns its problems. A file without
// problems is decoded over cfg, and the lists of entries it sets are added up
// to the earlier ones.
func apply(fsys fs.FS, name string, cfg *Config) []*Error {
	file := displayPath(name)

	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return []*Error{fileError(file, err)}
	}

	keys := fileKeys(data)

	var set Config

	err = toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields().Decode(&set)
	if err != nil {
		return sortedProblems(withoutRepeatedKeys(checkFile(file, keys, nil), decodeProblems(file, keys, err)))
	}

	problems := checkFile(file, keys, &set)
	if len(problems) > 0 {
		return sortedProblems(problems)
	}

	lists := cfg.entryLists()
	earlier := make([]Entries, len(lists))

	for i, list := range lists {
		earlier[i] = slices.Clone(*list)
	}

	err = toml.NewDecoder(bytes.NewReader(data)).Decode(cfg)
	if err != nil {
		return []*Error{fileError(file, err)}
	}

	// A list the file does not set still holds the earlier entries, and
	// adding them to themselves changes nothing.
	for i, list := range lists {
		*list = list.appliedTo(earlier[i])
	}

	return nil
}

// withoutRepeatedKeys returns first and then the problems of second about a
// key that first does not already report.
func withoutRepeatedKeys(first, second []*Error) []*Error {
	result := slices.Clone(first)

	for _, problem := range second {
		repeated := problem.Key != "" && slices.ContainsFunc(first, func(earlier *Error) bool {
			return earlier.Key == problem.Key
		})
		if !repeated {
			result = append(result, problem)
		}
	}

	return result
}

// sortedProblems orders problems by their place in the file.
func sortedProblems(problems []*Error) []*Error {
	slices.SortStableFunc(problems, func(a, b *Error) int {
		return cmp.Or(cmp.Compare(a.Line, b.Line), cmp.Compare(a.Column, b.Column))
	})

	return problems
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
