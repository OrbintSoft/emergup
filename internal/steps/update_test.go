// SPDX-License-Identifier: GPL-3.0-or-later

package steps_test

import (
	"bytes"
	"context"
	"io"
	"os"
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

// The question of the update, as it is written, and the names the tests
// use.
const (
	updateQuestion = "==> Update these packages? [Y/s/a/?] "
	updateName     = "update"
	kernelSources  = "sys-kernel/gentoo-sources"
)

// keys answers the questions with its keys, then with the end of the
// input. When silent, a question fails the test.
type keys struct {
	t      *testing.T
	keys   []rune
	silent bool
}

func (k *keys) ReadKey(context.Context) (rune, error) {
	if k.silent {
		k.t.Error("a question was asked")
	}

	if len(k.keys) == 0 {
		return 0, io.EOF
	}

	key := k.keys[0]
	k.keys = k.keys[1:]

	return key, nil
}

// updateRun is a run of the update step.
type updateRun struct {
	cfg    config.Update
	yes    bool
	color  bool
	keys   keys
	runner runnertest.Fake
	out    bytes.Buffer
}

// newUpdateRun returns a run whose pretend prints testdata/pretend, with
// the default configuration and a keyboard that answers with keys.
func newUpdateRun(t *testing.T, pretend string, keys ...rune) *updateRun {
	t.Helper()

	output, err := os.ReadFile("testdata/" + pretend) //nolint:gosec // A fixture of the tests.
	require.NoError(t, err)

	run := &updateRun{cfg: config.Default().Steps.Update}
	run.keys.t = t
	run.keys.keys = keys
	run.runner.Script(runnertest.Answer{Result: runner.Result{ExitCode: 0, Output: output}})

	return run
}

//nolint:ireturn // The step as the pipeline sees it.
func (r *updateRun) step(t *testing.T) pipeline.Step {
	t.Helper()

	cfg := config.Default()
	cfg.Steps.Update = r.cfg

	for _, step := range steps.BuiltIn(cfg, steps.Options{}) {
		if step.Name() == updateName {
			return step
		}
	}

	t.Fatal("no update step")

	return nil
}

func (r *updateRun) env() pipeline.Env {
	printer := ui.Printer{Out: &r.out, Colors: config.Default().Colors, Color: r.color, Log: nil}

	return pipeline.Env{
		Runner:  &r.runner,
		Printer: printer,
		Menu:    ui.Menu{Printer: printer, Keyboard: &r.keys, Yes: r.yes},
		Now:     func() time.Time { return now },
		Root:    nil,
	}
}

// update returns the update command, with --pretend or not, and before
// the target the extra arguments.
func update(pretend bool, extra ...string) runner.Command {
	args := []string{emerge, "--verbose", "--update", "--deep", "--newuse", "--verbose-conflicts"}

	command := runner.Command{Args: nil, Mode: runner.Stream, Root: true}

	if pretend {
		args = append(args, "--pretend")
		command.Mode, command.Root = runner.StreamCapture, false
	}

	args = append(args, extra...)
	args = append(args, "@world")
	command.Args = args

	return command
}

func TestUpdateShowsWhatItWouldDoAsksAndUpdates(t *testing.T) {
	t.Parallel()

	run := newUpdateRun(t, "pretend_two.txt", ui.KeyEnter)

	outcome := run.step(t).Run(t.Context(), run.env())
	assert.Equal(t, pipeline.Success(), outcome)
	assert.Equal(t, []runner.Command{update(true, "--color=n"), update(false, "--color=n")}, run.runner.Commands(),
		"the pretend unprivileged, shown and read; the update elevated")
	assert.Equal(t, updateQuestion+"update them\n", run.out.String())
}

func TestUpdateConfirmsItself(t *testing.T) {
	t.Parallel()

	run := newUpdateRun(t, "pretend_two.txt")

	confirms, isSelfConfirming := run.step(t).(pipeline.SelfConfirming)
	require.True(t, isSelfConfirming)
	assert.True(t, confirms.ConfirmsItself(), "the pipeline does not ask before the pretend")
}

func TestUpdateWithYesGoesAhead(t *testing.T) {
	t.Parallel()

	run := newUpdateRun(t, "pretend_two.txt")
	run.yes = true
	run.keys.silent = true

	outcome := run.step(t).Run(t.Context(), run.env())
	assert.Equal(t, pipeline.Success(), outcome)
	assert.Len(t, run.runner.Commands(), 2)
	assert.Equal(t, updateQuestion+"update them (--yes)\n", run.out.String())
}

func TestUpdateExcludesThePackagesOfTheConfiguration(t *testing.T) {
	t.Parallel()

	run := newUpdateRun(t, "pretend_two.txt", ui.KeyEnter)
	run.cfg.Exclude = config.Entries{kernelSources, "x11-drivers/nvidia-drivers"}
	run.color = true

	run.step(t).Run(t.Context(), run.env())

	extra := []string{"--color=y", "--exclude", kernelSources, "--exclude", "x11-drivers/nvidia-drivers"}
	assert.Equal(t, []runner.Command{update(true, extra...), update(false, extra...)}, run.runner.Commands())
}

func TestUpdateSkipsWhenThereIsNothingToUpdate(t *testing.T) {
	t.Parallel()

	for _, pretend := range []string{"pretend_none.txt", "pretend_none_coloured.txt"} {
		t.Run(pretend, func(t *testing.T) {
			t.Parallel()

			run := newUpdateRun(t, pretend)
			run.keys.silent = true

			outcome := run.step(t).Run(t.Context(), run.env())
			assert.Equal(t, pipeline.Skip("nothing to update"), outcome)
			assert.Len(t, run.runner.Commands(), 1, "only the pretend")
		})
	}
}

func TestUpdateAnswers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		keys    []rune
		outcome pipeline.Outcome
	}{
		{"skip the update", []rune{'s'}, pipeline.Skip("you chose not to update")},
		{"abort the run", []rune{'a'}, pipeline.Abort("you aborted the run")},
		{"Ctrl-C", []rune{ui.KeyInterrupt}, pipeline.Abort("you aborted the run")},
		{"the end of the input", nil, pipeline.Abort("you aborted the run")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			run := newUpdateRun(t, "pretend_two.txt", tt.keys...)

			outcome := run.step(t).Run(t.Context(), run.env())
			assert.Equal(t, tt.outcome, outcome)
			assert.Len(t, run.runner.Commands(), 1, "nothing is updated")
		})
	}
}

func TestUpdateFails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		answers []runnertest.Answer
		want    string
		ran     int
	}{
		{
			"when the pretend fails, as with a conflict",
			[]runnertest.Answer{{Result: runner.Result{ExitCode: 1}}},
			"emerge --pretend exited with status 1", 1,
		},
		{
			"when emerge does not start",
			[]runnertest.Answer{{Err: errCannotStart}},
			errCannotStart.Error(), 1,
		},
		{
			"when the update fails",
			[]runnertest.Answer{
				{Result: runner.Result{ExitCode: 0, Output: []byte("Total: 1 package\n")}},
				{Result: runner.Result{ExitCode: 1}},
			},
			"emerge exited with status 1", 2,
		},
		{
			"when the update does not start",
			[]runnertest.Answer{
				{Result: runner.Result{ExitCode: 0, Output: []byte("Total: 1 package\n")}},
				{Err: errCannotStart},
			},
			errCannotStart.Error(), 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			run := newUpdateRun(t, "pretend_two.txt", ui.KeyEnter)
			run.runner = runnertest.Fake{}
			run.runner.Script(tt.answers...)

			outcome := run.step(t).Run(t.Context(), run.env())
			assert.Equal(t, pipeline.Fail(tt.want), outcome)
			assert.Len(t, run.runner.Commands(), tt.ran)
		})
	}
}

func TestUpdatePretendShowsBothCommands(t *testing.T) {
	t.Parallel()

	run := newUpdateRun(t, "pretend_two.txt")
	run.cfg.Exclude = config.Entries{kernelSources}
	run.keys.silent = true

	lines, err := run.step(t).Pretend(t.Context(), run.env())
	require.NoError(t, err)
	assert.Equal(t, []string{
		"emerge --verbose --update --deep --newuse --verbose-conflicts --pretend --color=n " +
			"--exclude sys-kernel/gentoo-sources @world",
		"then, once confirmed: emerge --verbose --update --deep --newuse --verbose-conflicts --color=n " +
			"--exclude sys-kernel/gentoo-sources @world",
	}, lines)
	assert.Empty(t, run.runner.Commands(), "emerge does not run")
}
