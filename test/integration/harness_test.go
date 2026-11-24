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
	"strings"
	"testing"
	"time"

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

	return c.exec(tester, input, append([]string{"emergup"}, args...)...)
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
// own, with no input. A terminal has a single output: everything is in
// stdout, each line ending with "\r\n".
func (c *container) emergupTerminal(args ...string) result {
	c.t.Helper()

	return c.command(c.t.Context(), nil, append([]string{"exec", "--tty", "--user", tester, c.id, "emergup"}, args...)...)
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

	return c.command(c.t.Context(), strings.NewReader(input),
		append([]string{"exec", "--interactive", "--user", user, c.id}, args...)...)
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
