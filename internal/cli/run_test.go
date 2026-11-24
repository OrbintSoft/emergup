// SPDX-License-Identifier: GPL-3.0-or-later

package cli_test

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/OrbintSoft/emergup/internal/cli"
	"github.com/OrbintSoft/emergup/internal/lock"
	"github.com/OrbintSoft/emergup/internal/runner/runnertest"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// The lines of a run of the built-in steps.
const (
	question       = "==> Run this step? [Y/s/a/?] "
	withYes        = question + "run the step (--yes)\n"
	notAvailable   = "not available yet in this version of emergup"
	firstHeading   = "==> [1/6] sync: sync the repositories\n"
	lastHeading    = "==> [6/6] depclean: remove the packages no longer needed\n"
	finished       = "==> The run finished in <1s.\n"
	stopped        = "==> The run was stopped after <1s.\n"
	noTool         = "no elevation tool found: install sudo, doas or run0, or run emergup as root"
	runningAsRoot  = "==> Running as root: the commands run without sudo, doas or run0, and AI assistance is off.\n"
	optionYes      = "--yes"
	optionPretend  = "--pretend"
	userID         = 1000
	commandRun     = "run"
	invalidSyncKey = "[steps.sync]\nenabled = 1\n"
)

// system is a fake elevation.System with programs installed.
type system struct {
	programs []string
	euid     int
}

func (s system) LookPath(file string) (string, error) {
	if slices.Contains(s.programs, file) {
		return "/usr/bin/" + file, nil
	}

	return "", exec.ErrNotFound
}

func (s system) Geteuid() int { return s.euid }

// keys answers the questions with its keys, then with the end of the input.
// When silent, a question fails the test.
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

// locker is a fake lock.Locker that records whether it was taken and
// released; with err set it is held by another run.
type locker struct {
	err      error
	locked   bool
	released bool
}

func (l *locker) Lock(context.Context) (func() error, error) {
	if l.err != nil {
		return nil, l.err
	}

	l.locked = true

	return func() error {
		l.released = true

		return nil
	}, nil
}

// session is a system emergup runs on, for the run command.
type session struct {
	root   fs.FS
	keys   keys
	system system
	runner runnertest.Fake
	lock   locker
	color  bool
}

// newSession returns a system without configuration files, where an
// ordinary user has sudo, and a keyboard that answers with keys.
func newSession(t *testing.T, keys ...rune) *session {
	t.Helper()

	sess := &session{root: fstest.MapFS{}, system: system{programs: []string{"sudo"}, euid: userID}}
	sess.keys.t = t
	sess.keys.keys = keys

	return sess
}

// run runs emergup with args and returns the exit status and what was
// written to stdout and stderr.
func (s *session) run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()

	var stdout, stderr bytes.Buffer

	env := cli.Env{
		Stdout:   &stdout,
		Stderr:   &stderr,
		Root:     s.root,
		Keyboard: &s.keys,
		Runner:   &s.runner,
		System:   s.system,
		Lock:     &s.lock,
		Color:    s.color,
		Now:      func() time.Time { return time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC) },
	}
	code := cli.Main(t.Context(), env, "emergup", args)

	return code, stdout.String(), stderr.String()
}

func TestRunWithYesGoesThroughEveryStep(t *testing.T) {
	t.Parallel()

	for _, option := range []string{optionYes, "-y"} {
		t.Run(option, func(t *testing.T) {
			t.Parallel()

			sess := newSession(t)
			sess.keys.silent = true

			code, stdout, stderr := sess.run(t, commandRun, option)
			assert.Equal(t, 0, code)
			assert.Empty(t, stderr)
			assert.True(t, strings.HasPrefix(stdout, firstHeading+withYes+
				"==> sync skipped: "+notAvailable+"\n"), stdout)
			assert.Contains(t, stdout, lastHeading)
			assert.Contains(t, stdout, "==> Summary\n"+
				"    sync               skipped       <1s  "+notAvailable+"\n")
			assert.True(t, strings.HasSuffix(stdout, finished), stdout)
			assert.Empty(t, sess.runner.Commands(), "the steps not written yet run nothing")
		})
	}
}

func TestRunAsksBeforeEachStep(t *testing.T) {
	t.Parallel()

	sess := newSession(t, ui.KeyEnter, 'y', ui.KeyEnter, 'y', 's', ui.KeyEnter)

	code, stdout, _ := sess.run(t, commandRun)
	assert.Equal(t, 0, code)
	assert.Equal(t, 5, strings.Count(stdout, question+"run the step\n"))
	assert.Contains(t, stdout, question+"skip the step\n==> config-files skipped: you chose to skip it\n")
	assert.True(t, strings.HasSuffix(stdout, finished), stdout)
}

func TestRunExitsWith130WhenItIsStopped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		keys []rune
	}{
		{"the abort key", []rune{ui.KeyEnter, 'a'}},
		{"Ctrl-C", []rune{ui.KeyInterrupt}},
		{"the end of the input", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			code, stdout, _ := newSession(t, tt.keys...).run(t, commandRun)
			assert.Equal(t, 130, code)
			assert.True(t, strings.HasSuffix(stdout, stopped), stdout)
			assert.NotContains(t, stdout, lastHeading)
		})
	}
}

func TestRunPretendShowsTheStepsAndRunsNothing(t *testing.T) {
	t.Parallel()

	for _, option := range []string{optionPretend, "-p"} {
		t.Run(option, func(t *testing.T) {
			t.Parallel()

			sess := newSession(t)
			sess.keys.silent = true

			code, stdout, stderr := sess.run(t, commandRun, option)
			assert.Equal(t, 0, code)
			assert.Empty(t, stderr)
			assert.True(t, strings.HasPrefix(stdout, firstHeading+"    "+notAvailable+"\n"), stdout)
			assert.True(t, strings.HasSuffix(stdout, lastHeading+"    "+notAvailable+"\n"), stdout)
			assert.NotContains(t, stdout, "Summary")
			assert.Empty(t, sess.runner.Commands())
		})
	}
}

func TestRunWithoutAnElevationTool(t *testing.T) {
	t.Parallel()

	sess := newSession(t)
	sess.system.programs = nil
	sess.keys.silent = true

	code, stdout, _ := sess.run(t, commandRun, optionYes)
	assert.Equal(t, 1, code, "the run cannot start")
	assert.Equal(t, "==> "+noTool+"\n", stdout, "before the first step")

	code, stdout, _ = sess.run(t, commandRun, optionPretend)
	assert.Equal(t, 0, code, "pretend runs nothing, so it goes on")
	assert.True(t, strings.HasPrefix(stdout, "==> "+noTool+"; a run would stop before its first step\n"+
		firstHeading), stdout)
}

func TestRunAsRootSaysSoOnce(t *testing.T) {
	t.Parallel()

	for _, option := range []string{optionYes, optionPretend} {
		t.Run(option, func(t *testing.T) {
			t.Parallel()

			sess := newSession(t)
			sess.system = system{programs: nil, euid: 0}
			sess.keys.silent = true

			code, stdout, _ := sess.run(t, commandRun, option)
			assert.Equal(t, 0, code)
			assert.True(t, strings.HasPrefix(stdout, runningAsRoot+firstHeading), stdout)
			assert.Equal(t, 1, strings.Count(stdout, "Running as root"))
		})
	}
}

func TestRunWithAProblemInTheConfiguration(t *testing.T) {
	t.Parallel()

	sess := newSession(t)
	sess.root = fstest.MapFS{"etc/emergup/config.toml": {Data: []byte(invalidSyncKey)}}
	sess.keys.silent = true

	code, stdout, stderr := sess.run(t, commandRun, optionYes)
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "1 problem in the configuration.\n")
}

func TestRunFollowsTheConfiguration(t *testing.T) {
	t.Parallel()

	sess := newSession(t)
	sess.root = fstest.MapFS{"etc/emergup/config.toml": {Data: []byte(
		"[steps.live-rebuild]\nenabled = false\n" +
			"[steps.depclean]\nenabled = false\n" +
			"[colors]\nheading = \"underline\"\n",
	)}}
	sess.keys.silent = true
	sess.color = true

	code, stdout, _ := sess.run(t, commandRun, optionYes)
	assert.Equal(t, 0, code)
	assert.True(t, strings.HasPrefix(stdout, "\x1b[4m==> [1/4] sync: sync the repositories\x1b[0m\n"), stdout)
	assert.Contains(t, stdout, "\x1b[4m==> [4/4] config-files: update the configuration files\x1b[0m\n")
	assert.Contains(t, stdout, "    \x1b[33mdepclean           disabled\x1b[0m\n")
	assert.NotContains(t, stdout, "depclean: remove")
}

func TestRunWithoutColours(t *testing.T) {
	t.Parallel()

	sess := newSession(t)
	sess.keys.silent = true

	_, stdout, _ := sess.run(t, commandRun, optionYes)
	assert.Contains(t, stdout, firstHeading)
	assert.NotContains(t, stdout, "\x1b")
}

func TestRunUsageErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"an unknown option", []string{commandRun, "--bogus"}, `emergup run: unknown option "--bogus"`},
		{"an argument", []string{commandRun, "world"}, `emergup run: unexpected argument "world"`},
		{"combined short options", []string{commandRun, "-yp"}, `emergup run: unknown option "-yp"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sess := newSession(t)
			sess.keys.silent = true

			code, stdout, stderr := sess.run(t, tt.args...)
			assert.Equal(t, 2, code)
			assert.Empty(t, stdout)
			assert.Contains(t, stderr, tt.want+"\n\n"+usageLine)
		})
	}
}

func TestRunHoldsTheLockWhileItRuns(t *testing.T) {
	t.Parallel()

	sess := newSession(t)
	sess.keys.silent = true

	code, _, _ := sess.run(t, commandRun, optionYes)
	assert.Equal(t, 0, code)
	assert.True(t, sess.lock.locked, "the run takes the lock")
	assert.True(t, sess.lock.released, "and releases it at the end")
}

func TestRunRefusesToStartWhileAnotherRunIsActive(t *testing.T) {
	t.Parallel()

	sess := newSession(t)
	sess.lock.err = &lock.HeldError{PID: 1234, User: "mario"}
	sess.system = system{programs: nil, euid: 0}
	sess.keys.silent = true

	code, stdout, _ := sess.run(t, commandRun, optionYes)
	assert.Equal(t, 1, code)
	assert.Equal(t, "==> another emergup run is active: process 1234 of user mario\n", stdout,
		"before anything else")
}

func TestRunPretendDoesNotTakeTheLock(t *testing.T) {
	t.Parallel()

	sess := newSession(t)
	sess.lock.err = &lock.HeldError{PID: 1234, User: "mario"}
	sess.keys.silent = true

	code, stdout, _ := sess.run(t, commandRun, optionPretend)
	assert.Equal(t, 0, code)
	assert.True(t, strings.HasPrefix(stdout, firstHeading), stdout)
}
