// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"io/fs"
	"maps"
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
	cfg, _, err := Inspect(ctx, fsys)

	return cfg, err
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
// problems is decoded over cfg, the lists of entries it sets are added up to
// the earlier ones, and origins records the values it sets.
func apply(fsys fs.FS, name string, cfg *Config, origins *Origins) []*Error {
	file := displayPath(name)

	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return []*Error{fileError(file, err)}
	}

	keys := fileKeys(data)
	set, decoded, undecoded := decodeAlone(file, data, keys)

	// A key that must be a string is said to need one, rather than what the
	// decoder makes of it.
	problems := withoutRepeatedKeys(textProblems(file, keys), decoded)

	if set != nil {
		decodedKeys := maps.Clone(keys)
		maps.DeleteFunc(decodedKeys, func(key string, _ keyPlace) bool { return undecoded[key] })

		problems = withoutRepeatedKeys(problems, valueProblems(file, decodedKeys, set))
		problems = withoutRepeatedKeys(problems, customStepProblems(file, decodedKeys, set))
		problems = withoutRepeatedKeys(problems, hookProblems(file, decodedKeys, set))
	}

	if len(problems) > 0 {
		return sortedProblems(problems)
	}

	lists := cfg.entryLists()
	earlier := make([]Entries, len(lists))

	for i, list := range lists {
		earlier[i] = slices.Clone(*list.entries)
	}

	addTables(&cfg.CustomSteps, set.CustomSteps, NewCustomStep)
	addTables(&cfg.Hooks, set.Hooks, NewHook)

	err = toml.NewDecoder(bytes.NewReader(data)).Decode(cfg)
	if err != nil {
		return []*Error{fileError(file, err)}
	}

	// A list the file does not set still holds the earlier entries, and
	// adding them to themselves changes nothing.
	for i, list := range lists {
		*list.entries = list.entries.appliedTo(earlier[i], origins.entriesOf(list.key), file)
	}

	origins.setBy(file, keys)
	origins.setTablesBy(file, customStepsTable, maps.Keys(set.CustomSteps))
	origins.setTablesBy(file, hooksTable, maps.Keys(set.Hooks))

	return nil
}

// decodeAlone decodes data, the contents of file that sets keys, on its own,
// and returns what it decodes to, every problem the decoder finds, and the
// keys left undecoded because of them.
//
// The decoder stops at the first value it cannot decode. The key-value of
// that value is then blanked out, every other byte kept in its place, and the
// rest decoded again, until the decoder goes through; the unknown keys it
// reports last, along with what the file decodes to. set is nil when the
// decoding cannot go on: at a syntax error, or at a problem outside any
// key-value, such as a table defined twice.
func decodeAlone(file string, data []byte, keys map[string]keyPlace) (*Config, []*Error, map[string]bool) {
	regions, isTOML := keyValueRegions(data)
	data = slices.Clone(data)
	undecoded := make(map[string]bool)

	var problems []*Error

	for {
		var set Config

		err := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields().Decode(&set)
		if err == nil {
			return &set, problems, undecoded
		}

		problems = append(problems, decodeProblems(file, keys, err)...)

		if _, isMissing := errors.AsType[*toml.StrictMissingError](err); isMissing {
			// The decoder reports the unknown keys once it has decoded the rest.
			return &set, problems, undecoded
		}

		var skipped bool
		if isTOML {
			regions, skipped = skipFailed(data, regions, err, keys, undecoded)
		}

		if !skipped {
			return nil, problems, undecoded
		}
	}
}

// skipFailed blanks out in data the region of regions where the decoding
// error err is, marks the keys there in undecoded, and returns the regions
// left and true; it returns false when err is in no region. A region is
// blanked once, so decoding again and again ends.
func skipFailed(data []byte, regions []region, err error, keys map[string]keyPlace,
	undecoded map[string]bool,
) ([]region, bool) {
	decodeErr, isDecode := errors.AsType[*toml.DecodeError](err)
	if !isDecode {
		return regions, false
	}

	line, column := decodeErr.Position()
	at := offsetAt(data, line, column)

	index := slices.IndexFunc(regions, func(r region) bool { return r.start <= at && at < r.end })
	if index < 0 {
		return regions, false
	}

	failed := regions[index]

	blank(data, failed)

	for key, place := range keys {
		if failed.start <= place.offset && place.offset < failed.end {
			undecoded[key] = true
		}
	}

	return slices.Delete(regions, index, index+1), true
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

// addTables adds to tables, a table of tables such as the hooks, each table
// of added it does not have yet, as fresh returns it, so that a file decoded
// over it changes only the values it sets of a new table too.
func addTables[M ~map[string]T, T any](tables *M, added M, fresh func() T) {
	if *tables == nil {
		*tables = M{}
	}

	for name := range added {
		if _, exists := (*tables)[name]; !exists {
			(*tables)[name] = fresh()
		}
	}
}

// entryList is a list of entries of a configuration and its key.
type entryList struct {
	key     string
	entries *Entries
}

// entryLists returns the lists of c that add up across files.
func (c *Config) entryLists() []entryList {
	return []entryList{
		{key: "steps.sync.always-sync", entries: &c.Steps.Sync.AlwaysSync},
		{key: "steps.update.exclude", entries: &c.Steps.Update.Exclude},
		{key: "elevation.keep-env", entries: &c.Elevation.KeepEnv},
	}
}

// appliedTo returns earlier changed by the entries e that file writes: an
// entry is added unless it is already there, "-entry" removes it and "-*"
// removes them all. origins, the file each entry came from, follows.
func (e Entries) appliedTo(earlier Entries, origins map[string]string, file string) Entries {
	result := slices.Clone(earlier)

	for _, entry := range e {
		removed, isRemoval := strings.CutPrefix(entry, "-")

		switch {
		case entry == "-*":
			result = result[:0]

			clear(origins)
		case isRemoval:
			result = slices.DeleteFunc(result, func(candidate string) bool { return candidate == removed })

			delete(origins, removed)
		case !slices.Contains(result, entry):
			result = append(result, entry)
			origins[entry] = file
		}
	}

	return result
}

// displayPath is how a path of the root file system is shown to the user.
func displayPath(name string) string {
	return "/" + name
}
