// SPDX-License-Identifier: GPL-3.0-or-later

// Package config describes the configuration of an emergup run and its
// generic defaults: the values used for anything the configuration files do
// not set.
package config

import "time"

// Config is the configuration of a run.
type Config struct {
	Elevation Elevation `toml:"elevation"`
	Steps     Steps     `toml:"steps"`
	Hooks     Hooks     `toml:"hooks"`
	Colors    Colors    `toml:"colors"`
	Runs      Runs      `toml:"runs"`
}

// Runs says how many run directories, each with the log of its run, are
// kept.
type Runs struct {
	// Keep is how many of the latest run directories are kept; 0 keeps them
	// all.
	Keep int `toml:"keep"`
}

// Elevation says how a command that needs root is run.
type Elevation struct {
	Method ElevationMethod `toml:"method"`
}

// ElevationMethod names the program that runs a command as root; the
// configuration writes it by name, see String.
type ElevationMethod int

// The elevation methods; ElevationAuto uses the first of sudo, doas and run0
// that is installed.
const (
	ElevationAuto ElevationMethod = iota
	ElevationSudo
	ElevationDoas
	ElevationRun0
)

// Entries is a list of entries, such as package atoms or repository names,
// that adds up across the configuration files like USE in make.conf: a later
// file adds entries, "-entry" removes one added earlier and "-*" clears the
// list. Every other list, a command line above all, is replaced as a whole.
type Entries []string

// Steps holds the built-in steps, in the order a run performs them.
type Steps struct {
	Sync             Sync        `toml:"sync"`
	Update           Update      `toml:"update"`
	LiveRebuild      LiveRebuild `toml:"live-rebuild"`
	PreservedRebuild Step        `toml:"preserved-rebuild"`
	ConfigFiles      Step        `toml:"config-files"`
	Depclean         Step        `toml:"depclean"`
}

// Step is what every step has: whether it runs, and the program it starts
// with its first arguments, to which the step appends the arguments it works
// out itself.
type Step struct {
	Enabled bool     `toml:"enabled"`
	Command []string `toml:"command"`
}

// Sync updates the repositories; the names of the repositories to sync are
// appended to its command.
type Sync struct {
	Step

	// MinInterval skips a repository synced more recently than this.
	MinInterval Duration `toml:"min-interval"`
	// AlwaysSync names the repositories synced on every run, whatever
	// MinInterval says.
	AlwaysSync Entries `toml:"always-sync"`
}

// Update updates a package set: its command is followed by Options, an
// --exclude for each entry of Exclude, and Target.
type Update struct {
	Step

	Options []string `toml:"options"`
	Target  string   `toml:"target"`
	// Exclude lists the packages left out of the update.
	Exclude Entries `toml:"exclude"`
}

// LiveRebuildMode chooses how the live packages are rebuilt; the
// configuration writes it by name, see String.
type LiveRebuildMode int

// The live-rebuild modes.
const (
	// LiveRebuildAuto is LiveRebuildSmart when smart-live-rebuild is
	// installed, LiveRebuildInterval otherwise.
	LiveRebuildAuto LiveRebuildMode = iota
	// LiveRebuildSmart runs smart-live-rebuild, which rebuilds the live
	// packages whose upstream has changed.
	LiveRebuildSmart
	// LiveRebuildInterval rebuilds each live package built longer ago than
	// the interval.
	LiveRebuildInterval
	// LiveRebuildAlways rebuilds every live package.
	LiveRebuildAlways
)

// LiveRebuild rebuilds the live packages. Command, followed by the packages
// or by the @live-rebuild set, serves the interval and always modes;
// SmartCommand serves the smart mode.
type LiveRebuild struct {
	Step

	Mode     LiveRebuildMode `toml:"mode"`
	Interval Duration        `toml:"interval"`

	SmartCommand []string `toml:"smart-command"`
}

// Hooks maps the name of each hook to the hook. A file adds a hook with a
// table of its own, [hooks.<name>], and a later file changes the values it
// sets of a hook an earlier one added.
type Hooks map[string]Hook

// Hook is a command run before or after the whole run or a step.
type Hook struct {
	Enabled bool `toml:"enabled"`
	// At is where the hook runs, such as before the update.
	At HookPlace `toml:"at"`
	// Command is the program the hook starts, with its arguments.
	Command []string `toml:"command"`
	// Elevated runs Command as root, as the commands of the steps that need
	// root are; a hook runs as the invoking user otherwise.
	Elevated bool `toml:"elevated"`
	// OnFailure says what a hook that fails does to the run.
	OnFailure HookFailure `toml:"on-failure"`
	// Remedy is the command a hook that asks offers to run when it fails,
	// before it runs again; RemedyElevated runs it as root.
	Remedy         []string `toml:"remedy"`
	RemedyElevated bool     `toml:"remedy-elevated"`
}

// HookPlace is where a hook runs, written "before-<step>" or
// "after-<step>", where the step is the name of a step or "run" for the
// whole run: "before-update", "after-run". The empty place is no place, that
// of a hook a file has not finished. Load checks the place a file writes, see
// checkHookPlace.
type HookPlace string

// HookFailure says what a hook that fails does to the run; the
// configuration writes it by name, see String.
type HookFailure int

// The hook failures.
const (
	// HookStop fails the run, as a failed step does.
	HookStop HookFailure = iota
	// HookContinue only shows a notice, and the run goes on.
	HookContinue
	// HookAsk makes the hook a check: its failure asks whether to run its
	// remedy and check again, to go on, or to abort the run.
	HookAsk
)

// NewHook returns what a hook is before a file sets its values: enabled,
// run as the invoking user, and stopping the run when it fails.
func NewHook() Hook {
	return Hook{
		Enabled: true, At: "", Command: []string{}, Elevated: false,
		OnFailure: HookStop, Remedy: []string{}, RemedyElevated: false,
	}
}

// emerge is Portage's command, which most built-in steps start.
const emerge = "emerge"

// defaultInterval is how old a sync or a live package's build may be before
// a run renews it.
const defaultInterval = Duration(24 * time.Hour)

// defaultKeep is how many run directories are kept: enough to look back
// at the last few weeks of updates, without filling the disk with their
// logs.
const defaultKeep = 10

// Default returns the generic defaults, a new value on every call. They suit
// any Gentoo system: every built-in step enabled, running Portage's own tools,
// nothing excluded or always synced, and no hooks. The colours are among the sixteen
// that every terminal's theme sets, so they suit light and dark themes.
func Default() Config {
	return Config{
		Elevation: Elevation{Method: ElevationAuto},
		Steps: Steps{
			Sync: Sync{
				Step:        Step{Enabled: true, Command: []string{emerge, "--sync"}},
				MinInterval: defaultInterval,
				AlwaysSync:  Entries{},
			},
			Update: Update{
				Step:    Step{Enabled: true, Command: []string{emerge}},
				Options: []string{"--verbose", "--update", "--deep", "--newuse", "--verbose-conflicts"},
				Target:  "@world",
				Exclude: Entries{},
			},
			LiveRebuild: LiveRebuild{
				Step:         Step{Enabled: true, Command: []string{emerge, "--oneshot"}},
				Mode:         LiveRebuildAuto,
				Interval:     defaultInterval,
				SmartCommand: []string{"smart-live-rebuild"},
			},
			PreservedRebuild: Step{Enabled: true, Command: []string{emerge, "@preserved-rebuild"}},
			ConfigFiles:      Step{Enabled: true, Command: []string{"dispatch-conf"}},
			Depclean:         Step{Enabled: true, Command: []string{emerge, "--depclean"}},
		},
		Hooks: Hooks{},
		Colors: Colors{
			Heading: style(bold, named(blue)),
			Prompt:  style(bold, named(cyan)),
			Success: style(0, named(green)),
			Skip:    style(0, named(yellow)),
			Failure: style(bold, named(red)),
			Notice:  style(0, named(magenta)),
		},
		Runs: Runs{Keep: defaultKeep},
	}
}
