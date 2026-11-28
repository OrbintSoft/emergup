// SPDX-License-Identifier: GPL-3.0-or-later

package report_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/emergup/internal/report"
	"github.com/OrbintSoft/emergup/internal/runner"
	"github.com/OrbintSoft/emergup/internal/runner/runnertest"
)

// errScripted is the error of a command that does not start.
var errScripted = errors.New("exec: no such file")

// update is a command of the tests that a step runs.
func update() runner.Command {
	return runner.Command{Args: []string{"emerge", "--update", "@world"}, Mode: runner.Stream, Root: true}
}

func TestRecorderRemembersTheLastCommandAndItsOutput(t *testing.T) {
	t.Parallel()

	var fake runnertest.Fake

	fake.Script(
		runnertest.Answer{Watched: "first\n"},
		runnertest.Answer{Result: runner.Result{ExitCode: 1}, Watched: failedBuild},
	)

	recorder := &report.Recorder{Next: &fake}

	_, ran := recorder.Last()
	assert.False(t, ran, "nothing ran yet")

	_, err := recorder.Run(t.Context(), runner.Command{Args: []string{"portageq", "envvar", "ARCH"}, Mode: runner.Capture})
	require.NoError(t, err)

	result, err := recorder.Run(t.Context(), update())
	require.NoError(t, err)
	assert.Equal(t, 1, result.ExitCode, "the result of the command it ran")

	last, ran := recorder.Last()
	require.True(t, ran)
	assert.Equal(t, update(), last.Command, "as the step gave it, without the recorder's watch")
	assert.Equal(t, 1, last.Result.ExitCode)
	require.NoError(t, last.Err)
	assert.Equal(t, failedCPV, last.Output.Packages()[0].CPV)
	assert.Contains(t, last.Output.Lines(), ">>> Failed to emerge app-misc/foo-1.2, Log file:\n")
	assert.NotContains(t, last.Output.Lines(), "first\n", "the output of the command before is not there")
}

func TestRecorderRemembersACommandThatDidNotStart(t *testing.T) {
	t.Parallel()

	var fake runnertest.Fake

	fake.Script(runnertest.Answer{Err: errScripted})

	recorder := &report.Recorder{Next: &fake}

	_, err := recorder.Run(t.Context(), update())
	require.ErrorIs(t, err, errScripted)

	last, ran := recorder.Last()
	require.True(t, ran)
	require.ErrorIs(t, last.Err, errScripted)
}

func TestRecorderKeepsTheWatchACommandHasAlready(t *testing.T) {
	t.Parallel()

	var fake runnertest.Fake

	fake.Script(runnertest.Answer{Watched: "a line\n"})

	var own strings.Builder

	command := update()
	command.Watch = &own

	recorder := &report.Recorder{Next: &fake}

	_, err := recorder.Run(t.Context(), command)
	require.NoError(t, err)
	assert.Equal(t, "a line\n", own.String())

	last, _ := recorder.Last()
	assert.Equal(t, []string{"a line\n"}, last.Output.Lines())
}

func TestRecorderDoesNotWatchATerminalCommand(t *testing.T) {
	t.Parallel()

	var fake runnertest.Fake

	recorder := &report.Recorder{Next: &fake}

	_, err := recorder.Run(t.Context(), runner.Command{Args: []string{"dispatch-conf"}, Mode: runner.Terminal})
	require.NoError(t, err)

	assert.Nil(t, fake.Commands()[0].Watch, "its output never reaches emergup")

	last, ran := recorder.Last()
	require.True(t, ran)
	assert.Empty(t, last.Output.Lines())
}

func TestRecorderForgetsWhatRanBefore(t *testing.T) {
	t.Parallel()

	var fake runnertest.Fake

	recorder := &report.Recorder{Next: &fake}

	_, err := recorder.Run(t.Context(), update())
	require.NoError(t, err)

	recorder.Forget()

	_, ran := recorder.Last()
	assert.False(t, ran, "a step that ran nothing since has no last command")
}
