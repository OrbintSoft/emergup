// SPDX-License-Identifier: GPL-3.0-or-later

package runner_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/emergup/internal/runner"
)

// started is the time the tests write in the log.
var started = time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC) //nolint:gochecknoglobals // a fixed clock for the tests

// streams collects what an Exec writes.
type streams struct {
	stdout bytes.Buffer
	stderr bytes.Buffer
	log    bytes.Buffer
}

// newExec returns an Exec that reads stdin and writes into the returned
// streams, with a fixed clock.
func newExec(stdin string) (*runner.Exec, *streams) {
	out := &streams{}

	return &runner.Exec{
		Stdin:  strings.NewReader(stdin),
		Stdout: &out.stdout,
		Stderr: &out.stderr,
		Log:    &out.log,
		Now:    func() time.Time { return started },
	}, out
}

// childCommand runs the test binary as a child performing ops.
func childCommand(mode runner.Mode, ops ...string) runner.Command {
	return runner.Command{Args: append([]string{os.Args[0], childMarker}, ops...), Mode: mode}
}

func TestStreamShowsAndLogsTheOutput(t *testing.T) {
	t.Parallel()

	exec, out := newExec("")

	result, err := exec.Run(t.Context(), childCommand(runner.Stream, "out:hello", "err:oops", "exit:3"))
	require.NoError(t, err)
	assert.Equal(t, 3, result.ExitCode)
	assert.Positive(t, result.Duration)
	assert.Empty(t, result.Output)

	assert.Equal(t, "hello\n", out.stdout.String())
	assert.Equal(t, "oops\n", out.stderr.String())

	log := out.log.String()
	header := "==> 2026-11-23T10:00:00Z " + os.Args[0] + " " + childMarker + " out:hello err:oops exit:3\n"
	assert.True(t, strings.HasPrefix(log, header), "log starts with the time and the command line:\n%s", log)
	assert.Contains(t, log, "hello\n")
	assert.Contains(t, log, "oops\n")
	assert.Contains(t, log, "==> exit status 3 after ")
}

func TestTheLogQuotesArgumentsTheShellWouldSplit(t *testing.T) {
	t.Parallel()

	exec, out := newExec("")

	_, err := exec.Run(t.Context(), childCommand(runner.Stream, "out:two words", "out:it's"))
	require.NoError(t, err)
	assert.Contains(t, out.log.String(), ` 'out:two words' 'out:it'\''s'`+"\n")
}

func TestTheLogHasNoEscapeSequences(t *testing.T) {
	t.Parallel()

	for _, mode := range []runner.Mode{runner.Stream, runner.Capture} {
		exec, out := newExec("")

		result, err := exec.Run(t.Context(), childCommand(mode, "green:done", "bold:warning"))
		require.NoError(t, err)

		shown := out.stdout.String() + string(result.Output)
		assert.Equal(t, greenEscape+"done"+resetEscape+"\n", shown, "the colours reach their destination")
		assert.Equal(t, boldEscape+"warning"+resetEscape+"\n", out.stderr.String())

		log := out.log.String()
		assert.Contains(t, log, "\ndone\n")
		assert.Contains(t, log, "\nwarning\n")
		assert.NotContains(t, log, "\x1b")
	}
}

func TestCaptureCollectsStandardOutput(t *testing.T) {
	t.Parallel()

	exec, out := newExec("")

	result, err := exec.Run(t.Context(), childCommand(runner.Capture, "out:data", "err:warning"))
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "data\n", string(result.Output))

	assert.Empty(t, out.stdout.String())
	assert.Equal(t, "warning\n", out.stderr.String())
	assert.Contains(t, out.log.String(), "data\n")
	assert.Contains(t, out.log.String(), "warning\n")
}

func TestTerminalConnectsTheStandardStreams(t *testing.T) {
	t.Parallel()

	exec, out := newExec("typed\n")

	result, err := exec.Run(t.Context(), childCommand(runner.Terminal, "echo-stdin"))
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "typed\n", out.stdout.String())

	log := out.log.String()
	assert.Contains(t, log, " echo-stdin\n")
	assert.Contains(t, log, "==> exit status 0 after ")
	assert.NotContains(t, log, "typed", "an interactive command's output stays out of the log")
}

func TestStreamAndCaptureGiveNoStandardInput(t *testing.T) {
	t.Parallel()

	for _, mode := range []runner.Mode{runner.Stream, runner.Capture} {
		exec, out := newExec("typed\n")

		result, err := exec.Run(t.Context(), childCommand(mode, "echo-stdin"))
		require.NoError(t, err)
		assert.Empty(t, out.stdout.String())
		assert.Empty(t, result.Output)
	}
}

func TestACommandThatCannotStart(t *testing.T) {
	t.Parallel()

	exec, out := newExec("")

	missing := runner.Command{Args: []string{"/nonexistent/emergup-test-program"}, Mode: runner.Stream}

	_, err := exec.Run(t.Context(), missing)
	require.Error(t, err)
	assert.Contains(t, out.log.String(), "==> could not start: ")
}

func TestAnEmptyCommand(t *testing.T) {
	t.Parallel()

	exec, _ := newExec("")

	_, err := exec.Run(t.Context(), runner.Command{Args: nil, Mode: runner.Stream})
	require.Error(t, err)
}

func TestWithoutALog(t *testing.T) {
	t.Parallel()

	exec, out := newExec("")
	exec.Log = nil

	result, err := exec.Run(t.Context(), childCommand(runner.Stream, "out:hello"))
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "hello\n", out.stdout.String())
}

// readyWriter keeps what is written to it and closes ready once it holds
// "ready", the word the child writes when it waits for a signal.
type readyWriter struct {
	mu    sync.Mutex
	text  bytes.Buffer
	ready chan struct{}
	once  sync.Once
}

func newReadyWriter() *readyWriter {
	return &readyWriter{ready: make(chan struct{})}
}

func (w *readyWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.text.Write(data)

	if strings.Contains(w.text.String(), "ready") {
		w.once.Do(func() { close(w.ready) })
	}

	return len(data), nil
}

func (w *readyWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.text.String()
}

// runUntilReady runs cmd on exec, cancels the context once the child says it
// is ready, and returns how Run ended.
func runUntilReady(t *testing.T, exec *runner.Exec, stdout *readyWriter, cmd runner.Command) (runner.Result, error) {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	go func() {
		select {
		case <-stdout.ready:
			cancel()
		case <-ctx.Done():
		}
	}()

	return exec.Run(ctx, cmd) //nolint:wrapcheck // the tests look at the error as Run returns it
}

func TestCancellingAsksTheCommandToStop(t *testing.T) {
	t.Parallel()

	exec, _ := newExec("")
	stdout := newReadyWriter()
	exec.Stdout = stdout

	result, err := runUntilReady(t, exec, stdout, childCommand(runner.Stream, "wait-term"))
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 3, result.ExitCode, "the command stopped by itself")
	assert.Contains(t, stdout.String(), "terminated")
}

func TestACommandThatDoesNotStopIsKilled(t *testing.T) {
	t.Parallel()

	exec, _ := newExec("")
	stdout := newReadyWriter()
	exec.Stdout = stdout
	exec.GracePeriod = 200 * time.Millisecond

	result, err := runUntilReady(t, exec, stdout, childCommand(runner.Stream, "ignore-term"))
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, -1, result.ExitCode, "a signal ended the command")
	assert.Less(t, result.Duration, 10*time.Second)
}
