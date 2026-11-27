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
		{key: "elevation.keep-env", check: func(cfg *Config) error { return checkVariables(cfg.Elevation.KeepEnv) }},
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

		if problem := nameProblem(file, keys, prefix, "hook", name); problem != nil {
			problems = append(problems, problem)

			continue
		}

		hook := set.Hooks[name]
		problems = append(problems, keyProblems(file, keys, []keyCheck{
			{key: prefix + ".at", err: checkHookPlace(hook.At)},
			{key: prefix + ".command", err: checkTableCommand(hook.Command)},
			{key: prefix + ".remedy", err: checkTableCommand(hook.Remedy)},
			{key: prefix + ".remedy-status", err: checkRemedyStatus(hook.RemedyStatus)},
		})...)
	}

	return problems
}

// customStepProblems returns a problem for each custom step of set, what
// file alone decodes to, whose name a key cannot hold or is taken, and for
// each place and command of a custom step that keys, the keys the file
// sets, holds and that is wrong. As for hooks, a custom step that runs
// without them is found once every file is read, see incompleteCustomSteps.
func customStepProblems(file string, keys map[string]keyPlace, set *Config) []*Error {
	var problems []*Error

	for _, name := range slices.Sorted(maps.Keys(set.CustomSteps)) {
		prefix := customStepsTable + "." + name

		problem := nameProblem(file, keys, prefix, "custom step", name)
		if problem == nil {
			problem = takenNameProblem(file, keys, prefix, name)
		}

		if problem != nil {
			problems = append(problems, problem)

			continue
		}

		step := set.CustomSteps[name]
		problems = append(problems, keyProblems(file, keys, []keyCheck{
			{key: prefix + ".after", err: checkAfter(step.After)},
			{key: prefix + ".command", err: checkTableCommand(step.Command)},
		})...)
	}

	return problems
}

// nameProblem returns the problem of the table prefix of file, a kind of
// table named name, when a key cannot hold its name, or nil.
func nameProblem(file string, keys map[string]keyPlace, prefix, kind, name string) *Error {
	if isTableName(name) {
		return nil
	}

	place := firstPlace(keys, prefix+".")

	return &Error{
		File: file, Line: place.line, Column: place.column, Key: prefix,
		Err: message(fmt.Sprintf("%s name %q: use lowercase letters, digits and dashes", kind, name)),
	}
}

// takenNameProblem returns the problem of the custom step prefix of file,
// named name, when a hook's place could not tell it from a built-in step or
// from the whole run, or nil.
func takenNameProblem(file string, keys map[string]keyPlace, prefix, name string) *Error {
	var problem string

	switch {
	case name == RunName:
		problem = "hooks name the whole run so"
	case slices.Contains(StepNames(), name):
		problem = "a built-in step has it"
	default:
		return nil
	}

	place := firstPlace(keys, prefix+".")

	return &Error{
		File: file, Line: place.line, Column: place.column, Key: prefix,
		Err: message(fmt.Sprintf("custom step name %q: %s", name, problem)),
	}
}

// keyCheck is what is wrong, nil for nothing, with the value of a key.
type keyCheck struct {
	key string
	err error
}

// keyProblems returns a problem for each check of a key that keys, the keys
// file sets, holds and that is wrong.
func keyProblems(file string, keys map[string]keyPlace, checks []keyCheck) []*Error {
	var problems []*Error

	for _, check := range checks {
		if place, isSet := keys[check.key]; isSet && check.err != nil {
			problems = append(problems, &Error{
				File: file, Line: place.line, Column: place.column, Key: check.key, Err: check.err,
			})
		}
	}

	return problems
}

// checkTableCommand rejects the command of a hook or a custom step, or a
// hook's remedy, whose program is empty; an empty one is left to the checks
// made once every file is read.
func checkTableCommand(command []string) error {
	if len(command) == 0 {
		return nil
	}

	return checkCommand(command)
}

// maxExitStatus is the highest exit status a program can end with.
const maxExitStatus = 255

// checkRemedyStatus rejects, among the statuses for which a check offers its
// remedy, 0, the status of a check that passes, and a number that is no exit
// status.
func checkRemedyStatus(statuses []int) error {
	for _, status := range statuses {
		switch {
		case status == 0:
			return message("exit status 0 is a check that passes: list the statuses from 1 to 255 that the remedy fixes")
		case status < 0 || status > maxExitStatus:
			return message(fmt.Sprintf("exit status %d: an exit status is from 1 to 255", status))
		}
	}

	return nil
}

// isTableName reports whether name can name a hook or a custom step:
// lowercase letters, digits and dashes, starting with a letter or a digit,
// as the keys of the configuration are written, so that a dotted key names
// one table only.
func isTableName(name string) bool {
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

// configProblems returns the problems that only the whole configuration
// shows, once every file is read: those of the custom steps, then those of
// the hooks.
func configProblems(cfg Config, origins Origins) []error {
	return slices.Concat(incompleteCustomSteps(cfg, origins), misplacedHooks(cfg, origins), incompleteHooks(cfg, origins))
}

// missingValue is a value that a hook or a custom step that runs needs, and
// whether it is missing or does not go with the others.
type missingValue struct {
	key     string
	wrong   bool
	problem string
}

// tableProblems returns a problem for each value of the table prefix that
// is wrong, against the last file that writes the table.
func tableProblems(origins Origins, prefix string, values []missingValue) []error {
	var problems []error

	for _, value := range values {
		if value.wrong {
			problems = append(problems, &Error{
				File: origins.tables[prefix], Line: 0, Column: 0, Key: prefix + "." + value.key,
				Err: message(value.problem),
			})
		}
	}

	return problems
}

// incompleteCustomSteps returns a problem for each value of a custom step
// of cfg that runs which, once every file is read, is missing.
func incompleteCustomSteps(cfg Config, origins Origins) []error {
	var problems []error

	for _, name := range slices.Sorted(maps.Keys(cfg.CustomSteps)) {
		step := cfg.CustomSteps[name]
		if !step.Enabled {
			continue
		}

		problems = append(problems, tableProblems(origins, customStepsTable+"."+name, []missingValue{
			{"after", step.After == "", `missing: say which built-in step it follows, such as after = "preserved-rebuild"`},
			{"command", len(step.Command) == 0, "missing: give the program the step runs and its arguments"},
		})...)
	}

	return problems
}

// misplacedHooks returns a problem for each hook of cfg whose place names a
// step that, once every file is read, is neither a built-in step, nor a
// custom step, nor the whole run, where the file that sets the place sets
// it. A hook that is turned off is no exception: its place is wrong all the
// same.
func misplacedHooks(cfg Config, origins Origins) []error {
	var problems []error

	custom := slices.Sorted(maps.Keys(cfg.CustomSteps))

	for _, name := range slices.Sorted(maps.Keys(cfg.Hooks)) {
		place := cfg.Hooks[name].At

		step := place.Step()
		if place == "" || step == RunName || slices.Contains(StepNames(), step) || slices.Contains(custom, step) {
			continue
		}

		key := hooksTable + "." + name + ".at"
		where := origins.places[key]
		problems = append(problems, &Error{
			File: origins.keys[key], Line: where.line, Column: where.column, Key: key,
			Err: unknownHookPlace(place, custom),
		})
	}

	return problems
}

// incompleteHooks returns a problem for each value of a hook of cfg that
// runs which, once every file is read, is missing or does not go with the
// others.
func incompleteHooks(cfg Config, origins Origins) []error {
	var problems []error

	for _, name := range slices.Sorted(maps.Keys(cfg.Hooks)) {
		hook := cfg.Hooks[name]
		if !hook.Enabled {
			continue
		}

		problems = append(problems, tableProblems(origins, hooksTable+"."+name, []missingValue{
			{"at", hook.At == "", `missing: say where the hook runs, such as at = "before-update"`},
			{"command", len(hook.Command) == 0, "missing: give the program the hook runs and its arguments"},
			{
				"remedy", len(hook.Remedy) > 0 && hook.OnFailure != HookAsk,
				`only a hook that asks has a remedy: set on-failure = "ask"`,
			},
			{"remedy-elevated", hook.RemedyElevated && len(hook.Remedy) == 0, "there is no remedy to elevate"},
			{
				"remedy-interactive-only", hook.RemedyInteractiveOnly && len(hook.Remedy) == 0,
				"there is no remedy to run only when someone answers",
			},
			{
				"remedy-status", len(hook.RemedyStatus) > 0 && len(hook.Remedy) == 0,
				"there is no remedy to offer for these statuses",
			},
		})...)
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

// checkVariables rejects, besides what checkEntries rejects, an entry that
// names no environment variable, removed or not: sudo takes the names as a
// list separated by commas.
func checkVariables(entries Entries) error {
	if err := checkEntries(entries); err != nil {
		return err
	}

	for _, entry := range entries {
		name := strings.TrimPrefix(entry, "-")
		if entry != "-*" && !isVariableName(name) {
			return message(fmt.Sprintf("entry %q is not a variable name: use letters, digits and underscores, "+
				"not starting with a digit", entry))
		}
	}

	return nil
}

// isVariableName reports whether name can name an environment variable as
// a shell writes one: letters, digits and underscores, not starting with a
// digit.
func isVariableName(name string) bool {
	if name == "" || (name[0] >= '0' && name[0] <= '9') {
		return false
	}

	return !strings.ContainsFunc(name, func(char rune) bool {
		return char != '_' && (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9')
	})
}

// The tables of tables: those of the hooks and of the custom steps.
const (
	hooksTable       = "hooks"
	customStepsTable = "custom-steps"
)

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
