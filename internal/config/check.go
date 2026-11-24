// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"encoding"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"unicode"

	"github.com/pelletier/go-toml/v2/unstable"
)

// textValue is a value that a file writes as a string, read by UnmarshalText.
// The decoder would also store a TOML integer in one without calling
// UnmarshalText, so a file is checked to give these keys a string.
type textValue interface {
	encoding.TextUnmarshaler

	expected() string
}

// textProblems returns a problem for each key of textKeys that file gives
// something else than a string, which decoding does not report; keys are the
// keys the file sets.
func textProblems(file string, keys map[string]keyPlace) []*Error {
	var problems []*Error

	schema := schemaKeys(reflect.TypeFor[Config]())

	for _, key := range textKeys() {
		place, isSet := keys[key]
		if !isSet || place.kind == unstable.String {
			continue
		}

		value, isText := reflect.TypeAssert[textValue](reflect.New(schema[key]))
		if !isText {
			continue
		}

		problems = append(problems, &Error{
			File: file, Line: place.line, Column: place.column, Key: key,
			Err: message(fmt.Sprintf("expected %s, found %s", value.expected(), describeKind(place.kind))),
		})
	}

	return problems
}

// valueProblems returns a problem for each value of file that valueRules
// reject, which decoding does not; keys are the keys the file sets and set
// is what the file alone decodes to.
func valueProblems(file string, keys map[string]keyPlace, set *Config) []*Error {
	var problems []*Error

	for _, rule := range valueRules() {
		place, isSet := keys[rule.key]
		if !isSet {
			continue
		}

		err := rule.check(set)
		if err != nil {
			problems = append(problems, &Error{File: file, Line: place.line, Column: place.column, Key: rule.key, Err: err})
		}
	}

	return problems
}

// valueRule checks the value a file gives to one key.
type valueRule struct {
	key   string
	check func(cfg *Config) error
}

// valueRules are the checks on values that decoding alone does not reject.
func valueRules() []valueRule {
	return []valueRule{
		commandRule("steps.sync.command", func(cfg *Config) []string { return cfg.Steps.Sync.Command }),
		commandRule("steps.update.command", func(cfg *Config) []string { return cfg.Steps.Update.Command }),
		commandRule("steps.live-rebuild.command", func(cfg *Config) []string { return cfg.Steps.LiveRebuild.Command }),
		commandRule("steps.live-rebuild.smart-command",
			func(cfg *Config) []string { return cfg.Steps.LiveRebuild.SmartCommand }),
		commandRule("steps.preserved-rebuild.command",
			func(cfg *Config) []string { return cfg.Steps.PreservedRebuild.Command }),
		commandRule("steps.config-files.command", func(cfg *Config) []string { return cfg.Steps.ConfigFiles.Command }),
		commandRule("steps.depclean.command", func(cfg *Config) []string { return cfg.Steps.Depclean.Command }),
		{key: "steps.update.target", check: func(cfg *Config) error { return checkTarget(cfg.Steps.Update.Target) }},
		{key: "steps.update.exclude", check: func(cfg *Config) error { return checkEntries(cfg.Steps.Update.Exclude) }},
		{key: "steps.sync.always-sync", check: func(cfg *Config) error { return checkEntries(cfg.Steps.Sync.AlwaysSync) }},
		{key: "runs.keep", check: func(cfg *Config) error { return checkKeep(cfg.Runs.Keep) }},
	}
}

// commandRule checks the command line that command returns.
func commandRule(key string, command func(cfg *Config) []string) valueRule {
	return valueRule{key: key, check: func(cfg *Config) error { return checkCommand(command(cfg)) }}
}

// checkCommand rejects a command line without a program to run.
func checkCommand(command []string) error {
	switch {
	case len(command) == 0:
		return message("empty command: give the program to run and its first arguments")
	case command[0] == "":
		return message("empty program: the first element of the command is the program to run")
	}

	return nil
}

// checkTarget rejects an update with nothing to update.
func checkTarget(target string) error {
	if target == "" {
		return message(`empty target: give a package set such as "@world"`)
	}

	return nil
}

// checkKeep rejects a negative number of runs to keep.
func checkKeep(keep int) error {
	if keep < 0 {
		return message("must be 0 or more; 0 keeps every run")
	}

	return nil
}

// checkEntries rejects an entry that names nothing, and an entry with a
// space, which is several entries written as one.
func checkEntries(entries Entries) error {
	for _, entry := range entries {
		switch {
		case entry == "":
			return message("empty entry")
		case entry == "-":
			return message(`"-" removes nothing: write "-entry" to remove one entry or "-*" to remove them all`)
		case strings.ContainsFunc(entry, unicode.IsSpace):
			return message(fmt.Sprintf("entry %q contains a space: write each one as its own entry", entry))
		}
	}

	return nil
}

// schemaKeys maps every dotted key of the configuration type typ to its Go
// type; an embedded struct adds its keys to the table it is embedded in.
func schemaKeys(typ reflect.Type) map[string]reflect.Type {
	keys := make(map[string]reflect.Type)
	addSchemaKeys(keys, "", typ)

	return keys
}

func addSchemaKeys(keys map[string]reflect.Type, table string, typ reflect.Type) {
	for field := range typ.Fields() {
		if field.Anonymous {
			addSchemaKeys(keys, table, field.Type)

			continue
		}

		name, _, _ := strings.Cut(field.Tag.Get("toml"), ",")

		key := name
		if table != "" {
			key = table + "." + name
		}

		keys[key] = field.Type

		if field.Type.Kind() == reflect.Struct && !isTextValue(field.Type) {
			addSchemaKeys(keys, key, field.Type)
		}
	}
}

// textKeys are the keys whose value must be a TOML string, in sorted order.
func textKeys() []string {
	var keys []string

	for key, typ := range schemaKeys(reflect.TypeFor[Config]()) {
		if isTextValue(typ) {
			keys = append(keys, key)
		}
	}

	slices.Sort(keys)

	return keys
}

func isTextValue(typ reflect.Type) bool {
	return reflect.PointerTo(typ).Implements(reflect.TypeFor[textValue]())
}

// describeKind names the kind of a TOML value the way the messages do.
func describeKind(kind unstable.Kind) string {
	names := map[unstable.Kind]string{
		unstable.String:        "a string",
		unstable.Integer:       anInteger,
		unstable.Float:         "a number",
		unstable.Bool:          aBool,
		unstable.Array:         aList,
		unstable.InlineTable:   aTable,
		unstable.LocalDate:     "a date",
		unstable.LocalTime:     "a time",
		unstable.LocalDateTime: "a date and time",
		unstable.DateTime:      "a date and time",
	}

	name, known := names[kind]
	if !known {
		return "a " + kind.String()
	}

	return name
}
