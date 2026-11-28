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
	"os"
	"os/exec"
	"syscall"
	"time"
)

// Errors for a Command that cannot be run.
var (
	errNoProgram   = errors.New("no program to run")
	errUnknownMode = errors.New("unknown mode")
	errNoInto      = errors.New("a Save command needs somewhere to write")
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
	// Save writes the standard output, as it is, to Command.Into, for a
	// command whose output is the content of a file, which neither the
	// terminal nor the log is to get; standard error is shown and logged.
	Save
)

// Command is a program to run, with its arguments, and its Mode.
type Command struct {
	// Args holds the program, then its arguments.
	Args []string
	Mode Mode
	// Root marks a command that needs root; a runner that elevates runs it
	// through sudo, doas or run0.
	Root bool
	// Env holds variables, NAME=value, that the command gets on top of
	// emergup's own environment; a name emergup has already gets the value
	// given here.
	Env []string
	// Watch, when set, gets what the log gets, as it comes, a whole line
	// at a time: a way to look at the output of a command without keeping
	// it. A Terminal command gives it nothing.
	Watch io.Writer
	// Into gets the standard output of a Save command.
	Into io.Writer
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

	if len(cmd.Env) > 0 {
		// exec keeps the last value of a name that appears twice.
		child.Env = append(os.Environ(), cmd.Env...)
	}

	var output bytes.Buffer

	watch := newWatcher(cmd.Watch)

	if err := e.connect(child, cmd, log, watch, &output); err != nil {
		return Result{}, err
	}

	start := time.Now()
	err := child.Run()

	watch.flush()

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

// connect gives child the standard streams of cmd's mode: output to show
// goes to emergup's own streams, output to collect to output, output to
// save to cmd.Into, and what is logged to log and watch. Each stream has
// its own plainWriter: they are written to at the same time, and an escape
// sequence of one must not end in the other.
func (e *Exec) connect(child *exec.Cmd, cmd Command, log io.Writer, watch *watcher, output io.Writer) error {
	logged := func() io.Writer { return newPlainWriter(io.MultiWriter(log, watch.stream())) }

	switch cmd.Mode {
	case Stream:
		child.Stdout = io.MultiWriter(orDiscard(e.Stdout), logged())
		child.Stderr = io.MultiWriter(orDiscard(e.Stderr), logged())
	case Capture:
		child.Stdout = io.MultiWriter(output, logged())
		child.Stderr = io.MultiWriter(orDiscard(e.Stderr), logged())
	case Terminal:
		child.Stdin, child.Stdout, child.Stderr = e.Stdin, e.Stdout, e.Stderr
	case StreamCapture:
		child.Stdout = io.MultiWriter(orDiscard(e.Stdout), logged(), output)
		child.Stderr = io.MultiWriter(orDiscard(e.Stderr), logged())
	case Save:
		if cmd.Into == nil {
			return errNoInto
		}

		child.Stdout = cmd.Into
		child.Stderr = io.MultiWriter(orDiscard(e.Stderr), logged())
	default:
		return fmt.Errorf("%w %d", errUnknownMode, cmd.Mode)
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
