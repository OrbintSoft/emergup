// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"
)

// Errors for a value the configuration cannot take.
var (
	errInvalidDuration  = errors.New("invalid duration")
	errNegativeDuration = errors.New("negative duration")
	errUnknownName      = errors.New("unknown")
)

// Duration is a length of time, written in a configuration file the way Go
// writes durations: "24h", "90m", "1h30m".
type Duration time.Duration

// UnmarshalText reads a duration such as "24h".
func (d *Duration) UnmarshalText(text []byte) error {
	parsed, err := time.ParseDuration(string(text))
	if err != nil {
		return fmt.Errorf("%w %q, expected for example \"24h\" or \"90m\"", errInvalidDuration, text)
	}

	if parsed < 0 {
		return fmt.Errorf("%w %q", errNegativeDuration, text)
	}

	*d = Duration(parsed)

	return nil
}

// String writes the duration the way Go does, for example "24h0m0s".
func (d Duration) String() string {
	return time.Duration(d).String()
}

// MarshalText writes the duration the way a configuration file does, without
// the zero minutes and seconds Go adds: "24h" rather than "24h0m0s".
func (d Duration) MarshalText() ([]byte, error) {
	text := d.String()

	if whole, isWhole := strings.CutSuffix(text, "m0s"); isWhole {
		text = whole + "m"
	}

	if whole, isWhole := strings.CutSuffix(text, "h0m"); isWhole {
		text = whole + "h"
	}

	return []byte(text), nil
}

// expected says what a duration looks like in a configuration file.
func (*Duration) expected() string {
	return `a duration such as "24h"`
}

// elevationMethodNames are the names of the elevation methods, in the order
// of their constants.
func elevationMethodNames() []string {
	return []string{"auto", "sudo", "doas", "run0"}
}

// String returns the name the configuration uses for the method.
func (m ElevationMethod) String() string {
	return nameOf("ElevationMethod", int(m), elevationMethodNames())
}

// MarshalText writes the name of the method.
func (m ElevationMethod) MarshalText() ([]byte, error) {
	return []byte(m.String()), nil
}

// expected says what an elevation method looks like in a configuration file.
func (*ElevationMethod) expected() string {
	return "one of " + strings.Join(elevationMethodNames(), ", ")
}

// UnmarshalText reads the name of an elevation method.
func (m *ElevationMethod) UnmarshalText(text []byte) error {
	index, err := indexOf("elevation method", string(text), elevationMethodNames())
	if err != nil {
		return err
	}

	*m = ElevationMethod(index)

	return nil
}

// liveRebuildModeNames are the names of the live-rebuild modes, in the order
// of their constants.
func liveRebuildModeNames() []string {
	return []string{"auto", "smart", "interval", "always"}
}

// String returns the name the configuration uses for the mode.
func (m LiveRebuildMode) String() string {
	return nameOf("LiveRebuildMode", int(m), liveRebuildModeNames())
}

// MarshalText writes the name of the mode.
func (m LiveRebuildMode) MarshalText() ([]byte, error) {
	return []byte(m.String()), nil
}

// expected says what a live-rebuild mode looks like in a configuration file.
func (*LiveRebuildMode) expected() string {
	return "one of " + strings.Join(liveRebuildModeNames(), ", ")
}

// UnmarshalText reads the name of a live-rebuild mode.
func (m *LiveRebuildMode) UnmarshalText(text []byte) error {
	index, err := indexOf("live-rebuild mode", string(text), liveRebuildModeNames())
	if err != nil {
		return err
	}

	*m = LiveRebuildMode(index)

	return nil
}

// nameOf returns names[index], or a Go-like rendering of a value that has no
// name, such as "LiveRebuildMode(7)".
func nameOf(typeName string, index int, names []string) string {
	if index < 0 || index >= len(names) {
		return fmt.Sprintf("%s(%d)", typeName, index)
	}

	return names[index]
}

// indexOf returns the position of name in names, or an error that lists the
// names a value of the given kind can take.
func indexOf(kind, name string, names []string) (int, error) {
	for index, candidate := range names {
		if candidate == name {
			return index, nil
		}
	}

	return 0, fmt.Errorf("%w %s %q, expected one of %s", errUnknownName, kind, name, strings.Join(names, ", "))
}

// The two halves of a HookPlace: when the hook runs, and the name that
// stands for the whole run.
const (
	hookBefore = "before"
	hookAfter  = "after"
	// RunName is the step of a HookPlace that stands for the whole run.
	RunName = "run"
)

// When returns "before" or "after", or "" for no place.
func (p HookPlace) When() string {
	when, _, _ := strings.Cut(string(p), "-")

	return when
}

// Step returns the name of the step, "run" for the whole run, or "" for no
// place.
func (p HookPlace) Step() string {
	_, step, _ := strings.Cut(string(p), "-")

	return step
}

// MarshalText writes the place as the configuration does.
func (p HookPlace) MarshalText() ([]byte, error) {
	return []byte(p), nil
}

// hookPlaceExpected says what a hook's place looks like in a configuration
// file; custom names the custom steps, when they are known.
func hookPlaceExpected(custom []string) string {
	expected := `"before-" or "after-" followed by run, a built-in step (` + strings.Join(StepNames(), ", ") +
		") or a custom step"
	if len(custom) > 0 {
		expected += " (" + strings.Join(custom, ", ") + ")"
	}

	return expected
}

// checkHookPlace rejects a place that is neither before nor after. The empty
// place, no place, is left to incompleteHooks, and the step a place names to
// misplacedHooks, as a later file may add the custom step it names.
func checkHookPlace(place HookPlace) error {
	if place != "" && place.When() != hookBefore && place.When() != hookAfter {
		return unknownHookPlace(place, nil)
	}

	return nil
}

// unknownHookPlace is the problem of a hook's place that names no step;
// custom names the custom steps, when they are known.
func unknownHookPlace(place HookPlace, custom []string) error {
	return fmt.Errorf("%w hook place %q, expected %s", errUnknownName, string(place), hookPlaceExpected(custom))
}

// checkAfter rejects a custom step's place that names no built-in step; the
// empty place, no place, is left to incompleteCustomSteps.
func checkAfter(after string) error {
	if after != "" && !slices.Contains(StepNames(), after) {
		return fmt.Errorf("%w step %q, expected a built-in step: %s", errUnknownName, after,
			strings.Join(StepNames(), ", "))
	}

	return nil
}

// StepNames returns the names of the built-in steps, as the configuration
// writes them, in the order a run performs them.
func StepNames() []string {
	var names []string

	for field := range reflect.TypeFor[Steps]().Fields() {
		names = append(names, tomlName(field))
	}

	return names
}

// hookFailureNames are the names of the hook failures, in the order of
// their constants.
func hookFailureNames() []string {
	return []string{"stop", "continue", "ask"}
}

// String returns the name the configuration uses for the failure.
func (f HookFailure) String() string {
	return nameOf("HookFailure", int(f), hookFailureNames())
}

// MarshalText writes the name of the failure.
func (f HookFailure) MarshalText() ([]byte, error) {
	return []byte(f.String()), nil
}

// expected says what a hook failure looks like in a configuration file.
func (*HookFailure) expected() string {
	return "one of " + strings.Join(hookFailureNames(), ", ")
}

// UnmarshalText reads the name of a hook failure.
func (f *HookFailure) UnmarshalText(text []byte) error {
	index, err := indexOf("on-failure", string(text), hookFailureNames())
	if err != nil {
		return err
	}

	*f = HookFailure(index)

	return nil
}
