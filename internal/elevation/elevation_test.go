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

func TestResolve(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		method    config.ElevationMethod
		system    fakeSystem
		want      []string
		wantError string
	}{
		{"root needs nothing", config.ElevationAuto, fakeSystem{root, nil}, nil, ""},
		{"root needs nothing, even with a method set", config.ElevationDoas, fakeSystem{root, nil}, nil, ""},
		{"auto prefers sudo", config.ElevationAuto, fakeSystem{user, []string{run0, doas, sudo}}, []string{sudo}, ""},
		{"auto then doas", config.ElevationAuto, fakeSystem{user, []string{run0, doas}}, []string{doas}, ""},
		{"auto then run0", config.ElevationAuto, fakeSystem{user, []string{run0}}, []string{run0}, ""},
		{
			"auto with no tool", config.ElevationAuto,
			fakeSystem{user, nil},
			nil,
			"no elevation tool found: install sudo, doas or run0, or run emergup as root",
		},
		{"a method set", config.ElevationDoas, fakeSystem{user, []string{sudo, doas}}, []string{doas}, ""},
		{"run0 set", config.ElevationRun0, fakeSystem{user, []string{run0}}, []string{run0}, ""},
		{
			"a method set but missing", config.ElevationDoas,
			fakeSystem{user, []string{sudo}},
			nil,
			"doas is not installed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			prefix, err := elevation.Resolve(tt.method, tt.system)
			if tt.wantError != "" {
				require.ErrorContains(t, err, tt.wantError)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, prefix)
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

	elevated := elevation.Runner{Next: &fake, Prefix: []string{sudo}}

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

	direct := elevation.Runner{Next: &fake, Prefix: nil}
	update := runner.Command{Args: worldUpdate(), Mode: runner.Stream, Root: true}

	_, err := direct.Run(t.Context(), update)
	require.NoError(t, err)
	assert.Equal(t, []runner.Command{update}, fake.Commands())
}

func TestOSLooksAtTheRealSystem(t *testing.T) {
	t.Parallel()

	var system elevation.System = elevation.OS{}

	assert.Equal(t, os.Geteuid(), system.Geteuid())

	_, err := system.LookPath("emergup-test-surely-not-installed")
	require.ErrorIs(t, err, exec.ErrNotFound)
}
