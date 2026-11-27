// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// snapshotHook is a hook that a file adds, which the tests change.
const snapshotHook = "[hooks.snapshot]\nat = \"before-update\"\ncommand = [\"snap\", \"pre\"]\n"

// keyHookAt is the place of the hook h of the validation cases.
const keyHookAt = "hooks.h.at"

// askingHook is a check h with a remedy, whose last line is the fifth, to
// which the validation cases add one.
const askingHook = "[hooks.h]\nat = \"after-run\"\ncommand = [\"x\"]\non-failure = \"ask\"\nremedy = [\"fix\"]\n"

// keyRemedyStatus is the key of the remedy statuses of askingHook.
const keyRemedyStatus = "hooks.h.remedy-status"

// placeBeforeUpdate is the place of snapshotHook.
const placeBeforeUpdate = HookPlace("before-update")

// hookWith returns NewHook at place, running command, changed by change.
func hookWith(place HookPlace, command []string, change func(*Hook)) Hook {
	hook := NewHook()
	hook.At, hook.Command = place, command

	if change != nil {
		change(&hook)
	}

	return hook
}

func TestLoadAddsAHookWithItsDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := Load(t.Context(), fstest.MapFS{dropIn("10-snapshot.toml"): file(snapshotHook)})
	require.NoError(t, err)

	assert.Equal(t, Hooks{
		"snapshot": {
			Enabled: true, At: placeBeforeUpdate, Command: []string{"snap", "pre"}, Elevated: false,
			OnFailure: HookStop, Remedy: []string{}, RemedyElevated: false, RemedyStatus: []int{},
		},
	}, cfg.Hooks)
}

func TestLoadChangesOnlyTheValuesAFileSetsOfAHook(t *testing.T) {
	t.Parallel()

	cfg, err := Load(t.Context(), fstest.MapFS{
		MainFile: file(snapshotHook),
		dropIn("10-notify.toml"): file("[hooks.notify]\nat = \"after-run\"\ncommand = [\"notify\"]\n" +
			"on-failure = \"continue\"\n"),
		dropIn("20-disable.toml"): file("[hooks.snapshot]\nenabled = false\n"),
	})
	require.NoError(t, err)

	assert.Equal(t, Hooks{
		"snapshot": hookWith(placeBeforeUpdate, []string{"snap", "pre"}, func(h *Hook) { h.Enabled = false }),
		"notify":   hookWith("after-run", []string{"notify"}, func(h *Hook) { h.OnFailure = HookContinue }),
	}, cfg.Hooks)
}

func TestLoadReadsACheckWithItsRemedy(t *testing.T) {
	t.Parallel()

	cfg, err := Load(t.Context(), fstest.MapFS{
		MainFile: file("[hooks.backup]\nat = \"before-run\"\ncommand = [\"fresh\", \"20\"]\nelevated = true\n" +
			"on-failure = \"ask\"\nremedy = [\"redo-backup\", \"run\"]\nremedy-elevated = true\n" +
			"remedy-status = [1, 3]\n"),
	})
	require.NoError(t, err)

	assert.Equal(t, hookWith("before-run", []string{"fresh", "20"}, func(h *Hook) {
		h.Elevated, h.OnFailure, h.Remedy, h.RemedyElevated = true, HookAsk, []string{"redo-backup", "run"}, true
		h.RemedyStatus = []int{1, 3}
	}), cfg.Hooks["backup"])
}

func TestLoadReplacesTheRemedyStatusesOfAHook(t *testing.T) {
	t.Parallel()

	const check = "[hooks.backup]\nat = \"before-run\"\ncommand = [\"fresh\"]\non-failure = \"ask\"\n" +
		"remedy = [\"redo-backup\"]\nremedy-status = [1, 2]\n"

	tests := []struct {
		name   string
		dropIn string
		want   []int
	}{
		{"another list", "[hooks.backup]\nremedy-status = [3]\n", []int{3}},
		{"the default back", "[hooks.backup]\nremedy-status = []\n", []int{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := Load(t.Context(), fstest.MapFS{MainFile: file(check), dropIn("10.toml"): file(tt.dropIn)})
			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg.Hooks["backup"].RemedyStatus)
		})
	}
}

func TestLoadAcceptsATurnedOffHookAnotherFileDoesNotAdd(t *testing.T) {
	t.Parallel()

	cfg, err := Load(t.Context(), fstest.MapFS{dropIn("20-disable.toml"): file("[hooks.snapshot]\nenabled = false\n")})
	require.NoError(t, err)
	assert.False(t, cfg.Hooks["snapshot"].Enabled)
}

func TestLoadAcceptsEveryPlace(t *testing.T) {
	t.Parallel()

	for _, step := range append([]string{RunName}, StepNames()...) {
		for _, when := range []string{"before", "after"} {
			place := when + "-" + step

			cfg, err := Load(t.Context(), fstest.MapFS{
				MainFile: file("[hooks.h]\nat = \"" + place + "\"\ncommand = [\"x\"]\n"),
			})
			require.NoError(t, err, place)

			assert.Equal(t, when, cfg.Hooks["h"].At.When())
			assert.Equal(t, step, cfg.Hooks["h"].At.Step())
		}
	}
}

func TestStepNamesAreTheBuiltInStepsInOrder(t *testing.T) {
	t.Parallel()

	assert.Equal(t,
		[]string{"sync", "update", "live-rebuild", "preserved-rebuild", "config-files", "depclean"}, StepNames())
}

//nolint:funlen // one row per kind of problem, easier to read as one table
func TestLoadReportsWhereEachHookProblemIs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		line    int
		column  int
		key     string
		want    string
	}{
		{
			"unknown step", "[hooks.h]\nat = \"before-upgrade\"\ncommand = [\"x\"]\n",
			2, 1, keyHookAt, `unknown hook place "before-upgrade", expected "before-" or "after-" followed by run`,
		},
		{
			"no before or after", "[hooks.h]\nat = \"update\"\ncommand = [\"x\"]\n",
			2, 1, keyHookAt, `unknown hook place "update"`,
		},
		{
			"integer for a place", "[hooks.h]\nat = 1\ncommand = [\"x\"]\n",
			2, 6, keyHookAt, `expected "before-" or "after-" followed by run, a built-in step (sync, update, ` +
				"live-rebuild, preserved-rebuild, config-files, depclean) or a custom step, found an integer",
		},
		{
			"unknown failure", "[hooks.h]\nat = \"after-run\"\ncommand = [\"x\"]\non-failure = \"ignore\"\n",
			4, 14, "hooks.h.on-failure", `unknown on-failure "ignore", expected one of stop, continue, ask`,
		},
		{
			emptyProgram, "[hooks.h]\nat = \"after-run\"\ncommand = [\"\", \"x\"]\n",
			3, 1, "hooks.h.command", emptyProgram,
		},
		{
			"empty remedy program", "[hooks.h]\nat = \"after-run\"\ncommand = [\"x\"]\non-failure = \"ask\"\n" +
				"remedy = [\"\"]\n",
			5, 1, "hooks.h.remedy", emptyProgram,
		},
		{
			"status 0", askingHook + "remedy-status = [1, 0]\n",
			6, 1, keyRemedyStatus, "exit status 0 is a check that passes: list the statuses from 1 to 255",
		},
		{
			"status above 255", askingHook + "remedy-status = [256]\n",
			6, 1, keyRemedyStatus, "exit status 256: an exit status is from 1 to 255",
		},
		{
			"negative status", askingHook + "remedy-status = [-1]\n",
			6, 1, keyRemedyStatus, "exit status -1: an exit status is from 1 to 255",
		},
		{
			"string for a status", askingHook + "remedy-status = [\"1\"]\n",
			6, 18, keyRemedyStatus, "expected an integer, found a string",
		},
		{
			"integer for the statuses", askingHook + "remedy-status = 1\n",
			6, 17, keyRemedyStatus, "expected a list of integers, found an integer",
		},
		{
			"unknown key", "[hooks.h]\nat = \"after-run\"\ncommand = [\"x\"]\nwhen = \"now\"\n",
			4, 1, "hooks.h.when", unknownKey,
		},
		{
			"name with capitals", "[hooks.Snap]\nat = \"after-run\"\ncommand = [\"x\"]\n",
			2, 1, "hooks.Snap", `hook name "Snap": use lowercase letters, digits and dashes`,
		},
		{
			"name with a dot", "[hooks.\"a.b\"]\nat = \"after-run\"\ncommand = [\"x\"]\n",
			2, 1, "hooks.a.b", `hook name "a.b"`,
		},
		{
			"value for a hook", "[hooks]\nsnapshot = \"before-update\"\n",
			2, 12, "hooks.snapshot", "expected a table, found a string",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := Load(t.Context(), fstest.MapFS{dropIn("10-broken.toml"): file(tt.content)})
			assert.Equal(t, Config{}, cfg)

			list := problems(t, err)
			require.Len(t, list, 1)
			assert.Equal(t, brokenFile, list[0].File)
			assert.Equal(t, tt.line, list[0].Line, "line")
			assert.Equal(t, tt.column, list[0].Column, "column")
			assert.Equal(t, tt.key, list[0].Key)
			assert.ErrorContains(t, list[0], tt.want)
		})
	}
}

func TestLoadReportsAHookThatRunsWithoutAPlaceOrACommand(t *testing.T) {
	t.Parallel()

	_, err := Load(t.Context(), fstest.MapFS{
		MainFile:                 file("[hooks.snapshot]\nenabled = false\nat = \"before-update\"\n"),
		dropIn("10-enable.toml"): file("[hooks.snapshot]\nenabled = true\n\n[hooks.bare]\n"),
	})

	got := make([]string, 0, len(problems(t, err)))
	for _, problem := range problems(t, err) {
		got = append(got, problem.Error())
	}

	const enabling = "/etc/emergup/config.d/10-enable.toml"

	assert.Equal(t, []string{
		enabling + `: hooks.bare.at: missing: say where the hook runs, such as at = "before-update"`,
		enabling + ": hooks.bare.command: missing: give the program the hook runs and its arguments",
		enabling + ": hooks.snapshot.command: missing: give the program the hook runs and its arguments",
	}, got)
}

func TestLoadReportsAValueThatDoesNotGoWithTheOthersOfAHook(t *testing.T) {
	t.Parallel()

	_, err := Load(t.Context(), fstest.MapFS{
		MainFile: file("[hooks.backup]\nat = \"before-run\"\ncommand = [\"fresh\"]\non-failure = \"ask\"\n" +
			"remedy = [\"redo-backup\"]\n"),
		dropIn("10-stop.toml"): file("[hooks.backup]\non-failure = \"stop\"\n\n" +
			"[hooks.lone]\nat = \"after-run\"\ncommand = [\"x\"]\non-failure = \"ask\"\nremedy-elevated = true\n" +
			"remedy-status = [1]\n"),
	})

	got := make([]string, 0, len(problems(t, err)))
	for _, problem := range problems(t, err) {
		got = append(got, problem.Error())
	}

	const stopping = "/etc/emergup/config.d/10-stop.toml"

	assert.Equal(t, []string{
		stopping + `: hooks.backup.remedy: only a hook that asks has a remedy: set on-failure = "ask"`,
		stopping + ": hooks.lone.remedy-elevated: there is no remedy to elevate",
		stopping + ": hooks.lone.remedy-status: there is no remedy to offer for these statuses",
	}, got)
}

func TestInspectSaysWhereTheValuesOfAHookComeFrom(t *testing.T) {
	t.Parallel()

	_, origins, err := Inspect(t.Context(), fstest.MapFS{
		MainFile:          file(snapshotHook),
		dropIn("10.toml"): file("[hooks.snapshot]\non-failure = \"continue\"\n"),
	})
	require.NoError(t, err)

	assert.Equal(t, shownMain, origins.Of("hooks.snapshot.at"))
	assert.Equal(t, shownDropIn, origins.Of("hooks.snapshot.on-failure"))
	assert.Empty(t, origins.Of("hooks.snapshot.enabled"))
}

func TestWriteTOMLWritesTheHooksInTheOrderOfTheirNames(t *testing.T) {
	t.Parallel()

	cfg, origins, err := Inspect(t.Context(), fstest.MapFS{
		MainFile:          file(snapshotHook),
		dropIn("10.toml"): file("[hooks.notify]\nat = \"after-run\"\ncommand = [\"notify\"]\n"),
	})
	require.NoError(t, err)

	const defaults = "elevated = false  # default\non-failure = \"stop\"  # default\nremedy = []  # default\n" +
		"remedy-elevated = false  # default\nremedy-status = []  # default\n"

	out := shown(t, cfg, origins)
	assert.Contains(t, out, "[steps.depclean]\n")
	assert.Contains(t, out, "\n\n[hooks.notify]\nenabled = true  # default\nat = \"after-run\"  # "+shownDropIn+"\n"+
		"command = [\"notify\"]  # "+shownDropIn+"\n"+defaults+"\n"+
		"[hooks.snapshot]\nenabled = true  # default\nat = \"before-update\"  # "+shownMain+"\n"+
		"command = [\"snap\", \"pre\"]  # "+shownMain+"\n"+defaults+"\n[colors]\n")
}

func TestWriteTOMLReadsBackTheHooks(t *testing.T) {
	t.Parallel()

	cfg, origins, err := Inspect(t.Context(), fstest.MapFS{
		MainFile: file(snapshotHook + "\n[hooks.notify]\nat = \"after-run\"\ncommand = [\"n\"]\n" +
			"\n[hooks.backup]\nat = \"before-run\"\ncommand = [\"fresh\"]\nelevated = true\non-failure = \"ask\"\n" +
			"remedy = [\"redo\"]\nremedy-elevated = true\nremedy-status = [1, 2]\n"),
		dropIn("10.toml"): file("[hooks.snapshot]\nenabled = false\n\n[hooks.off]\nenabled = false\n"),
	})
	require.NoError(t, err)

	back, err := Load(t.Context(), fstest.MapFS{MainFile: file(shown(t, cfg, origins))})
	require.NoError(t, err)
	assert.Equal(t, cfg, back)
}
