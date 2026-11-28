// SPDX-License-Identifier: GPL-3.0-or-later

package runnertest_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/emergup/internal/runner"
	"github.com/OrbintSoft/emergup/internal/runner/runnertest"
)

var errScripted = errors.New("scripted failure")

// emerge is the program of most commands of the tests.
const emerge = "emerge"

func TestFakeRecordsTheCommandsAndAnswersInOrder(t *testing.T) {
	t.Parallel()

	var fake runnertest.Fake

	fake.Script(
		runnertest.Answer{Result: runner.Result{ExitCode: 1}},
		runnertest.Answer{Result: runner.Result{Output: []byte("data")}, Err: errScripted},
	)

	sync := runner.Command{Args: []string{emerge, "--sync"}, Mode: runner.Stream}
	query := runner.Command{Args: []string{"portageq", "envvar", "ARCH"}, Mode: runner.Capture}
	depclean := runner.Command{Args: []string{emerge, "--depclean"}, Mode: runner.Stream}

	first, err := fake.Run(t.Context(), sync)
	require.NoError(t, err)
	assert.Equal(t, 1, first.ExitCode)

	second, err := fake.Run(t.Context(), query)
	require.ErrorIs(t, err, errScripted)
	assert.Equal(t, []byte("data"), second.Output)

	third, err := fake.Run(t.Context(), depclean)
	require.NoError(t, err)
	assert.Equal(t, runner.Result{}, third, "a successful result once the answers are used up")

	assert.Equal(t, []runner.Command{sync, query, depclean}, fake.Commands())
}

func TestFakeStopsWhenTheContextIsDone(t *testing.T) {
	t.Parallel()

	var fake runnertest.Fake

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := fake.Run(ctx, runner.Command{Args: []string{emerge}, Mode: runner.Stream})
	require.ErrorIs(t, err, context.Canceled)
	assert.Len(t, fake.Commands(), 1, "the command is recorded even so")
}

func TestFakeIsARunner(t *testing.T) {
	t.Parallel()

	var _ runner.Runner = &runnertest.Fake{}
}

func TestFakeGivesTheWatchWhatTheAnswerSays(t *testing.T) {
	t.Parallel()

	var fake runnertest.Fake

	fake.Script(runnertest.Answer{Watched: "a line\nanother\n"}, runnertest.Answer{})

	var watch strings.Builder

	_, err := fake.Run(t.Context(), runner.Command{Args: []string{emerge}, Mode: runner.Stream, Watch: &watch})
	require.NoError(t, err)
	assert.Equal(t, "a line\nanother\n", watch.String())

	_, err = fake.Run(t.Context(), runner.Command{Args: []string{emerge}, Mode: runner.Stream})
	require.NoError(t, err, "a command without a watch")
}

func TestFakeSavesTheOutputOfASaveCommand(t *testing.T) {
	t.Parallel()

	var fake runnertest.Fake

	fake.Script(runnertest.Answer{Result: runner.Result{ExitCode: 0, Output: []byte("the content\n")}})

	var into strings.Builder

	result, err := fake.Run(t.Context(), runner.Command{Args: []string{"cat"}, Mode: runner.Save, Into: &into})
	require.NoError(t, err)
	assert.Equal(t, "the content\n", into.String())
	assert.Empty(t, result.Output, "a Save command collects nothing")
}
