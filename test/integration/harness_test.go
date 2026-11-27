// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

// Package integration_test runs the emergup binary in containers of a
// Gentoo image, with the real emerge, portageq and sudo, and checks what a
// user would see. `make integration` builds the binary and the image and
// runs the tests; each test names the feature it verifies.
package integration_test

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The environment variables through which `make integration` tells the
// tests what to run.
const (
	// The image the containers run.
	imageVariable = "EMERGUP_TEST_IMAGE"
	// The absolute path of the emergup binary under test.
	binaryVariable = "EMERGUP_TEST_BINARY"
	// The version stamped in that binary.
	versionVariable = "EMERGUP_TEST_VERSION"
	// The command that runs the containers; docker when it is not set.
	dockerVariable = "EMERGUP_TEST_DOCKER"
)

// program is emergup's command in the container: the binary under test.
const program = "emergup"

// interactive is the option of exec that gives a command its standard
// input.
const interactive = "--interactive"

// The users of the image.
const (
	// tester is an ordinary user who may run any command through sudo.
	tester = "tester"
	root   = "root"
)

// The emergup configuration of the image: its main file, which the image
// sets up, and the directory of the drop-ins, which it does not create.
const (
	configFile      = "/etc/emergup/config.toml"
	configDirectory = "/etc/emergup/config.d"
)

// The test repository: where Portage keeps it, and the upstream it syncs
// from, where a test publishes what the next sync should bring.
const (
	repository = "/var/db/repos/emergup-test"
	upstream   = "/srv/emergup-test"
)

// timestamp is where an rsync tree records when its snapshot was made.
const timestamp = "/metadata/timestamp.chk"

// container is a container of the test image that lives as long as its
// test, with the emergup binary under test on its PATH. It has no network:
// everything a test needs is in the image.
type container struct {
	t      *testing.T
	docker string
	id     string
	// snapshots counts the snapshots published to the upstream.
	snapshots int
}

// result is what a command run in a container did.
type result struct {
	stdout string
	stderr string
	code   int
}

// setting returns the value of the environment variable name, which `make
// integration` sets.
func setting(t *testing.T, name string) string {
	t.Helper()

	value := os.Getenv(name)
	if value == "" {
		t.Fatalf("%s is not set: run the integration tests with make integration", name)
	}

	return value
}

// start starts a container for the test t, removed when the test ends.
func start(t *testing.T) *container {
	t.Helper()

	ctr := &container{t: t, docker: cmp.Or(os.Getenv(dockerVariable), "docker"), id: "", snapshots: 0}

	started := ctr.command(t.Context(), nil, "run", "--detach", "--rm", "--network", "none",
		"--volume", setting(t, binaryVariable)+":/usr/local/bin/emergup:ro",
		setting(t, imageVariable), "sleep", "infinity")
	require.Equal(t, 0, started.code, "the container does not start: %s", started.stderr)

	ctr.id = strings.TrimSpace(started.stdout)

	// The test's context is cancelled before its cleanup runs.
	ctx := context.WithoutCancel(t.Context())

	t.Cleanup(func() {
		removed := ctr.command(ctx, nil, "rm", "--force", ctr.id)
		if removed.code != 0 {
			t.Errorf("the container %s is not removed: %s", ctr.id, removed.stderr)
		}
	})

	return ctr
}

// emergup runs emergup with args as tester, with no input.
func (c *container) emergup(args ...string) result {
	c.t.Helper()

	return c.emergupInput("", args...)
}

// emergupInput runs emergup with args as tester, with input on its
// standard input.
func (c *container) emergupInput(input string, args ...string) result {
	c.t.Helper()

	return c.emergupAs(tester, input, args...)
}

// emergupAs runs emergup with args as user, with input on its standard
// input.
func (c *container) emergupAs(user, input string, args ...string) result {
	c.t.Helper()

	return c.exec(user, input, append([]string{program}, args...)...)
}

// emergupWith runs emergup with args as tester, with no input and the
// variables of env, each NAME=value, added to its environment.
func (c *container) emergupWith(env []string, args ...string) result {
	c.t.Helper()

	options := make([]string, 0, 1+2*len(env))
	options = append(options, interactive)

	for _, variable := range env {
		options = append(options, "--env", variable)
	}

	args = append([]string{program}, args...)

	return c.command(c.t.Context(), strings.NewReader(""), c.execArgs(tester, options, args)...)
}

// emergupRun runs `emergup run` with options as tester, with input on its
// standard input, and fails the test unless it succeeds; it returns the
// standard output.
func (c *container) emergupRun(input string, options ...string) string {
	c.t.Helper()

	got := c.emergupInput(input, append([]string{"run"}, options...)...)
	require.Equal(c.t, 0, got.code, "the exit status, with the output:\n%s%s", got.stdout, got.stderr)

	return got.stdout
}

// emergupTerminal runs emergup with args as tester on a terminal of its
// own, with no input and the variables of env, each NAME=value, added to
// its environment. A terminal has a single output: everything is in
// stdout, each line ending with "\r\n".
func (c *container) emergupTerminal(env []string, args ...string) result {
	c.t.Helper()

	options := make([]string, 0, 1+2*len(env))
	options = append(options, "--tty")

	for _, variable := range env {
		options = append(options, "--env", variable)
	}

	return c.command(c.t.Context(), nil, c.execArgs(tester, options, append([]string{program}, args...))...)
}

// runTimeout is how long a run in the background gets to end once it has
// all its input.
const runTimeout = 2 * time.Minute

// background is an emergup run that goes on while the test does: the test
// reads its output as it comes and gives it its input.
type background struct {
	t   *testing.T
	cmd *exec.Cmd
	// input is the run's standard input, or the keyboard of its terminal.
	input io.Writer
	// endInput ends the input of a run that reads it from a pipe; nil for
	// a terminal, whose input ends with the run.
	endInput func() error
	output   *syncBuffer
	// copied, when not nil, is closed once the whole output is in output.
	copied chan struct{}
	// ended receives how the command ended; finished says that finish
	// received it.
	ended    chan error
	finished bool
}

// begin starts the command of the run and waits for its end in the
// background; release frees what the run uses, once it has ended. A test
// that fails before the end of the run ends it: the test's context, which
// ends first, stops the command.
func (b *background) begin(release func()) {
	b.t.Helper()

	require.NoError(b.t, b.cmd.Start())

	b.ended = make(chan error, 1)

	go func() { b.ended <- b.cmd.Wait() }()

	b.t.Cleanup(func() {
		if !b.finished {
			if b.endInput != nil {
				_ = b.endInput()
			}

			b.t.Logf("the run ended before the test finished it: %v", <-b.ended)
		}

		release()
	})
}

// emergupBackground starts emergup with args as tester, with its standard
// input open and empty, so that it waits at its first question.
func (c *container) emergupBackground(args ...string) *background {
	c.t.Helper()

	//nolint:gosec // The tests choose what runs.
	cmd := exec.CommandContext(c.t.Context(), c.docker,
		c.execArgs(tester, []string{interactive}, append([]string{program}, args...))...)

	input, err := cmd.StdinPipe()
	require.NoError(c.t, err)

	output := &syncBuffer{mutex: sync.Mutex{}, buffer: bytes.Buffer{}}
	cmd.Stdout = output
	cmd.Stderr = output

	run := &background{
		t: c.t, cmd: cmd, input: input, endInput: input.Close, output: output, copied: nil,
		ended: nil, finished: false,
	}
	run.begin(func() {})

	return run
}

// waitFor waits until the output of the run holds text.
func (b *background) waitFor(text string) {
	b.t.Helper()

	require.Eventually(b.t, func() bool { return strings.Contains(b.output.String(), text) },
		30*time.Second, 100*time.Millisecond, "the output never holds %q:\n%s", text, b.output)
}

// send gives the run text as its input: lines, or the keys pressed on its
// terminal.
func (b *background) send(text string) {
	b.t.Helper()

	_, err := io.WriteString(b.input, text)
	require.NoError(b.t, err)
}

// finish gives the run the rest of its input, then its end, and returns
// what it did, both of its outputs in stdout.
func (b *background) finish(input string) result {
	b.t.Helper()

	b.send(input)

	if b.endInput != nil {
		require.NoError(b.t, b.endInput())
	}

	var err error

	select {
	case err = <-b.ended:
		b.finished = true
	case <-time.After(runTimeout):
		// The end of the test stops the run.
		require.FailNow(b.t, "the run does not end", "its output:\n%s", b.output)
	}

	var exitError *exec.ExitError
	if err != nil && !errors.As(err, &exitError) {
		require.NoError(b.t, err)
	}

	if b.copied != nil {
		<-b.copied
	}

	return result{stdout: b.output.String(), stderr: "", code: b.cmd.ProcessState.ExitCode()}
}

// syncBuffer is a buffer that a command writes while the test reads it.
type syncBuffer struct {
	mutex  sync.Mutex
	buffer bytes.Buffer
}

// Write implements io.Writer.
func (s *syncBuffer) Write(data []byte) (int, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.buffer.Write(data) //nolint:wrapcheck // bytes.Buffer.Write returns no error.
}

// String returns what was written so far.
func (s *syncBuffer) String() string {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.buffer.String()
}

// assertInOrder checks that text holds each of parts, each after the one
// before it.
func assertInOrder(t *testing.T, text string, parts ...string) {
	t.Helper()

	rest := text

	for _, part := range parts {
		_, after, found := strings.Cut(rest, part)
		if !assert.True(t, found, "%q is missing, or comes too early, in:\n%s", part, text) {
			return
		}

		rest = after
	}
}

// logLine is the last line of a run, which says where its log is.
var logLine = regexp.MustCompile(`==> The log is in (\S+)\r?\n$`)

// logPath returns the path of the log of the run whose output is stdout.
func logPath(t *testing.T, stdout string) string {
	t.Helper()

	found := logLine.FindStringSubmatch(stdout)
	require.NotNil(t, found, "the run does not say where its log is:\n%s", stdout)

	return found[1]
}

// runLog returns the log of the run whose output is stdout, read as user.
func (c *container) runLog(user, stdout string) string {
	c.t.Helper()

	path := logPath(c.t, stdout)

	read := c.exec(user, "", "cat", path)
	require.Equal(c.t, 0, read.code, "the log %s is not read: %s", path, read.stderr)

	return read.stdout
}

// root runs args as root, as the set-up of a test, which fails when the
// command does.
func (c *container) root(args ...string) {
	c.t.Helper()

	done := c.exec(root, "", args...)
	require.Equal(c.t, 0, done.code, "%v failed: %s", args, done.stderr)
}

// exists says whether the file at path exists in the container.
func (c *container) exists(path string) bool {
	c.t.Helper()

	return c.exec(root, "", "test", "-e", path).code == 0
}

// install copies the file source of the host to target in the container,
// owned by root and readable by every user, as a configuration file is.
func (c *container) install(source, target string) {
	c.t.Helper()

	copied := c.command(c.t.Context(), nil, "cp", source, c.id+":"+target)
	require.Equal(c.t, 0, copied.code, "%s is not copied: %s", source, copied.stderr)

	c.root("chown", "root:root", target)
	c.root("chmod", "0644", target)
}

// publish publishes a new snapshot of the test repository, with an empty
// file named name added.
func (c *container) publish(name string) {
	c.t.Helper()

	c.root("touch", upstream+"/"+name)
	c.stamp()
}

// publishTree publishes a new snapshot of the test repository, with the
// files of testdata/name added at the same paths, such as the ebuilds of
// testdata/name/emergup-test/<package>.
func (c *container) publishTree(name string) {
	c.t.Helper()

	copied := c.command(c.t.Context(), nil, "cp", filepath.Join("testdata", name)+"/.", c.id+":"+upstream)
	require.Equal(c.t, 0, copied.code, "%s is not copied: %s", name, copied.stderr)

	c.root("chown", "-R", "root:root", upstream)
	c.stamp()
}

// stamp dates the snapshot of the upstream: Portage syncs an rsync tree
// only when the timestamp of its upstream has changed, so each snapshot
// gets a timestamp later than every one before it, the first one a second
// from now.
func (c *container) stamp() {
	c.t.Helper()

	c.snapshots++
	made := time.Now().UTC().Truncate(time.Second).Add(time.Duration(c.snapshots) * time.Second)

	stamp := filepath.Join(c.t.TempDir(), "timestamp.chk")
	require.NoError(c.t, os.WriteFile(stamp, []byte(made.Format(time.RFC1123Z)+"\n"), 0o600))

	c.install(stamp, upstream+timestamp)
}

// syncRepository syncs the test repository with emerge, as root, outside
// emergup.
func (c *container) syncRepository() {
	c.t.Helper()

	c.root("emerge", "--sync", "emergup-test")
}

// version returns the version of the package pn of the test repository
// that is installed: each one installs a file that holds it.
func (c *container) version(pn string) string {
	c.t.Helper()

	got := c.exec(root, "", "cat", "/usr/share/emergup-test/"+pn)
	require.Equal(c.t, 0, got.code, "%s is not installed: %s", pn, got.stderr)

	return strings.TrimSpace(got.stdout)
}

// dropIns installs every file of testdata/name in /etc/emergup/config.d.
func (c *container) dropIns(name string) {
	c.t.Helper()

	directory := filepath.Join("testdata", name)

	files, err := os.ReadDir(directory)
	require.NoError(c.t, err)

	c.root("mkdir", "-p", configDirectory)

	for _, file := range files {
		c.install(filepath.Join(directory, file.Name()), configDirectory+"/"+file.Name())
	}
}

// exec runs args in the container as user, with input on its standard
// input.
func (c *container) exec(user, input string, args ...string) result {
	c.t.Helper()

	return c.command(c.t.Context(), strings.NewReader(input), c.execArgs(user, []string{interactive}, args)...)
}

// execArgs returns the arguments of the container command that runs args
// in the container as user, with the options of exec.
func (c *container) execArgs(user string, options, args []string) []string {
	return slices.Concat([]string{"exec"}, options, []string{"--user", user, c.id}, args)
}

// command runs the container command with args, and input on its standard
// input when it is not nil, and returns what it did; the test fails when
// the command cannot run at all.
func (c *container) command(ctx context.Context, input io.Reader, args ...string) result {
	c.t.Helper()

	var stdout, stderr bytes.Buffer

	cmd := exec.CommandContext(ctx, c.docker, args...) //nolint:gosec // The tests choose what runs.
	cmd.Stdin = input
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	var exitError *exec.ExitError
	if err != nil && !errors.As(err, &exitError) {
		require.NoError(c.t, err)
	}

	return result{stdout: stdout.String(), stderr: stderr.String(), code: cmd.ProcessState.ExitCode()}
}
