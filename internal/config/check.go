// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"encoding"
	"fmt"
	"maps"
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

	for key, place := range keys {
		typ, known := schema[schemaKey(schema, key)]
		if !known || place.kind == unstable.String {
			continue
		}

		value, isText := reflect.TypeAssert[textValue](reflect.New(typ))
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

// hookProblems returns a problem for each hook of set, what file alone
// decodes to, whose name a key cannot hold, and for each place and command
// of a hook that keys, the keys the file sets, holds and that is wrong. A
// hook's place and command may be empty here, as a hook a file turns off
// needs neither: a hook that runs without them is found once every file is
// read, see incompleteHooks.
func hookProblems(file string, keys map[string]keyPlace, set *Config) []*Error {
	var problems []*Error

	for _, name := range slices.Sorted(maps.Keys(set.Hooks)) {
		prefix := hooksTable + "." + name

		if !isHookName(name) {
			place := firstPlace(keys, prefix+".")
			problems = append(problems, &Error{
				File: file, Line: place.line, Column: place.column, Key: prefix,
				Err: message(fmt.Sprintf("hook name %q: use lowercase letters, digits and dashes", name)),
			})

			continue
		}

		hook := set.Hooks[name]
		checks := []struct {
			key string
			err error
		}{
			{key: prefix + ".at", err: checkHookPlace(hook.At)},
			{key: prefix + ".command", err: checkHookCommand(hook.Command)},
			{key: prefix + ".remedy", err: checkHookCommand(hook.Remedy)},
		}

		for _, check := range checks {
			if place, isSet := keys[check.key]; isSet && check.err != nil {
				problems = append(problems, &Error{
					File: file, Line: place.line, Column: place.column, Key: check.key, Err: check.err,
				})
			}
		}
	}

	return problems
}

// checkHookCommand rejects a hook's command or remedy whose program is
// empty; an empty one is left to incompleteHooks.
func checkHookCommand(command []string) error {
	if len(command) == 0 {
		return nil
	}

	return checkCommand(command)
}

// isHookName reports whether name can name a hook: lowercase letters,
// digits and dashes, starting with a letter or a digit, as the keys of the
// configuration are written, so that a dotted key names one hook only.
func isHookName(name string) bool {
	if name == "" || name[0] == '-' {
		return false
	}

	return !strings.ContainsFunc(name, func(char rune) bool {
		return (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-'
	})
}

// firstPlace returns the first place of keys where a key that starts with
// prefix is set, or no place when there is none.
func firstPlace(keys map[string]keyPlace, prefix string) keyPlace {
	var first keyPlace

	for key, place := range keys {
		if strings.HasPrefix(key, prefix) && (first.line == 0 || place.offset < first.offset) {
			first = place
		}
	}

	return first
}

// incompleteHooks returns a problem for each value of a hook of cfg that
// runs which, once every file is read, is missing or does not go with the
// others, against the last file that writes its table.
func incompleteHooks(cfg Config, origins Origins) []error {
	var problems []error

	for _, name := range slices.Sorted(maps.Keys(cfg.Hooks)) {
		hook := cfg.Hooks[name]
		if !hook.Enabled {
			continue
		}

		prefix := hooksTable + "." + name

		checks := []struct {
			key     string
			wrong   bool
			problem string
		}{
			{"at", hook.At == "", `missing: say where the hook runs, such as at = "before-update"`},
			{"command", len(hook.Command) == 0, "missing: give the program the hook runs and its arguments"},
			{
				"remedy", len(hook.Remedy) > 0 && hook.OnFailure != HookAsk,
				`only a hook that asks has a remedy: set on-failure = "ask"`,
			},
			{"remedy-elevated", hook.RemedyElevated && len(hook.Remedy) == 0, "there is no remedy to elevate"},
		}

		for _, check := range checks {
			if check.wrong {
				problems = append(problems, &Error{
					File: origins.hooks[name], Line: 0, Column: 0, Key: prefix + "." + check.key,
					Err: message(check.problem),
				})
			}
		}
	}

	return problems
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

// hooksTable is the table of the hooks.
const hooksTable = "hooks"

// anyName stands, in a key of the schema, for the name of an entry of a
// table of tables such as the hooks: "hooks.*.command".
const anyName = "*"

// schemaKeys maps every dotted key of the configuration type typ to its Go
// type; an embedded struct adds its keys to the table it is embedded in, and
// a map of tables adds the keys of its tables under anyName.
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

		switch {
		case field.Type.Kind() == reflect.Struct && !isTextValue(field.Type):
			addSchemaKeys(keys, key, field.Type)
		case field.Type.Kind() == reflect.Map && field.Type.Elem().Kind() == reflect.Struct:
			keys[key+"."+anyName] = field.Type.Elem()
			addSchemaKeys(keys, key+"."+anyName, field.Type.Elem())
		}
	}
}

// schemaKey returns the key of schema that key, a dotted key of a file,
// sets: the name of an entry of a map of tables becomes anyName, so that
// "hooks.snapshot.at" is "hooks.*.at".
func schemaKey(schema map[string]reflect.Type, key string) string {
	parts := strings.Split(key, ".")

	for i := 1; i < len(parts); i++ {
		table, known := schema[strings.Join(parts[:i], ".")]
		if known && table.Kind() == reflect.Map {
			parts[i] = anyName
		}
	}

	return strings.Join(parts, ".")
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
