// SPDX-License-Identifier: GPL-3.0-or-later

// Package runner starts the external commands emergup drives, such as emerge,
// shows or collects their output, and copies it to a log.
package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"syscall"
	"time"
)

// Errors for a Command that cannot be run.
var (
	errNoProgram   = errors.New("no program to run")
	errUnknownMode = errors.New("unknown mode")
)

// Mode says what happens to the output of a command.
type Mode int

// The modes.
const (
	// Stream shows the output as it comes and copies it to the log.
	Stream Mode = iota
	// Capture collects standard output in Result.Output, shows standard
	// error, and copies both to the log.
	Capture
	// Terminal connects the command to emergup's own standard streams, for
	// an interactive tool; the log records only the command and its end.
	Terminal
	// StreamCapture shows the output as it comes, copies it to the log, and
	// collects standard output in Result.Output, for a command whose output
	// the user reads and emergup reads too.
	StreamCapture
)

// Command is a program to run, with its arguments, and its Mode.
type Command struct {
	// Args holds the program, then its arguments.
	Args []string
	Mode Mode
	// Root marks a command that needs root; a runner that elevates runs it
	// through sudo, doas or run0.
	Root bool
}

// Result is how a command ended.
type Result struct {
	// ExitCode is the exit status, -1 when a signal ended the command.
	ExitCode int
	// Output is the standard output of a Capture or StreamCapture command.
	Output []byte
	// Duration is the time from the start of the command to its end.
	Duration time.Duration
}

// Runner runs commands.
type Runner interface {
	// Run runs cmd and returns how it ended. The error reports a command
	// that could not start, or a ctx that ended while it ran; a command
	// that fails is a non-zero Result.ExitCode, not an error.
	Run(ctx context.Context, cmd Command) (Result, error)
}

// DefaultGracePeriod is how long a command gets to stop after it is asked
// to, before it is killed.
const DefaultGracePeriod = 30 * time.Second

// Exec runs commands as child processes. When its context ends, a command
// gets SIGTERM and GracePeriod to stop by itself before it is killed: the
// terminal has already given it the SIGINT of a Ctrl-C, and a merge stopped
// abruptly can leave a package half installed.
type Exec struct {
	// Stdin, Stdout and Stderr are emergup's own standard streams; a nil
	// writer discards what is written to it.
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	// Log receives a copy of every command and its output, without the
	// terminal's escape sequences; nil keeps none. The log is best effort: a
	// failed write to it never stops the output to the terminal.
	Log io.Writer
	// GracePeriod is how long a command gets to stop once the context ends,
	// before it is killed; zero means DefaultGracePeriod.
	GracePeriod time.Duration
	// Now gives the time written in the log; nil means time.Now.
	Now func() time.Time
}

// Run implements Runner.
func (e *Exec) Run(ctx context.Context, cmd Command) (Result, error) {
	if len(cmd.Args) == 0 {
		return Result{}, errNoProgram
	}

	log := newLogWriter(e.Log)
	log.printf("==> %s %s\n", e.now().Format(time.RFC3339), commandLine(cmd.Args))

	//nolint:gosec // running the commands the configuration names is what emergup is for
	child := exec.CommandContext(ctx, cmd.Args[0], cmd.Args[1:]...)
	child.Cancel = func() error { return child.Process.Signal(syscall.SIGTERM) }
	child.WaitDelay = e.gracePeriod()

	var output bytes.Buffer

	if err := e.connect(child, cmd.Mode, log, &output); err != nil {
		return Result{}, err
	}

	start := time.Now()
	err := child.Run()
	result := Result{ExitCode: 0, Output: nil, Duration: time.Since(start)}

	if cmd.Mode == Capture || cmd.Mode == StreamCapture {
		result.Output = output.Bytes()
	}

	if err != nil {
		exitErr, exited := errors.AsType[*exec.ExitError](err)
		if !exited {
			log.printf("==> could not start: %v\n", err)

			return result, fmt.Errorf("running %s: %w", cmd.Args[0], err)
		}

		result.ExitCode = exitErr.ExitCode()
	}

	log.printf("==> %s after %s\n", ending(result.ExitCode), result.Duration.Round(time.Millisecond))

	if ctxErr := ctx.Err(); ctxErr != nil {
		return result, fmt.Errorf("running %s: %w", cmd.Args[0], ctxErr)
	}

	return result, nil
}

// connect gives child the standard streams of mode: output to show goes to
// emergup's own streams, output to collect to output, and both to log.
// Each stream has its own plainWriter: they are written to at the same
// time, and an escape sequence of one must not end in the other.
func (e *Exec) connect(child *exec.Cmd, mode Mode, log io.Writer, output io.Writer) error {
	switch mode {
	case Stream:
		child.Stdout = io.MultiWriter(orDiscard(e.Stdout), newPlainWriter(log))
		child.Stderr = io.MultiWriter(orDiscard(e.Stderr), newPlainWriter(log))
	case Capture:
		child.Stdout = io.MultiWriter(output, newPlainWriter(log))
		child.Stderr = io.MultiWriter(orDiscard(e.Stderr), newPlainWriter(log))
	case Terminal:
		child.Stdin, child.Stdout, child.Stderr = e.Stdin, e.Stdout, e.Stderr
	case StreamCapture:
		child.Stdout = io.MultiWriter(orDiscard(e.Stdout), newPlainWriter(log), output)
		child.Stderr = io.MultiWriter(orDiscard(e.Stderr), newPlainWriter(log))
	default:
		return fmt.Errorf("%w %d", errUnknownMode, mode)
	}

	return nil
}

func (e *Exec) now() time.Time {
	if e.Now == nil {
		return time.Now()
	}

	return e.Now()
}

func (e *Exec) gracePeriod() time.Duration {
	if e.GracePeriod == 0 {
		return DefaultGracePeriod
	}

	return e.GracePeriod
}

// ending says how a command with the exit code ended.
func ending(exitCode int) string {
	if exitCode < 0 {
		return "ended by a signal"
	}

	return fmt.Sprintf("exit status %d", exitCode)
}

func orDiscard(w io.Writer) io.Writer {
	if w == nil {
		return io.Discard
	}

	return w
}
