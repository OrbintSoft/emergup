// SPDX-License-Identifier: GPL-3.0-or-later

package steps_test

import (
	"bytes"
	"os/exec"
	"slices"
	"strconv"
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

// The live packages of the tests, and where their build time is.
const (
	example      = "app-misc/example-9999"
	other        = "dev-libs/other-9999-r1"
	exampleBuilt = "var/db/pkg/" + example + "/BUILD_TIME"
	smartLive    = "smart-live-rebuild"
	liveName     = "live-rebuild"
	emergeFailed = "emerge exited with status 1"
)

// queryVDB is the command that asks where the installed packages are.
//
//nolint:gochecknoglobals // The query several tests expect.
var queryVDB = runner.Command{Args: []string{"portageq", "vdb_path"}, Mode: runner.Capture, Root: false}

// builtAgo returns the content of a BUILD_TIME written ago before now.
func builtAgo(ago time.Duration) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte(strconv.FormatInt(now.Add(-ago).Unix(), 10) + "\n")}
}

// liveRun is a run of the live-rebuild step on a system with two live
// packages, example built 30h05m ago and other 2h ago, and two that are
// not live.
type liveRun struct {
	cfg       config.LiveRebuild
	color     bool
	installed []string
	root      fstest.MapFS
	runner    runnertest.Fake
	out       bytes.Buffer
}

func newLiveRun(t *testing.T, mode config.LiveRebuildMode) *liveRun {
	t.Helper()

	run := &liveRun{
		cfg: config.Default().Steps.LiveRebuild,
		root: fstest.MapFS{
			"var/db/pkg/" + example + "/PROPERTIES":                {Data: []byte("live\n")},
			exampleBuilt:                                           builtAgo(30*time.Hour + 5*time.Minute),
			"var/db/pkg/" + other + "/PROPERTIES":                  {Data: []byte("live test_network\n")},
			"var/db/pkg/" + other + "/BUILD_TIME":                  builtAgo(2 * time.Hour),
			"var/db/pkg/dev-libs/stable-1.0/BUILD_TIME":            builtAgo(100 * time.Hour),
			"var/db/pkg/sys-apps/props-2.0/PROPERTIES":             {Data: []byte("test_network\n")},
			"var/db/pkg/sys-apps/props-2.0/BUILD_TIME":             builtAgo(100 * time.Hour),
			"var/db/pkg/app-misc/-MERGING-example-9999/PROPERTIES": {Data: []byte("live\n")},
		},
	}
	run.cfg.Mode = mode
	run.runner.Script(runnertest.Answer{Result: runner.Result{ExitCode: 0, Output: []byte("/var/db/pkg\n")}})

	return run
}

//nolint:ireturn // The step as the pipeline sees it.
func (r *liveRun) step(t *testing.T) pipeline.Step {
	t.Helper()

	cfg := config.Default()
	cfg.Steps.LiveRebuild = r.cfg

	for _, step := range steps.BuiltIn(cfg, steps.Options{}) {
		if step.Name() == liveName {
			return step
		}
	}

	t.Fatal("no live-rebuild step")

	return nil
}

func (r *liveRun) env() pipeline.Env {
	printer := ui.Printer{Out: &r.out, Colors: config.Default().Colors, Color: r.color, Log: nil}

	return pipeline.Env{
		Runner:  &r.runner,
		Printer: printer,
		Menu:    ui.Menu{Printer: printer, Keyboard: nil, Yes: true},
		Now:     func() time.Time { return now },
		Root:    r.root,
		LookPath: func(file string) (string, error) {
			if slices.Contains(r.installed, file) {
				return "/usr/bin/" + file, nil
			}

			return "", exec.ErrNotFound
		},
	}
}

// rebuild returns the command that rebuilds packages.
func rebuild(color string, packages ...string) runner.Command {
	args := make([]string, 0, 3+len(packages))
	args = append(args, emerge, "--oneshot", "--color="+color)

	for _, atom := range packages {
		args = append(args, "="+atom)
	}

	return runner.Command{Args: args, Mode: runner.Stream, Root: true}
}

func TestLiveRebuildByInterval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mode    config.LiveRebuildMode
		prepare func(run *liveRun)
		rows    string
		rebuilt []string
	}{
		{
			"the packages built longer ago than the interval", config.LiveRebuildInterval, func(*liveRun) {},
			"    app-misc/example-9999   built 30h05m ago\n    dev-libs/other-9999-r1  built 2h00m ago, kept\n",
			[]string{example},
		},
		{
			"always", config.LiveRebuildAlways, func(*liveRun) {},
			"    app-misc/example-9999   rebuilt on every run\n    dev-libs/other-9999-r1  rebuilt on every run\n",
			[]string{example, other},
		},
		{
			"an interval of 0", config.LiveRebuildInterval, func(run *liveRun) { run.cfg.Interval = 0 },
			"    app-misc/example-9999   rebuilt on every run\n    dev-libs/other-9999-r1  rebuilt on every run\n",
			[]string{example, other},
		},
		{
			"a build time not known", config.LiveRebuildInterval, func(run *liveRun) {
				run.root[exampleBuilt] = &fstest.MapFile{Data: []byte("garbage\n")}
				delete(run.root, "var/db/pkg/"+other+"/BUILD_TIME")
			},
			"    app-misc/example-9999   build time not known\n    dev-libs/other-9999-r1  build time not known\n",
			[]string{example, other},
		},
		{
			"auto without smart-live-rebuild", config.LiveRebuildAuto, func(*liveRun) {},
			"    smart-live-rebuild is not installed: the live packages built longer ago than 24h are rebuilt\n" +
				"    app-misc/example-9999   built 30h05m ago\n    dev-libs/other-9999-r1  built 2h00m ago, kept\n",
			[]string{example},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			run := newLiveRun(t, tt.mode)
			tt.prepare(run)

			outcome := run.step(t).Run(t.Context(), run.env())
			assert.Equal(t, pipeline.Success(), outcome)
			assert.Equal(t, tt.rows, run.out.String())
			assert.Equal(t, []runner.Command{queryVDB, rebuild("n", tt.rebuilt...)}, run.runner.Commands(),
				"one command for every package due, at its exact version")
		})
	}
}

func TestLiveRebuildSkipsWhenNothingIsDue(t *testing.T) {
	t.Parallel()

	run := newLiveRun(t, config.LiveRebuildInterval)
	run.cfg.Interval = config.Duration(48 * time.Hour)

	outcome := run.step(t).Run(t.Context(), run.env())
	assert.Equal(t, pipeline.Skip("every live package was built less than 48h ago"), outcome)
	assert.Equal(t, []runner.Command{queryVDB}, run.runner.Commands())
}

func TestLiveRebuildWithoutLivePackages(t *testing.T) {
	t.Parallel()

	for _, root := range []fstest.MapFS{
		{"var/db/pkg/dev-libs/stable-1.0/BUILD_TIME": builtAgo(time.Hour)},
		{},
	} {
		run := newLiveRun(t, config.LiveRebuildAlways)
		run.root = root

		outcome := run.step(t).Run(t.Context(), run.env())
		assert.Equal(t, pipeline.Skip("no live package installed"), outcome)
		assert.Empty(t, run.out.String())
	}
}

func TestLiveRebuildGivesEmergeTheColoursOfEmergup(t *testing.T) {
	t.Parallel()

	run := newLiveRun(t, config.LiveRebuildInterval)
	run.color = true

	run.step(t).Run(t.Context(), run.env())
	assert.Equal(t, []runner.Command{queryVDB, rebuild("y", example)}, run.runner.Commands())
}

func TestLiveRebuildBySmartLiveRebuild(t *testing.T) {
	t.Parallel()

	for _, mode := range []config.LiveRebuildMode{config.LiveRebuildSmart, config.LiveRebuildAuto} {
		run := newLiveRun(t, mode)
		run.installed = []string{smartLive}
		run.runner = runnertest.Fake{}

		outcome := run.step(t).Run(t.Context(), run.env())
		assert.Equal(t, pipeline.Success(), outcome, mode)
		assert.Equal(t, []runner.Command{{Args: []string{smartLive}, Mode: runner.Stream, Root: true}},
			run.runner.Commands(), "smart-live-rebuild checks the upstreams itself, elevated")
	}
}

func TestLiveRebuildSkipsWhenSmartLiveRebuildIsNotInstalled(t *testing.T) {
	t.Parallel()

	run := newLiveRun(t, config.LiveRebuildSmart)
	run.runner = runnertest.Fake{}

	outcome := run.step(t).Run(t.Context(), run.env())
	assert.Equal(t, pipeline.Skip("smart-live-rebuild is not installed"), outcome)
	assert.Empty(t, run.runner.Commands())
}

// liveFailure is a run of the live rebuild that fails.
type liveFailure struct {
	name    string
	mode    config.LiveRebuildMode
	answers []runnertest.Answer
	root    fstest.MapFS
	want    string
}

// liveFailures are the runs of TestLiveRebuildFails.
func liveFailures() []liveFailure {
	return []liveFailure{
		{
			"when portageq fails", config.LiveRebuildInterval,
			[]runnertest.Answer{{Result: runner.Result{ExitCode: 1}}},
			nil,
			"could not find the installed packages: portageq exited with status 1",
		},
		{
			"when portageq names no database", config.LiveRebuildInterval,
			[]runnertest.Answer{{Result: runner.Result{ExitCode: 0, Output: []byte("\n")}}},
			nil,
			"could not find the installed packages: portageq named no database",
		},
		{
			"when emerge fails", config.LiveRebuildInterval,
			[]runnertest.Answer{
				{Result: runner.Result{ExitCode: 0, Output: []byte("/var/db/pkg\n")}},
				{Result: runner.Result{ExitCode: 1}},
			},
			nil,
			emergeFailed,
		},
		{
			"when smart-live-rebuild fails", config.LiveRebuildSmart,
			[]runnertest.Answer{{Result: runner.Result{ExitCode: 2}}},
			nil,
			"smart-live-rebuild exited with status 2",
		},
		{
			"when the database cannot be read", config.LiveRebuildInterval,
			[]runnertest.Answer{{Result: runner.Result{ExitCode: 0, Output: []byte("/var/db/pkg\n")}}},
			fstest.MapFS{"var/db/pkg": {Data: []byte("a file, not a directory")}},
			"could not read the installed packages",
		},
	}
}

func TestLiveRebuildFails(t *testing.T) {
	t.Parallel()

	tests := liveFailures()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			run := newLiveRun(t, tt.mode)
			run.installed = []string{smartLive}
			run.runner = runnertest.Fake{}
			run.runner.Script(tt.answers...)

			if tt.root != nil {
				run.root = tt.root
			}

			outcome := run.step(t).Run(t.Context(), run.env())
			assert.Equal(t, pipeline.Failed, outcome.Status)
			assert.Contains(t, outcome.Reason, tt.want)
		})
	}
}

func TestLiveRebuildPretend(t *testing.T) {
	t.Parallel()

	run := newLiveRun(t, config.LiveRebuildInterval)

	lines, err := run.step(t).Pretend(t.Context(), run.env())
	require.NoError(t, err)
	assert.Equal(t, []string{
		"app-misc/example-9999   built 30h05m ago",
		"dev-libs/other-9999-r1  built 2h00m ago, kept",
		"emerge --oneshot --color=n =app-misc/example-9999",
	}, lines)
	assert.Equal(t, []runner.Command{queryVDB}, run.runner.Commands(), "nothing is rebuilt")

	run = newLiveRun(t, config.LiveRebuildInterval)
	run.cfg.Interval = config.Duration(48 * time.Hour)

	lines, err = run.step(t).Pretend(t.Context(), run.env())
	require.NoError(t, err)
	assert.Equal(t, "nothing to rebuild", lines[len(lines)-1])

	run = newLiveRun(t, config.LiveRebuildSmart)
	run.installed = []string{smartLive}

	lines, err = run.step(t).Pretend(t.Context(), run.env())
	require.NoError(t, err)
	assert.Equal(t, []string{smartLive}, lines)

	run = newLiveRun(t, config.LiveRebuildSmart)

	lines, err = run.step(t).Pretend(t.Context(), run.env())
	require.NoError(t, err)
	assert.Equal(t, []string{"smart-live-rebuild is not installed, so the step would be skipped"}, lines)
}
