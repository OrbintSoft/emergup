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
}

// Elevation says how a command that needs root is run.
type Elevation struct {
	Method ElevationMethod `toml:"method"`
}

// ElevationMethod names the program that runs a command as root.
type ElevationMethod string

// The elevation methods; ElevationAuto uses the first of sudo, doas and run0
// that is installed.
const (
	ElevationAuto ElevationMethod = "auto"
	ElevationSudo ElevationMethod = "sudo"
	ElevationDoas ElevationMethod = "doas"
	ElevationRun0 ElevationMethod = "run0"
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
	MinInterval time.Duration `toml:"min-interval"`
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

// LiveRebuildMode chooses how the live packages are rebuilt.
type LiveRebuildMode string

// The live-rebuild modes.
const (
	// LiveRebuildAuto is LiveRebuildSmart when smart-live-rebuild is
	// installed, LiveRebuildInterval otherwise.
	LiveRebuildAuto LiveRebuildMode = "auto"
	// LiveRebuildSmart runs smart-live-rebuild, which rebuilds the live
	// packages whose upstream has changed.
	LiveRebuildSmart LiveRebuildMode = "smart"
	// LiveRebuildInterval rebuilds each live package built longer ago than
	// the interval.
	LiveRebuildInterval LiveRebuildMode = "interval"
	// LiveRebuildAlways rebuilds every live package.
	LiveRebuildAlways LiveRebuildMode = "always"
)

// LiveRebuild rebuilds the live packages. Command, followed by the packages
// or by the @live-rebuild set, serves the interval and always modes;
// SmartCommand serves the smart mode.
type LiveRebuild struct {
	Step

	Mode     LiveRebuildMode `toml:"mode"`
	Interval time.Duration   `toml:"interval"`

	SmartCommand []string `toml:"smart-command"`
}

// emerge is Portage's command, which most built-in steps start.
const emerge = "emerge"

// defaultInterval is how old a sync or a live package's build may be before
// a run renews it.
const defaultInterval = 24 * time.Hour

// Default returns the generic defaults, a new value on every call. They suit
// any Gentoo system: every built-in step enabled, running Portage's own tools,
// and nothing excluded or always synced.
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
	}
}
