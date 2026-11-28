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

func TestStreamCaptureShowsAndCollectsTheOutput(t *testing.T) {
	t.Parallel()

	exec, out := newExec("")

	result, err := exec.Run(t.Context(), childCommand(runner.StreamCapture, "green:data", "err:warning"))
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)

	assert.Equal(t, greenEscape+"data"+resetEscape+"\n", out.stdout.String(), "shown as it comes")
	assert.Equal(t, greenEscape+"data"+resetEscape+"\n", string(result.Output), "and collected as it is")
	assert.Equal(t, "warning\n", out.stderr.String())

	log := out.log.String()
	assert.Contains(t, log, "\ndata\n")
	assert.Contains(t, log, "\nwarning\n")
	assert.NotContains(t, log, "\x1b")
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

	for _, mode := range []runner.Mode{runner.Stream, runner.Capture, runner.StreamCapture} {
		exec, out := newExec("typed\n")

		result, err := exec.Run(t.Context(), childCommand(mode, "echo-stdin"))
		require.NoError(t, err)
		assert.Empty(t, out.stdout.String())
		assert.Empty(t, result.Output)
	}
}

func TestACommandReadsItsStandardInput(t *testing.T) {
	t.Parallel()

	for _, mode := range []runner.Mode{runner.Stream, runner.Capture, runner.StreamCapture} {
		exec, out := newExec("typed\n")

		cmd := childCommand(mode, "echo-stdin")
		cmd.Stdin = strings.NewReader("the request\nin two lines\n")

		result, err := exec.Run(t.Context(), cmd)
		require.NoError(t, err)
		assert.Equal(t, 0, result.ExitCode)

		if mode == runner.Capture {
			assert.Equal(t, "the request\nin two lines\n", string(result.Output), "the mode %d", mode)
		} else {
			assert.Equal(t, "the request\nin two lines\n", out.stdout.String(), "the mode %d", mode)
		}

		assert.NotContains(t, out.stdout.String(), "typed", "not emergup's own standard input")
	}
}

func TestATerminalCommandReadsNoOtherStandardInput(t *testing.T) {
	t.Parallel()

	exec, out := newExec("typed\n")

	cmd := childCommand(runner.Terminal, "echo-stdin")
	cmd.Stdin = strings.NewReader("the request\n")

	_, err := exec.Run(t.Context(), cmd)
	require.ErrorContains(t, err, "a Terminal command reads emergup's own standard input")
	assert.Empty(t, out.stdout.String(), "it does not run")
}

// The test sets emergup's own environment, so it does not run in parallel.
func TestTheCommandGetsItsVariablesOnTopOfTheEnvironment(t *testing.T) {
	t.Setenv("EMERGUP_TEST_KEPT", "kept")
	t.Setenv("EMERGUP_TEST_REPLACED", "old")

	exec, out := newExec("")

	cmd := childCommand(runner.Stream,
		"env:EMERGUP_TEST_KEPT", "env:EMERGUP_TEST_REPLACED", "env:EMERGUP_TEST_ADDED", "env:EMERGUP_TEST_UNSET")
	cmd.Env = []string{"EMERGUP_TEST_REPLACED=new", "EMERGUP_TEST_ADDED=added"}

	result, err := exec.Run(t.Context(), cmd)
	require.NoError(t, err)
	assert.Zero(t, result.ExitCode)
	assert.Equal(t, "kept\nnew\nadded\nunset\n", out.stdout.String())
}

func TestWithoutVariablesTheCommandGetsTheEnvironment(t *testing.T) {
	t.Setenv("EMERGUP_TEST_KEPT", "kept")

	exec, out := newExec("")

	_, err := exec.Run(t.Context(), childCommand(runner.Stream, "env:EMERGUP_TEST_KEPT"))
	require.NoError(t, err)
	assert.Equal(t, "kept\n", out.stdout.String())
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

// lockedBuffer is a buffer that a test reads while a command writes to it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// A bytes.Buffer grows as it must, and never fails.
	_, _ = b.buf.Write(data)

	return len(data), nil
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

// watched runs cmd with a watch and returns what the watch got, a line
// each.
func watched(t *testing.T, cmd runner.Command) []string {
	t.Helper()

	exec, _ := newExec("")

	var watch lockedBuffer

	cmd.Watch = &watch

	_, err := exec.Run(t.Context(), cmd)
	require.NoError(t, err)

	text := watch.String()
	if text == "" {
		return nil
	}

	require.True(t, strings.HasSuffix(text, "\n"), "every line ends with a newline: %q", text)

	// The text ends with a newline, after which SplitAfter gives "".
	lines := strings.SplitAfter(text, "\n")

	return lines[:len(lines)-1]
}

func TestWatchGetsWhatTheLogGetsLineByLine(t *testing.T) {
	t.Parallel()

	for _, mode := range []runner.Mode{runner.Stream, runner.Capture, runner.StreamCapture} {
		lines := watched(t, childCommand(mode, "green:done", "bold:warning", "part:no newline"))
		require.NotEmpty(t, lines, mode)
		assert.ElementsMatch(t, []string{"done\n", "warning\n", "no newline\n"}, lines, mode)
		assert.Equal(t, "no newline\n", lines[len(lines)-1], "the last line gets a newline, once the command ends")
	}
}

func TestWatchGetsTheLinesOfEachStreamWhole(t *testing.T) {
	t.Parallel()

	lines := watched(t, childCommand(runner.Stream, "part:first ", "err:other", "out:half"))
	assert.ElementsMatch(t, []string{"first half\n", "other\n"}, lines,
		"a line of standard error does not cut one of standard output")
}

func TestWatchCutsAVeryLongLine(t *testing.T) {
	t.Parallel()

	lines := watched(t, childCommand(runner.Stream, "long:70000"))
	require.Len(t, lines, 2)
	assert.Len(t, lines[0], 64*1024+1, "64 KiB, then a newline")
	assert.Equal(t, strings.Repeat("x", 70000-64*1024)+"\n", lines[1])
}

func TestWatchGetsNothingOfATerminalCommand(t *testing.T) {
	t.Parallel()

	assert.Empty(t, watched(t, childCommand(runner.Terminal, "out:hello")))
}

func TestSaveWritesTheStandardOutputOnlyToInto(t *testing.T) {
	t.Parallel()

	exec, out := newExec("")

	var into bytes.Buffer

	cmd := childCommand(runner.Save, "green:content", "out:more", "err:warning")
	cmd.Into = &into

	result, err := exec.Run(t.Context(), cmd)
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Empty(t, result.Output)

	assert.Equal(t, greenEscape+"content"+resetEscape+"\nmore\n", into.String(), "as it is")
	assert.Empty(t, out.stdout.String(), "not shown")
	assert.Equal(t, "warning\n", out.stderr.String())

	log := out.log.String()
	assert.Contains(t, log, "\nwarning\n")
	assert.NotContains(t, log, "\ncontent\n", "not logged")
	assert.NotContains(t, log, "\nmore\n", "not logged")
	assert.Contains(t, log, "==> exit status 0 after ")
}

func TestSaveNeedsSomewhereToWrite(t *testing.T) {
	t.Parallel()

	exec, _ := newExec("")

	_, err := exec.Run(t.Context(), childCommand(runner.Save, "out:content"))
	require.Error(t, err)
}
