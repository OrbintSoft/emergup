// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"reflect"
)

// Origins says where the values of a configuration come from.
type Origins struct {
	// Files are the configuration files read, in the order they were read,
	// as shown to the user.
	Files []string

	keys map[string]string
	// places holds where the file of keys sets each key.
	places  map[string]keyPlace
	entries map[string]map[string]string
	// tables maps each table of a table of tables, such as "hooks.snapshot",
	// to the last file that writes it, even one that sets none of its values.
	tables map[string]string
}

// Of returns the file that last set key, or "" when key keeps its generic
// default.
func (o Origins) Of(key string) string {
	return o.keys[key]
}

// OfEntry returns the file that added entry to the list key, or "" when the
// entry is one of the generic defaults.
func (o Origins) OfEntry(key, entry string) string {
	return o.entries[key][entry]
}

// Inspect reads the configuration like Load, and also says where each of its
// values comes from.
func Inspect(ctx context.Context, fsys fs.FS) (Config, Origins, error) {
	origins := Origins{
		Files: nil, keys: make(map[string]string), places: make(map[string]keyPlace),
		entries: make(map[string]map[string]string), tables: make(map[string]string),
	}

	files, err := configurationFiles(fsys)
	if err != nil {
		return Config{}, Origins{}, err
	}

	cfg := Default()

	var problems []error

	for _, name := range files {
		err := ctx.Err()
		if err != nil {
			return Config{}, Origins{}, fmt.Errorf("reading the configuration: %w", err)
		}

		origins.Files = append(origins.Files, displayPath(name))

		for _, problem := range apply(fsys, name, &cfg, &origins) {
			problems = append(problems, problem)
		}
	}

	if len(problems) == 0 {
		problems = configProblems(cfg, origins)
	}

	if len(problems) > 0 {
		return Config{}, Origins{}, errors.Join(problems...)
	}

	return cfg, origins, nil
}

// setBy records file as the origin of every value that keys, the keys the
// file sets, holds, and where it sets it; a key of a table is not a value of
// its own. Origins holds maps, so a copy records into the same ones.
func (o Origins) setBy(file string, keys map[string]keyPlace) {
	schema := schemaKeys(reflect.TypeFor[Config]())

	for key, place := range keys {
		typ, known := schema[schemaKey(schema, key)]
		if known && (typ.Kind() != reflect.Struct || isTextValue(typ)) {
			o.keys[key], o.places[key] = file, place
		}
	}
}

// setTablesBy records file as the last file that writes each of the tables
// that names gives of a table of tables, such as the hooks.
func (o Origins) setTablesBy(file, table string, names iter.Seq[string]) {
	for name := range names {
		o.tables[table+"."+name] = file
	}
}

// entriesOf returns the origins of the entries of the list key, creating
// them when the list has none yet.
func (o Origins) entriesOf(key string) map[string]string {
	origins, exist := o.entries[key]
	if !exist {
		origins = make(map[string]string)
		o.entries[key] = origins
	}

	return origins
}
