// SPDX-License-Identifier: GPL-3.0-or-later

package steps_test

import (
	"bytes"
	"os"
	"strings"
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

// What the depclean asks.
const (
	depcleanName     = "depclean"
	depcleanQuestion = "==> Remove these packages? [Y/s/a/?] "
)

// depcleanRun is a run of the depclean step.
type depcleanRun struct {
	yes    bool
	color  bool
	keys   keys
	runner runnertest.Fake
	out    bytes.Buffer
}

// newDepcleanRun returns a run whose pretend prints testdata/pretend, and
// a keyboard that answers with keys.
func newDepcleanRun(t *testing.T, pretend string, keys ...rune) *depcleanRun {
	t.Helper()

	output, err := os.ReadFile("testdata/" + pretend) //nolint:gosec // A fixture of the tests.
	require.NoError(t, err)

	run := &depcleanRun{}
	run.keys.t = t
	run.keys.keys = keys
	run.runner.Script(runnertest.Answer{Result: runner.Result{ExitCode: 0, Output: output}})

	return run
}

//nolint:ireturn // The step as the pipeline sees it.
func (r *depcleanRun) step(t *testing.T) pipeline.Step {
	t.Helper()

	for _, step := range steps.BuiltIn(config.Default(), steps.Options{}) {
		if step.Name() == depcleanName {
			return step
		}
	}

	t.Fatal("no depclean step")

	return nil
}

func (r *depcleanRun) env() pipeline.Env {
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

// depclean returns the depclean command, with --pretend or not.
func depclean(pretend bool, color string) runner.Command {
	if pretend {
		return runner.Command{
			Args: []string{emerge, "--depclean", "--pretend", "--color=" + color}, Mode: runner.StreamCapture, Root: false,
		}
	}

	return runner.Command{Args: []string{emerge, "--depclean", "--color=" + color}, Mode: runner.Stream, Root: true}
}

func TestDepcleanShowsWhatItWouldRemoveAsksAndRemoves(t *testing.T) {
	t.Parallel()

	run := newDepcleanRun(t, "depclean_one.txt", ui.KeyEnter)

	outcome := run.step(t).Run(t.Context(), run.env())
	assert.Equal(t, pipeline.Success(), outcome)
	assert.Equal(t, []runner.Command{depclean(true, "n"), depclean(false, "n")}, run.runner.Commands(),
		"the pretend unprivileged, shown and read; the depclean elevated")
	assert.Equal(t, depcleanQuestion+"remove them\n", run.out.String())
}

func TestDepcleanConfirmsItself(t *testing.T) {
	t.Parallel()

	confirms, isSelfConfirming := newDepcleanRun(t, "depclean_one.txt").step(t).(pipeline.SelfConfirming)
	require.True(t, isSelfConfirming)
	assert.True(t, confirms.ConfirmsItself())
}

func TestDepcleanWithYesGoesAhead(t *testing.T) {
	t.Parallel()

	run := newDepcleanRun(t, "depclean_one.txt")
	run.yes = true
	run.color = true
	run.keys.silent = true

	outcome := run.step(t).Run(t.Context(), run.env())
	assert.Equal(t, pipeline.Success(), outcome)
	assert.Equal(t, []runner.Command{depclean(true, "y"), depclean(false, "y")}, run.runner.Commands())
	assert.Equal(t, "\x1b[1;36m"+strings.TrimSuffix(depcleanQuestion, " ")+"\x1b[0m remove them (--yes)\n",
		run.out.String())
}

func TestDepcleanSkipsWhenThereIsNothingToRemove(t *testing.T) {
	t.Parallel()

	run := newDepcleanRun(t, "depclean_none.txt")
	run.keys.silent = true

	outcome := run.step(t).Run(t.Context(), run.env())
	assert.Equal(t, pipeline.Skip("nothing to remove"), outcome, "without a question")
	assert.Len(t, run.runner.Commands(), 1, "only the pretend")
}

func TestDepcleanAnswers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		keys    []rune
		outcome pipeline.Outcome
	}{
		{"skip the depclean", []rune{'s'}, pipeline.Skip("you chose not to remove them")},
		{caseAbort, []rune{'a'}, pipeline.Abort("you aborted the run")},
		{caseEndOfInput, nil, pipeline.Abort("you aborted the run")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			run := newDepcleanRun(t, "depclean_one.txt", tt.keys...)

			outcome := run.step(t).Run(t.Context(), run.env())
			assert.Equal(t, tt.outcome, outcome)
			assert.Len(t, run.runner.Commands(), 1, "nothing is removed")
		})
	}
}

func TestDepcleanFails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		answers []runnertest.Answer
		want    string
	}{
		{
			"when the pretend fails, as with unresolved dependencies",
			[]runnertest.Answer{{Result: runner.Result{ExitCode: 1}}},
			"emerge --pretend exited with status 1",
		},
		{
			"when the depclean fails",
			[]runnertest.Answer{
				{Result: runner.Result{ExitCode: 0, Output: []byte("Number to remove:     1\n")}},
				{Result: runner.Result{ExitCode: 1}},
			},
			emergeFailed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			run := newDepcleanRun(t, "depclean_one.txt", ui.KeyEnter)
			run.runner = runnertest.Fake{}
			run.runner.Script(tt.answers...)

			outcome := run.step(t).Run(t.Context(), run.env())
			assert.Equal(t, pipeline.Fail(tt.want), outcome)
		})
	}
}

func TestDepcleanPretendShowsBothCommands(t *testing.T) {
	t.Parallel()

	run := newDepcleanRun(t, "depclean_one.txt")
	run.keys.silent = true

	lines, err := run.step(t).Pretend(t.Context(), run.env())
	require.NoError(t, err)
	assert.Equal(t, []string{
		"emerge --depclean --pretend --color=n",
		"then, once confirmed: emerge --depclean --color=n",
	}, lines)
	assert.Empty(t, run.runner.Commands(), "emerge does not run")
}
