// SPDX-License-Identifier: GPL-3.0-or-later

package steps_test

import (
	"bytes"
	"errors"
	"os"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/emergup/internal/config"
	"github.com/OrbintSoft/emergup/internal/pipeline"
	"github.com/OrbintSoft/emergup/internal/runner"
	"github.com/OrbintSoft/emergup/internal/runner/runnertest"
	"github.com/OrbintSoft/emergup/internal/steps"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// The files the sync reads the last sync from.
const (
	gentooTimestamp = "var/db/repos/gentoo/metadata/timestamp.chk"
	guruFetchHead   = "var/db/repos/guru/.git/FETCH_HEAD"
	syncName        = "sync"
	gentoo          = "gentoo"
	guru            = "guru"
	emergeSync      = "--sync"
	emerge          = "emerge"
)

// errCannotStart is the error of a command that does not start.
var errCannotStart = errors.New("exec: no such file")

// now is when the runs of the tests take place.
var now = time.Date(2026, 11, 24, 10, 12, 0, 0, time.UTC) //nolint:gochecknoglobals // A fixed clock.

// The commands the sync runs.
var (
	//nolint:gochecknoglobals // The query every test expects.
	listRepositories = runner.Command{Args: []string{"portageq", "repos_config", "/"}, Mode: runner.Capture, Root: false}
)

// syncRun is a run of the sync step on a system whose repositories are
// those of testdata/repos_config.txt.
type syncRun struct {
	cfg    config.Sync
	force  bool
	color  bool
	root   fstest.MapFS
	runner runnertest.Fake
	out    bytes.Buffer
}

// newSyncRun returns a run where gentoo was synced 3h12m ago and guru
// 26h05m ago, with the default configuration.
func newSyncRun(t *testing.T) *syncRun {
	t.Helper()

	reposConfig, err := os.ReadFile("testdata/repos_config.txt")
	require.NoError(t, err)

	run := &syncRun{
		cfg: config.Default().Steps.Sync,
		root: fstest.MapFS{
			gentooTimestamp: {Data: []byte("Tue, 24 Nov 2026 07:00:00 +0000\n")},
			guruFetchHead:   {ModTime: now.Add(-26*time.Hour - 5*time.Minute)},
		},
	}
	run.runner.Script(runnertest.Answer{Result: runner.Result{ExitCode: 0, Output: reposConfig}})

	return run
}

//nolint:ireturn // The step as the pipeline sees it.
func (r *syncRun) step(t *testing.T) pipeline.Step {
	t.Helper()

	cfg := config.Default()
	cfg.Steps.Sync = r.cfg

	for _, step := range steps.BuiltIn(cfg, steps.Options{ForceSync: r.force}) {
		if step.Name() == syncName {
			return step
		}
	}

	t.Fatal("no sync step")

	return nil
}

func (r *syncRun) env() pipeline.Env {
	printer := ui.Printer{Out: &r.out, Colors: config.Default().Colors, Color: r.color, Log: nil}

	return pipeline.Env{
		Runner:  &r.runner,
		Printer: printer,
		Menu:    ui.Menu{Printer: printer, Keyboard: nil, Yes: true},
		Now:     func() time.Time { return now },
		Root:    r.root,
	}
}

// sync returns the command that syncs repositories.
func sync(color string, repositories ...string) runner.Command {
	return runner.Command{
		Args: append([]string{emerge, emergeSync, "--color=" + color}, repositories...),
		Mode: runner.Stream,
		Root: true,
	}
}

// decisionCase is a run of the sync that decides what to do with each
// repository.
type decisionCase struct {
	name    string
	prepare func(run *syncRun)
	rows    string
	synced  []string
}

// decisionCases are the runs of TestSyncDecidesForEachRepository.
func decisionCases() []decisionCase {
	return []decisionCase{
		{
			"the repositories due", func(*syncRun) {},
			"    gentoo  synced 3h12m ago, kept\n    guru    synced 26h05m ago\n",
			[]string{guru},
		},
		{
			"always synced", func(run *syncRun) { run.cfg.AlwaysSync = config.Entries{gentoo} },
			"    gentoo  always synced\n    guru    synced 26h05m ago\n",
			[]string{gentoo, guru},
		},
		{
			"always synced, which Portage does not sync by itself",
			func(run *syncRun) { run.cfg.AlwaysSync = config.Entries{"frozen"} },
			"    gentoo  synced 3h12m ago, kept\n    guru    synced 26h05m ago\n    frozen  always synced\n",
			[]string{guru, "frozen"},
		},
		{
			"--force-sync", func(run *syncRun) { run.force = true },
			"    gentoo  --force-sync\n    guru    --force-sync\n",
			[]string{gentoo, guru},
		},
		{
			"an interval of 0", func(run *syncRun) { run.cfg.MinInterval = 0 },
			"    gentoo  synced on every run\n    guru    synced on every run\n",
			[]string{gentoo, guru},
		},
		{
			"a last sync not known", func(run *syncRun) { delete(run.root, guruFetchHead) },
			"    gentoo  synced 3h12m ago, kept\n    guru    last sync not known\n",
			[]string{guru},
		},
		{
			"a timestamp without a date", func(run *syncRun) {
				run.root[gentooTimestamp] = &fstest.MapFile{Data: []byte("garbage\n"), ModTime: now.Add(-30 * time.Hour)}
			},
			"    gentoo  synced 30h00m ago\n    guru    synced 26h05m ago\n",
			[]string{gentoo, guru},
		},
		{
			"a longer interval", func(run *syncRun) { run.cfg.MinInterval = config.Duration(48 * time.Hour) },
			"    gentoo  synced 3h12m ago, kept\n    guru    synced 26h05m ago, kept\n",
			nil,
		},
	}
}

func TestSyncDecidesForEachRepository(t *testing.T) {
	t.Parallel()

	tests := decisionCases()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			run := newSyncRun(t)
			tt.prepare(run)

			outcome := run.step(t).Run(t.Context(), run.env())
			assert.Equal(t, tt.rows, run.out.String())

			want := []runner.Command{listRepositories}
			if tt.synced != nil {
				want = append(want, sync("n", tt.synced...))
			}

			assert.Equal(t, want, run.runner.Commands(), "one command for every repository due")

			if tt.synced == nil {
				assert.Equal(t, pipeline.Skip("every repository was synced less than 48h ago"), outcome)
			} else {
				assert.Equal(t, pipeline.Success(), outcome)
			}
		})
	}
}

func TestSyncSkipsWhenNoRepositoryIsDue(t *testing.T) {
	t.Parallel()

	run := newSyncRun(t)
	run.root[guruFetchHead] = &fstest.MapFile{ModTime: now.Add(-time.Hour)}

	outcome := run.step(t).Run(t.Context(), run.env())
	assert.Equal(t, pipeline.Skip("every repository was synced less than 24h ago"), outcome)
	assert.Equal(t, "    gentoo  synced 3h12m ago, kept\n    guru    synced 1h00m ago, kept\n", run.out.String())
	assert.Equal(t, []runner.Command{listRepositories}, run.runner.Commands())
}

func TestSyncSaysWhenAnAlwaysSyncedRepositoryDoesNotExist(t *testing.T) {
	t.Parallel()

	run := newSyncRun(t)
	run.cfg.AlwaysSync = config.Entries{"nope", "local"}

	outcome := run.step(t).Run(t.Context(), run.env())
	assert.Equal(t, pipeline.Success(), outcome)
	assert.Equal(t, "==> always-sync names nope, which is not a repository\n"+
		"==> always-sync names local, which Portage cannot sync: it has no sync-type\n"+
		"    gentoo  synced 3h12m ago, kept\n    guru    synced 26h05m ago\n", run.out.String())
}

func TestSyncWithoutRepositoriesToSync(t *testing.T) {
	t.Parallel()

	run := newSyncRun(t)
	run.runner = runnertest.Fake{}
	run.runner.Script(runnertest.Answer{Result: runner.Result{
		ExitCode: 0, Output: []byte("[DEFAULT]\nauto-sync = yes\n\n[local]\nlocation = /var/db/repos/local\n"),
	}})

	outcome := run.step(t).Run(t.Context(), run.env())
	assert.Equal(t, pipeline.Skip("no repository to sync"), outcome)
	assert.Empty(t, run.out.String())
}

func TestSyncGivesEmergeTheColoursOfEmergup(t *testing.T) {
	t.Parallel()

	run := newSyncRun(t)
	run.color = true

	run.step(t).Run(t.Context(), run.env())
	assert.Equal(t, []runner.Command{listRepositories, sync("y", guru)}, run.runner.Commands())
}

func TestSyncRunsTheConfiguredCommand(t *testing.T) {
	t.Parallel()

	run := newSyncRun(t)
	run.cfg.Command = []string{emerge, emergeSync, "--quiet"}

	run.step(t).Run(t.Context(), run.env())
	assert.Equal(t, []runner.Command{listRepositories, {
		Args: []string{emerge, emergeSync, "--quiet", "--color=n", guru}, Mode: runner.Stream, Root: true,
	}}, run.runner.Commands())
}

func TestSyncFails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		answers []runnertest.Answer
		want    string
		ran     int
	}{
		{
			"when the repositories cannot be listed",
			[]runnertest.Answer{{Result: runner.Result{ExitCode: 1}}},
			"could not list the repositories: portageq exited with status 1", 1,
		},
		{
			"when portageq does not start",
			[]runnertest.Answer{{Err: errCannotStart}},
			"could not list the repositories: exec: no such file", 1,
		},
		{
			"when emerge fails",
			[]runnertest.Answer{{}, {Result: runner.Result{ExitCode: 1}}},
			"emerge exited with status 1", 2,
		},
		{
			"when emerge does not start",
			[]runnertest.Answer{{}, {Err: errCannotStart}},
			"exec: no such file", 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			run := newSyncRun(t)

			if len(tt.answers) > 1 {
				// The first answer is the listing of newSyncRun.
				run.runner.Script(tt.answers[1:]...)
			} else {
				run.runner = runnertest.Fake{}
				run.runner.Script(tt.answers...)
			}

			outcome := run.step(t).Run(t.Context(), run.env())
			assert.Equal(t, pipeline.Fail(tt.want), outcome)
			assert.Len(t, run.runner.Commands(), tt.ran)
		})
	}
}

func TestSyncPretendSaysWhatItWouldDo(t *testing.T) {
	t.Parallel()

	run := newSyncRun(t)

	lines, err := run.step(t).Pretend(t.Context(), run.env())
	require.NoError(t, err)
	assert.Equal(t, []string{
		"gentoo  synced 3h12m ago, kept",
		"guru    synced 26h05m ago",
		"emerge --sync --color=n guru",
	}, lines)
	assert.Equal(t, []runner.Command{listRepositories}, run.runner.Commands(), "it syncs nothing")
	assert.Empty(t, run.out.String())

	run = newSyncRun(t)
	run.root[guruFetchHead] = &fstest.MapFile{ModTime: now.Add(-time.Hour)}

	lines, err = run.step(t).Pretend(t.Context(), run.env())
	require.NoError(t, err)
	assert.Equal(t, []string{
		"gentoo  synced 3h12m ago, kept",
		"guru    synced 1h00m ago, kept",
		"nothing to sync",
	}, lines)
}

func TestSyncPretendFailsWhenTheRepositoriesCannotBeListed(t *testing.T) {
	t.Parallel()

	run := newSyncRun(t)
	run.runner = runnertest.Fake{}
	run.runner.Script(runnertest.Answer{Result: runner.Result{ExitCode: 1}})

	_, err := run.step(t).Pretend(t.Context(), run.env())
	require.EqualError(t, err, "could not list the repositories: portageq exited with status 1")
}
