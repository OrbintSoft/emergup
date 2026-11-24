// SPDX-License-Identifier: GPL-3.0-or-later

package rundir_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/emergup/internal/rundir"
)

// The variables that place the state directory.
const (
	stateHome = "XDG_STATE_HOME"
	home      = "HOME"
	userHome  = "/home/mario"
)

// environment returns a getenv that finds only vars.
func environment(vars map[string]string) func(string) string {
	return func(name string) string { return vars[name] }
}

// started is when the runs of the tests start.
func started(minute int) time.Time {
	return time.Date(2026, 11, 24, 10, minute, 5, 0, time.UTC)
}

func TestStateDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		vars map[string]string
		want string
	}{
		{"XDG_STATE_HOME", map[string]string{stateHome: "/var/state", home: userHome}, "/var/state/emergup"},
		{
			"a relative XDG_STATE_HOME",
			map[string]string{stateHome: "state", home: userHome},
			userHome + "/.local/state/emergup",
		},
		{"HOME", map[string]string{home: userHome}, userHome + "/.local/state/emergup"},
		{"root", map[string]string{home: "/root"}, "/root/.local/state/emergup"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir, err := rundir.StateDir(environment(tt.vars))
			require.NoError(t, err)
			assert.Equal(t, tt.want, dir)
		})
	}
}

func TestStateDirNeedsAnAbsolutePath(t *testing.T) {
	t.Parallel()

	for _, vars := range []map[string]string{nil, {home: "mario"}, {stateHome: "state"}} {
		_, err := rundir.StateDir(environment(vars))
		require.ErrorContains(t, err, "no state directory: neither XDG_STATE_HOME nor HOME is an absolute path")
	}
}

// newStore returns a Store whose state directory is a new temporary one,
// and the directory of its runs.
func newStore(t *testing.T) (rundir.Store, string) {
	t.Helper()

	state := t.TempDir()

	return rundir.Store{Getenv: environment(map[string]string{stateHome: state})},
		filepath.Join(state, "emergup", "runs")
}

// create makes a run directory with store and closes its log.
func create(t *testing.T, store rundir.Store, now time.Time) rundir.Run {
	t.Helper()

	run, err := store.Create(now)
	require.NoError(t, err)
	require.NotNil(t, run.Log)
	require.NoError(t, run.Log.Close())

	return run
}

func TestCreateMakesTheRunDirectoryAndItsLog(t *testing.T) {
	t.Parallel()

	store, runs := newStore(t)

	run, err := store.Create(started(4))
	require.NoError(t, err)
	require.NotNil(t, run.Log)

	dir := filepath.Join(runs, "2026-11-24T10-04-05")
	assert.Equal(t, dir, run.Dir)
	assert.Equal(t, filepath.Join(dir, "emergup.log"), run.LogPath)

	_, err = run.Log.Write([]byte("==> a line\n"))
	require.NoError(t, err)
	require.NoError(t, run.Log.Close())

	written, err := os.ReadFile(run.LogPath)
	require.NoError(t, err)
	assert.Equal(t, "==> a line\n", string(written))

	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
	assert.Equal(t, fs.FileMode(0o700), info.Mode().Perm(), "only the user can open the run directory")

	info, err = os.Stat(run.LogPath)
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o600), info.Mode().Perm(), "only the user can read the log")
}

func TestCreateNeverReusesARunDirectory(t *testing.T) {
	t.Parallel()

	store, runs := newStore(t)

	first := create(t, store, started(4))
	second := create(t, store, started(4))
	third := create(t, store, started(4))

	assert.Equal(t, filepath.Join(runs, "2026-11-24T10-04-05"), first.Dir)
	assert.Equal(t, filepath.Join(runs, "2026-11-24T10-04-05-2"), second.Dir)
	assert.Equal(t, filepath.Join(runs, "2026-11-24T10-04-05-3"), third.Dir)

	require.NoError(t, os.RemoveAll(first.Dir))

	fourth := create(t, store, started(4))
	assert.Equal(t, filepath.Join(runs, "2026-11-24T10-04-05-4"), fourth.Dir,
		"the name of a removed run is not taken again, so the latest run is the last in order")
}

func TestCreateWithoutAStateDirectory(t *testing.T) {
	t.Parallel()

	_, err := rundir.Store{Getenv: environment(nil)}.Create(started(4))
	require.ErrorContains(t, err, "no state directory")
}

// entries returns the names in dir.
func entries(t *testing.T, dir string) []string {
	t.Helper()

	found, err := os.ReadDir(dir)
	require.NoError(t, err)

	names := make([]string, 0, len(found))
	for _, entry := range found {
		names = append(names, entry.Name())
	}

	return names
}

func TestPruneKeepsTheLatestRuns(t *testing.T) {
	t.Parallel()

	store, runs := newStore(t)

	for _, minute := range []int{3, 1, 5, 2} {
		create(t, store, started(minute))
	}

	current := create(t, store, started(5))
	require.NoError(t, os.WriteFile(filepath.Join(runs, "notes.txt"), nil, 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(runs, "kept by hand"), 0o700))

	require.NoError(t, store.Prune(3, current.Dir))
	assert.Equal(t, []string{
		"2026-11-24T10-03-05", "2026-11-24T10-05-05", "2026-11-24T10-05-05-2",
		"kept by hand", "notes.txt",
	}, entries(t, runs), "the three latest runs, and what emergup did not create")

	require.NoError(t, store.Prune(0, current.Dir))
	assert.Len(t, entries(t, runs), 5, "0 keeps every run")

	require.NoError(t, store.Prune(10, current.Dir))
	assert.Len(t, entries(t, runs), 5, "fewer runs than keep")
}

func TestPruneNeverRemovesTheCurrentRun(t *testing.T) {
	t.Parallel()

	store, runs := newStore(t)

	create(t, store, started(3))
	create(t, store, started(5))

	// The clock went back: the current run is named as if it were the
	// oldest.
	current := create(t, store, started(1))

	require.NoError(t, store.Prune(2, current.Dir))
	assert.Equal(t, []string{"2026-11-24T10-01-05", "2026-11-24T10-05-05"}, entries(t, runs),
		"the current run and the latest of the others")
}

func TestPruneWithoutRuns(t *testing.T) {
	t.Parallel()

	store, _ := newStore(t)
	require.NoError(t, store.Prune(3, ""), "no run directory yet")
}
