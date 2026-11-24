// SPDX-License-Identifier: GPL-3.0-or-later

package steps_test

import (
	"bytes"
	"testing"
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

// What the preserved rebuild asks, writes and runs.
const (
	preservedQuestion = "==> Rebuild the packages that use them? [Y/s/a/?] "
	preservedName     = "preserved-rebuild"
	noPreserved       = "no preserved libraries"
	twoLibraries      = "    dev-libs/foo-1.0  /usr/lib64/libfoo.so.1\n" +
		"    dev-libs/bar-2.0  /usr/lib64/libbar.so.2\n" +
		"    dev-libs/bar-2.0  /usr/lib64/libbar.so.2.0.0\n"
)

// listPreserved is the query of the preserved libraries.
//
//nolint:gochecknoglobals // The query every test expects.
var listPreserved = runner.Command{
	Args: []string{portageq, "list_preserved_libs", "/"}, Mode: runner.Capture, Root: false,
}

// preservedRun is a run of the preserved rebuild where Portage preserved
// a library of foo and two of bar, written in both the forms portageq has.
type preservedRun struct {
	cfg    config.Step
	yes    bool
	color  bool
	keys   keys
	runner runnertest.Fake
	out    bytes.Buffer
}

func newPreservedRun(t *testing.T, keys ...rune) *preservedRun {
	t.Helper()

	run := &preservedRun{cfg: config.Default().Steps.PreservedRebuild}
	run.keys.t = t
	run.keys.keys = keys
	run.runner.Script(runnertest.Answer{Result: runner.Result{ExitCode: 0, Output: []byte(
		"dev-libs/foo-1.0: /usr/lib64/libfoo.so.1\n" +
			"dev-libs/bar-2.0 /usr/lib64/libbar.so.2 /usr/lib64/libbar.so.2.0.0\n",
	)}})

	return run
}

//nolint:ireturn // The step as the pipeline sees it.
func (r *preservedRun) step(t *testing.T) pipeline.Step {
	t.Helper()

	cfg := config.Default()
	cfg.Steps.PreservedRebuild = r.cfg

	for _, step := range steps.BuiltIn(cfg, steps.Options{}) {
		if step.Name() == preservedName {
			return step
		}
	}

	t.Fatal("no preserved-rebuild step")

	return nil
}

func (r *preservedRun) env() pipeline.Env {
	printer := ui.Printer{Out: &r.out, Colors: config.Default().Colors, Color: r.color, Log: nil}

	return pipeline.Env{
		Runner:   &r.runner,
		Printer:  printer,
		Menu:     ui.Menu{Printer: printer, Keyboard: &r.keys, Yes: r.yes},
		Now:      func() time.Time { return now },
		Root:     nil,
		LookPath: nil,
	}
}

// preservedRebuild returns the command that rebuilds the packages.
func preservedRebuild(color string) runner.Command {
	return runner.Command{
		Args: []string{emerge, "@preserved-rebuild", "--color=" + color}, Mode: runner.Stream, Root: true,
	}
}

func TestPreservedRebuildListsAsksAndRebuilds(t *testing.T) {
	t.Parallel()

	run := newPreservedRun(t, ui.KeyEnter)

	outcome := run.step(t).Run(t.Context(), run.env())
	assert.Equal(t, pipeline.Success(), outcome)
	assert.Equal(t, twoLibraries+preservedQuestion+"rebuild them\n", run.out.String())
	assert.Equal(t, []runner.Command{listPreserved, preservedRebuild("n")}, run.runner.Commands())
}

func TestPreservedRebuildConfirmsItself(t *testing.T) {
	t.Parallel()

	confirms, isSelfConfirming := newPreservedRun(t).step(t).(pipeline.SelfConfirming)
	require.True(t, isSelfConfirming)
	assert.True(t, confirms.ConfirmsItself(), "no question when there is nothing to rebuild")
}

func TestPreservedRebuildWithYesAndColours(t *testing.T) {
	t.Parallel()

	run := newPreservedRun(t)
	run.yes = true
	run.color = true
	run.keys.silent = true

	outcome := run.step(t).Run(t.Context(), run.env())
	assert.Equal(t, pipeline.Success(), outcome)
	assert.Equal(t, []runner.Command{listPreserved, preservedRebuild("y")}, run.runner.Commands())
}

func TestPreservedRebuildSkipsWithoutPreservedLibraries(t *testing.T) {
	t.Parallel()

	for _, answer := range []runnertest.Answer{
		{Result: runner.Result{ExitCode: 1}},
		{Result: runner.Result{ExitCode: 0, Output: []byte("\n")}},
	} {
		run := newPreservedRun(t)
		run.keys.silent = true
		run.runner = runnertest.Fake{}
		run.runner.Script(answer)

		outcome := run.step(t).Run(t.Context(), run.env())
		assert.Equal(t, pipeline.Skip(noPreserved), outcome, "without a question")
		assert.Empty(t, run.out.String())
		assert.Equal(t, []runner.Command{listPreserved}, run.runner.Commands())
	}
}

func TestPreservedRebuildAnswers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		keys    []rune
		outcome pipeline.Outcome
	}{
		{"skip the rebuild", []rune{'s'}, pipeline.Skip("you chose not to rebuild them")},
		{"abort the run", []rune{'a'}, pipeline.Abort("you aborted the run")},
		{"the end of the input", nil, pipeline.Abort("you aborted the run")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			run := newPreservedRun(t, tt.keys...)

			outcome := run.step(t).Run(t.Context(), run.env())
			assert.Equal(t, tt.outcome, outcome)
			assert.Equal(t, []runner.Command{listPreserved}, run.runner.Commands(), "nothing is rebuilt")
		})
	}
}

func TestPreservedRebuildFails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		answers []runnertest.Answer
		want    string
	}{
		{
			"when portageq fails",
			[]runnertest.Answer{{Result: runner.Result{ExitCode: 2}}},
			"could not list the preserved libraries: portageq exited with status 2",
		},
		{
			"when portageq does not start",
			[]runnertest.Answer{{Err: errCannotStart}},
			"could not list the preserved libraries: " + errCannotStart.Error(),
		},
		{
			whenEmergeFails,
			[]runnertest.Answer{
				{Result: runner.Result{ExitCode: 0, Output: []byte("dev-libs/foo-1.0: /usr/lib64/libfoo.so.1\n")}},
				{Result: runner.Result{ExitCode: 1}},
			},
			emergeFailed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			run := newPreservedRun(t, ui.KeyEnter)
			run.runner = runnertest.Fake{}
			run.runner.Script(tt.answers...)

			outcome := run.step(t).Run(t.Context(), run.env())
			assert.Equal(t, pipeline.Fail(tt.want), outcome)
		})
	}
}

func TestPreservedRebuildPretend(t *testing.T) {
	t.Parallel()

	run := newPreservedRun(t)
	run.keys.silent = true

	lines, err := run.step(t).Pretend(t.Context(), run.env())
	require.NoError(t, err)
	assert.Equal(t, []string{
		"dev-libs/foo-1.0  /usr/lib64/libfoo.so.1",
		"dev-libs/bar-2.0  /usr/lib64/libbar.so.2",
		"dev-libs/bar-2.0  /usr/lib64/libbar.so.2.0.0",
		"emerge @preserved-rebuild --color=n",
	}, lines)
	assert.Equal(t, []runner.Command{listPreserved}, run.runner.Commands(), "nothing is rebuilt")

	run = newPreservedRun(t)
	run.runner = runnertest.Fake{}
	run.runner.Script(runnertest.Answer{Result: runner.Result{ExitCode: 1}})

	lines, err = run.step(t).Pretend(t.Context(), run.env())
	require.NoError(t, err)
	assert.Equal(t, []string{noPreserved}, lines)
}
