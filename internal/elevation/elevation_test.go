// SPDX-License-Identifier: GPL-3.0-or-later

package elevation_test

import (
	"os"
	"os/exec"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/emergup/internal/config"
	"github.com/OrbintSoft/emergup/internal/elevation"
	"github.com/OrbintSoft/emergup/internal/runner"
	"github.com/OrbintSoft/emergup/internal/runner/runnertest"
)

// The user ids of the tests.
const (
	root = 0
	user = 1000
)

// The elevation tools.
const (
	sudo = "sudo"
	doas = "doas"
	run0 = "run0"
)

// worldUpdate returns the command line of a @world update.
func worldUpdate() []string {
	return []string{"emerge", "--update", "@world"}
}

// fakeSystem is a System with the given effective user id, on which only the
// installed programs are found.
type fakeSystem struct {
	euid      int
	installed []string
}

func (f fakeSystem) LookPath(file string) (string, error) {
	if slices.Contains(f.installed, file) {
		return "/usr/bin/" + file, nil
	}

	return "", exec.ErrNotFound
}

func (f fakeSystem) Geteuid() int {
	return f.euid
}

func (fakeSystem) LookupEnv(name string) (string, bool) {
	return environment(name)
}

func TestResolve(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		method    config.ElevationMethod
		system    fakeSystem
		want      string
		wantError string
	}{
		{"root needs nothing", config.ElevationAuto, fakeSystem{root, nil}, "", ""},
		{"root needs nothing, even with a method set", config.ElevationDoas, fakeSystem{root, nil}, "", ""},
		{"auto prefers sudo", config.ElevationAuto, fakeSystem{user, []string{run0, doas, sudo}}, sudo, ""},
		{"auto then doas", config.ElevationAuto, fakeSystem{user, []string{run0, doas}}, doas, ""},
		{"auto then run0", config.ElevationAuto, fakeSystem{user, []string{run0}}, run0, ""},
		{
			"auto with no tool", config.ElevationAuto,
			fakeSystem{user, nil},
			"",
			"no elevation tool found: install sudo, doas or run0, or run emergup as root",
		},
		{"a method set", config.ElevationDoas, fakeSystem{user, []string{sudo, doas}}, doas, ""},
		{"run0 set", config.ElevationRun0, fakeSystem{user, []string{run0}}, run0, ""},
		{
			"a method set but missing", config.ElevationDoas,
			fakeSystem{user, []string{sudo}},
			"",
			"doas is not installed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			program, err := elevation.Resolve(tt.method, tt.system)
			if tt.wantError != "" {
				require.ErrorContains(t, err, tt.wantError)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, program)
		})
	}
}

func TestIsRoot(t *testing.T) {
	t.Parallel()
	assert.True(t, elevation.IsRoot(fakeSystem{root, nil}))
	assert.False(t, elevation.IsRoot(fakeSystem{user, nil}))
}

func TestRunnerElevatesOnlyTheCommandsThatNeedRoot(t *testing.T) {
	t.Parallel()

	var fake runnertest.Fake

	elevated := elevation.Runner{Next: &fake, Program: sudo, Keep: nil, LookupEnv: nil}

	update := runner.Command{Args: worldUpdate(), Mode: runner.Stream, Root: true}
	query := runner.Command{Args: []string{"portageq", "envvar", "ARCH"}, Mode: runner.Capture}

	_, err := elevated.Run(t.Context(), update)
	require.NoError(t, err)
	_, err = elevated.Run(t.Context(), query)
	require.NoError(t, err)
	_, err = elevated.Run(t.Context(), update)
	require.NoError(t, err)

	commands := fake.Commands()
	require.Len(t, commands, 3)
	assert.Equal(t, append([]string{sudo}, worldUpdate()...), commands[0].Args)
	assert.Equal(t, runner.Stream, commands[0].Mode)
	assert.Equal(t, query, commands[1])
	assert.Equal(t, commands[0], commands[2], "the prefix does not pile up from one command to the next")
	assert.Equal(t, worldUpdate(), update.Args, "the caller's command is left as it was")
}

func TestRunnerAsRootRunsEverythingDirectly(t *testing.T) {
	t.Parallel()

	var fake runnertest.Fake

	direct := elevation.Runner{Next: &fake, Program: "", Keep: []string{editor}, LookupEnv: environment}
	update := runner.Command{Args: worldUpdate(), Mode: runner.Stream, Root: true, Env: []string{"EMERGUP_HOOK=h"}}

	_, err := direct.Run(t.Context(), update)
	require.NoError(t, err)
	assert.Equal(t, []runner.Command{update}, fake.Commands())
}

// The variables of the tests.
const (
	editor = "EDITOR"
	visual = "VISUAL"
	pager  = "PAGER"
	hookIs = "EMERGUP_HOOK=backup"
	stepIs = "EMERGUP_STEP=run"
)

// environment looks a variable up in the environment of the tests, where
// EDITOR is vim and PAGER is less.
func environment(name string) (string, bool) {
	value, isSet := map[string]string{editor: "vim", pager: "less"}[name]

	return value, isSet
}

// elevatedWith runs, through a Runner with program that keeps keep, a
// @world update as root with env on top of the environment, and returns
// the command the next runner got.
func elevatedWith(t *testing.T, program string, keep, env []string) runner.Command {
	t.Helper()

	var fake runnertest.Fake

	elevated := elevation.Runner{Next: &fake, Program: program, Keep: keep, LookupEnv: environment}
	_, err := elevated.Run(t.Context(), runner.Command{Args: worldUpdate(), Mode: runner.Stream, Root: true, Env: env})
	require.NoError(t, err)

	commands := fake.Commands()
	require.Len(t, commands, 1)

	return commands[0]
}

func TestSudoKeepsTheVariablesToKeepAndThoseOfTheCommand(t *testing.T) {
	t.Parallel()

	got := elevatedWith(t, sudo, []string{editor, visual}, []string{hookIs, stepIs, "EDITOR=nano"})

	assert.Equal(t, append([]string{sudo, "--preserve-env=EDITOR,VISUAL,EMERGUP_HOOK,EMERGUP_STEP"}, worldUpdate()...),
		got.Args)
	assert.Equal(t, []string{hookIs, stepIs, "EDITOR=nano"}, got.Env, "sudo reads them from its own environment")
}

func TestSudoWithNothingToKeepHasNoOption(t *testing.T) {
	t.Parallel()

	assert.Equal(t, append([]string{sudo}, worldUpdate()...), elevatedWith(t, sudo, nil, nil).Args)
}

func TestRun0SetsTheVariablesToKeepThatAreSet(t *testing.T) {
	t.Parallel()

	got := elevatedWith(t, run0, []string{editor, visual, pager}, []string{hookIs, "PAGER=more", "PAGER=most"})

	assert.Equal(t, append([]string{
		run0, "--setenv=EDITOR=vim", "--setenv=PAGER=most", "--setenv=EMERGUP_HOOK=backup",
	}, worldUpdate()...), got.Args, "VISUAL is not set; the last PAGER of the command wins, as it would")
}

func TestDoasKeepsNothingItself(t *testing.T) {
	t.Parallel()

	got := elevatedWith(t, doas, []string{editor}, []string{hookIs})

	assert.Equal(t, append([]string{doas}, worldUpdate()...), got.Args)
}

func TestACommandAsTheUserKeepsItsEnvironmentAsItIs(t *testing.T) {
	t.Parallel()

	var fake runnertest.Fake

	elevated := elevation.Runner{Next: &fake, Program: sudo, Keep: []string{editor}, LookupEnv: environment}
	query := runner.Command{Args: []string{"portageq", "envvar", "ARCH"}, Mode: runner.Capture, Env: []string{hookIs}}

	_, err := elevated.Run(t.Context(), query)
	require.NoError(t, err)
	assert.Equal(t, []runner.Command{query}, fake.Commands())
}

func TestOSLooksAtTheRealSystem(t *testing.T) {
	t.Parallel()

	var system elevation.System = elevation.OS{}

	assert.Equal(t, os.Geteuid(), system.Geteuid())

	path, isSet := os.LookupEnv("PATH")
	gotPath, gotSet := system.LookupEnv("PATH")
	assert.Equal(t, isSet, gotSet)
	assert.Equal(t, path, gotPath)

	_, err := system.LookPath("emergup-test-surely-not-installed")
	require.ErrorIs(t, err, exec.ErrNotFound)
}
