// SPDX-License-Identifier: GPL-3.0-or-later

package cli_test

import (
	"bytes"
	"context"
	"errors"
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
	"github.com/OrbintSoft/emergup/internal/rundir"
	"github.com/OrbintSoft/emergup/internal/runner"
	"github.com/OrbintSoft/emergup/internal/runner/runnertest"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// The lines of a run of the built-in steps.
const (
	question        = "==> Run this step? [Y/s/a/?] "
	withYes         = question + "run the step (--yes)\n"
	noRepository    = "no repository to sync"
	portageq        = "portageq"
	sudo            = "sudo"
	nothingToUpdate = "Total: 0 packages, Size of downloads: 0 KiB\n"
	firstHeading    = "==> [1/6] sync: sync the repositories\n"
	lastHeading     = "==> [6/6] depclean: remove the packages no longer needed\n"
	finished        = "==> The run finished in <1s.\n"
	stopped         = "==> The run was stopped after <1s.\n"
	noTool          = "no elevation tool found: install sudo, doas or run0, or run emergup as root"
	runningAsRoot   = "==> Running as root: the commands run without sudo, doas or run0, and AI assistance is off.\n"
	optionYes       = "--yes"
	optionPretend   = "--pretend"
	userID          = 1000
	commandRun      = "run"
	invalidSyncKey  = "[steps.sync]\nenabled = 1\n"
	runDir          = "/state/emergup/runs/2026-11-24T10-00-00"
	configFile      = "etc/emergup/config.toml"
	logLine         = "==> The log is in " + runDir + "/emergup.log\n"
)

// listRepositories is the command the sync lists the repositories with.
//
//nolint:gochecknoglobals // The query several tests expect.
var listRepositories = runner.Command{
	Args: []string{portageq, "repos_config", "/"}, Mode: runner.Capture, Root: false,
}

// queryVDB is the command that asks where the installed packages are.
//
//nolint:gochecknoglobals // The query several tests expect.
var queryVDB = runner.Command{Args: []string{portageq, "vdb_path"}, Mode: runner.Capture, Root: false}

// depcleanPretend is the command the depclean shows what it would remove
// with.
//
//nolint:gochecknoglobals // The command several tests expect.
var depcleanPretend = runner.Command{
	Args: []string{"emerge", "--depclean", optionPretend, "--color=n"}, Mode: runner.StreamCapture, Root: false,
}

// queryProtected is the command that asks which configuration Portage
// protects.
//
//nolint:gochecknoglobals // The query several tests expect.
var queryProtected = runner.Command{
	Args: []string{portageq, "envvar", "CONFIG_PROTECT", "CONFIG_PROTECT_MASK"}, Mode: runner.Capture, Root: false,
}

// listPreserved is the command that lists the preserved libraries,
// through sudo.
//
//nolint:gochecknoglobals // The query several tests expect.
var listPreserved = runner.Command{
	Args: []string{sudo, portageq, "list_preserved_libs", "/"}, Mode: runner.Capture, Root: true,
}

// updatePretend is the command the update shows what it would do with.
//
//nolint:gochecknoglobals // The command several tests expect.
var updatePretend = runner.Command{
	Args: []string{
		"emerge", "--verbose", "--update", "--deep", "--newuse", "--verbose-conflicts", "--pretend", "--color=n",
		"@world",
	},
	Mode: runner.StreamCapture, Root: false,
}

// Errors of the fake run directories.
var (
	errNoState    = errors.New("no state directory: neither XDG_STATE_HOME nor HOME is an absolute path")
	errPermission = errors.New("permission denied")
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

// logFile is the log of a run, kept in memory.
type logFile struct {
	bytes.Buffer

	closed bool
}

func (l *logFile) Close() error {
	l.closed = true

	return nil
}

// store is a fake cli.RunStore: it makes no directory, and keeps the log
// of the run in memory. With err set it cannot make the directory.
type store struct {
	err      error
	pruneErr error
	log      logFile
	created  bool
	kept     int
	current  string
}

func (s *store) Create(time.Time) (rundir.Run, error) {
	if s.err != nil {
		return rundir.Run{}, s.err
	}

	s.created = true

	return rundir.Run{Dir: runDir, LogPath: runDir + "/emergup.log", Log: &s.log}, nil
}

func (s *store) Prune(keep int, current string) error {
	s.kept, s.current = keep, current

	return s.pruneErr
}

// The kinds of command portage answers.
const (
	kindPretend    = "pretend"
	kindDepclean   = "depclean pretend"
	kindVDB        = "portageq vdb_path"
	kindRepository = "portageq repos_config"
)

// portage is a runner.Runner for a system's Portage: it answers each
// command with what outputs holds for its kind, and records every command.
type portage struct {
	recorded runnertest.Fake
	outputs  map[string]string
}

// commandKind says what cmd does, for portage: kindDepclean or
// kindPretend for a pretend, otherwise its first two words.
func commandKind(cmd runner.Command) string {
	switch {
	case slices.Contains(cmd.Args, optionPretend) && slices.Contains(cmd.Args, "--depclean"):
		return kindDepclean
	case slices.Contains(cmd.Args, optionPretend):
		return kindPretend
	}

	return strings.Join(cmd.Args[:min(2, len(cmd.Args))], " ")
}

func (p *portage) Run(ctx context.Context, cmd runner.Command) (runner.Result, error) {
	if _, err := p.recorded.Run(ctx, cmd); err != nil {
		return runner.Result{}, err //nolint:wrapcheck // The fake's own error.
	}

	return runner.Result{ExitCode: 0, Output: []byte(p.outputs[commandKind(cmd)]), Duration: 0}, nil
}

// Commands returns the commands run so far, in order.
func (p *portage) Commands() []runner.Command {
	return p.recorded.Commands()
}

// session is a system emergup runs on, for the run command.
type session struct {
	root      fs.FS
	keys      keys
	system    system
	runner    portage
	runnerLog io.Writer
	runs      store
	lock      locker
	color     bool
}

// newSession returns a system without configuration files, where an
// ordinary user has sudo, and a keyboard that answers with keys. Its
// Portage has no repository to sync and nothing to update.
func newSession(t *testing.T, keys ...rune) *session {
	t.Helper()

	sess := &session{root: fstest.MapFS{}, system: system{programs: []string{sudo}, euid: userID}}
	sess.keys.t = t
	sess.keys.keys = keys
	sess.runner.outputs = map[string]string{
		kindPretend:    nothingToUpdate,
		kindDepclean:   "Number to remove:     0\n",
		kindVDB:        "/var/db/pkg\n",
		kindRepository: "",
	}

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
		NewRunner: func(log io.Writer) runner.Runner {
			s.runnerLog = log

			return &s.runner
		},
		Runs:   &s.runs,
		System: s.system,
		Lock:   &s.lock,
		Color:  s.color,
		Now:    func() time.Time { return time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC) },
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
				"==> sync skipped: "+noRepository+"\n"), stdout)
			assert.Contains(t, stdout, lastHeading)
			assert.Contains(t, stdout, "==> Summary\n"+
				"    sync               skipped       <1s  "+noRepository+"\n"+
				"    update             skipped       <1s  nothing to update\n")
			assert.True(t, strings.HasSuffix(stdout, finished+logLine), stdout)
			assert.Equal(t, []runner.Command{
				listRepositories, updatePretend, queryVDB, listPreserved, queryProtected, depcleanPretend,
			}, sess.runner.Commands(),
				"the queries and the pretends, unprivileged but for the preserved libraries; with nothing to do, nothing more")
		})
	}
}

func TestRunAsksBeforeEachStep(t *testing.T) {
	t.Parallel()

	sess := newSession(t, ui.KeyEnter, 's')

	code, stdout, _ := sess.run(t, commandRun)
	assert.Equal(t, 0, code)
	assert.Equal(t, 1, strings.Count(stdout, question+"run the step\n"),
		"the steps with nothing to do ask nothing")
	assert.Contains(t, stdout, question+"skip the step\n==> live-rebuild skipped: you chose to skip it\n")
	assert.True(t, strings.HasSuffix(stdout, finished+logLine), stdout)
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
			assert.True(t, strings.HasSuffix(stdout, stopped+logLine), stdout)
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
			assert.True(t, strings.HasPrefix(stdout, firstHeading+"    "+noRepository+"\n"), stdout)
			assert.True(t, strings.HasSuffix(stdout, lastHeading+
				"    emerge --depclean --pretend --color=n\n"+
				"    then, once confirmed: emerge --depclean --color=n\n"), stdout)
			assert.NotContains(t, stdout, "Summary")
			assert.Equal(t, []runner.Command{listRepositories, queryVDB}, sess.runner.Commands(), "nothing elevated")
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
	assert.Equal(t, "==> "+noTool+"\n"+logLine, stdout, "before the first step")

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
	sess.root = fstest.MapFS{configFile: {Data: []byte(invalidSyncKey)}}
	sess.keys.silent = true

	code, stdout, stderr := sess.run(t, commandRun, optionYes)
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "1 problem in the configuration.\n")
}

func TestRunFollowsTheConfiguration(t *testing.T) {
	t.Parallel()

	sess := newSession(t)
	sess.root = fstest.MapFS{configFile: {Data: []byte(
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
	sess.lock.err = &lock.HeldError{PID: 1234}
	sess.system = system{programs: nil, euid: 0}
	sess.keys.silent = true

	code, stdout, _ := sess.run(t, commandRun, optionYes)
	assert.Equal(t, 1, code)
	assert.Equal(t, "==> another emergup run of this user is active: process 1234\n", stdout,
		"before anything else")
}

func TestRunPretendDoesNotTakeTheLock(t *testing.T) {
	t.Parallel()

	sess := newSession(t)
	sess.lock.err = &lock.HeldError{PID: 1234}
	sess.keys.silent = true

	code, stdout, _ := sess.run(t, commandRun, optionPretend)
	assert.Equal(t, 0, code)
	assert.True(t, strings.HasPrefix(stdout, firstHeading), stdout)
}

func TestRunKeepsALogOfWhatItShows(t *testing.T) {
	t.Parallel()

	sess := newSession(t, ui.KeyEnter, '?', 's')

	code, stdout, _ := sess.run(t, commandRun)
	assert.Equal(t, 0, code)
	assert.Equal(t, stdout, sess.runs.log.String(), "the questions, the answers and the summary too")
	assert.True(t, sess.runs.log.closed)
	assert.Same(t, &sess.runs.log, sess.runnerLog, "the commands and their output go to the log")
}

func TestRunKeepsItsLogWithoutColours(t *testing.T) {
	t.Parallel()

	sess := newSession(t)
	sess.keys.silent = true
	sess.color = true

	_, stdout, _ := sess.run(t, commandRun, optionYes)
	assert.Contains(t, stdout, "\x1b[")
	assert.Contains(t, sess.runs.log.String(), firstHeading+withYes)
	assert.NotContains(t, sess.runs.log.String(), "\x1b")
}

func TestRunRemovesTheOldestRunDirectories(t *testing.T) {
	t.Parallel()

	sess := newSession(t)
	sess.keys.silent = true

	code, _, _ := sess.run(t, commandRun, optionYes)
	assert.Equal(t, 0, code)
	assert.Equal(t, 10, sess.runs.kept, "by default")
	assert.Equal(t, runDir, sess.runs.current, "the current run is kept")

	sess = newSession(t)
	sess.keys.silent = true
	sess.root = fstest.MapFS{configFile: {Data: []byte("[runs]\nkeep = 3\n")}}

	code, _, _ = sess.run(t, commandRun, optionYes)
	assert.Equal(t, 0, code)
	assert.Equal(t, 3, sess.runs.kept, "as the configuration says")
}

func TestRunGoesOnWhenTheOldRunDirectoriesCannotBeRemoved(t *testing.T) {
	t.Parallel()

	sess := newSession(t)
	sess.keys.silent = true
	sess.runs.pruneErr = errPermission

	code, stdout, _ := sess.run(t, commandRun, optionYes)
	assert.Equal(t, 0, code)
	assert.True(t, strings.HasPrefix(stdout,
		"==> could not remove the oldest run directories: permission denied\n"+firstHeading), stdout)
}

func TestRunDoesNotStartWithoutARunDirectory(t *testing.T) {
	t.Parallel()

	sess := newSession(t)
	sess.keys.silent = true
	sess.runs.err = errNoState

	code, stdout, _ := sess.run(t, commandRun, optionYes)
	assert.Equal(t, 1, code)
	assert.Equal(t, "==> "+errNoState.Error()+"\n", stdout)
}

func TestRunPretendMakesNoRunDirectory(t *testing.T) {
	t.Parallel()

	sess := newSession(t)
	sess.keys.silent = true

	code, stdout, _ := sess.run(t, commandRun, optionPretend)
	assert.Equal(t, 0, code)
	assert.False(t, sess.runs.created)
	assert.Nil(t, sess.runnerLog)
	assert.NotContains(t, stdout, "The log is in")
}

func TestRunMakesNoRunDirectoryWhileAnotherRunIsActive(t *testing.T) {
	t.Parallel()

	sess := newSession(t)
	sess.keys.silent = true
	sess.lock.err = &lock.HeldError{PID: 1234}

	code, _, _ := sess.run(t, commandRun, optionYes)
	assert.Equal(t, 1, code)
	assert.False(t, sess.runs.created)
}

func TestRunForceSyncSyncsEveryRepository(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"without it", nil, "    gentoo  synced 1h00m ago, kept\n    nothing to sync\n"},
		{"with it", []string{"--force-sync"}, "    gentoo  --force-sync\n    emerge --sync --color=n gentoo\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sess := newSession(t)
			sess.keys.silent = true
			sess.root = fstest.MapFS{
				"var/db/repos/gentoo/metadata/timestamp.chk": {Data: []byte("Tue, 24 Nov 2026 09:00:00 +0000\n")},
			}
			sess.runner.outputs[kindRepository] = "[gentoo]\nlocation = /var/db/repos/gentoo\nsync-type = rsync\n"

			code, stdout, _ := sess.run(t, append([]string{commandRun, optionPretend}, tt.args...)...)
			assert.Equal(t, 0, code)
			assert.Contains(t, stdout, firstHeading+tt.want)
		})
	}
}

func TestRunUpdatesOnceTheUserConfirms(t *testing.T) {
	t.Parallel()

	sess := newSession(t, ui.KeyEnter, ui.KeyEnter, 'a')
	sess.runner.outputs[kindPretend] = "Total: 2 packages (2 upgrades)\n"

	code, stdout, _ := sess.run(t, commandRun)
	assert.Equal(t, 130, code, "stopped at the next step")
	assert.Contains(t, stdout, "==> [2/6] update: update @world\n"+
		"==> Update these packages? [Y/s/a/?] update them\n"+
		"==> update succeeded in <1s\n", "one question, after the pretend")

	elevated := runner.Command{
		Args: append([]string{sudo}, slices.DeleteFunc(slices.Clone(updatePretend.Args),
			func(arg string) bool { return arg == "--pretend" })...),
		Mode: runner.Stream,
		Root: true,
	}
	assert.Equal(t, []runner.Command{listRepositories, updatePretend, elevated}, sess.runner.Commands())
}
