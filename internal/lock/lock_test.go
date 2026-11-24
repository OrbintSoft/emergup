// SPDX-License-Identifier: GPL-3.0-or-later

package lock_test

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/OrbintSoft/emergup/internal/lock"
)

// holderMarker, as the first argument of the test binary, makes it a child
// process that takes the lock file named by the second argument, writes
// "locked" and holds it until its standard input ends.
const holderMarker = "-emergup-test-holder"

func TestMain(m *testing.M) {
	if len(os.Args) > 2 && os.Args[1] == holderMarker {
		os.Exit(hold(os.Args[2]))
	}

	os.Exit(m.Run())
}

func hold(path string) int {
	release, err := lock.Acquire(path)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)

		return 1
	}

	_, _ = fmt.Fprintln(os.Stdout, "locked")
	_, _ = io.Copy(io.Discard, os.Stdin)

	_ = release()

	return 0
}

// lockPath returns the path of a lock file in a new directory.
func lockPath(t *testing.T) string {
	t.Helper()

	return filepath.Join(t.TempDir(), "run.lock")
}

// read returns what the file at path holds.
func read(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path) //nolint:gosec // A file the test made.
	require.NoError(t, err)

	return string(data)
}

func TestAcquireTakesTheLockOnce(t *testing.T) {
	t.Parallel()

	path := lockPath(t)

	release, err := lock.Acquire(path)
	require.NoError(t, err)
	assert.Equal(t, strconv.Itoa(os.Getpid())+"\n", read(t, path), "the lock says which process holds it")

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o600), info.Mode().Perm(), "only the user can open the lock")

	_, err = lock.Acquire(path)
	held, isHeld := errors.AsType[*lock.HeldError](err)
	require.True(t, isHeld, "a second lock fails: %v", err)
	assert.Equal(t, &lock.HeldError{PID: os.Getpid()}, held)

	require.NoError(t, release())
	assert.Empty(t, read(t, path), "a released lock names no process")
	assert.FileExists(t, path, "the lock file is never removed")

	release, err = lock.Acquire(path)
	require.NoError(t, err, "released, the lock can be taken again")
	require.NoError(t, release())
}

// TestAcquireIsFreedWhenItsProcessEnds starts a process, so it does not run
// alongside the other tests: between its fork and its exec the new process
// holds a copy of every descriptor, a lock released meanwhile among them,
// which stays taken until the exec.
//
//nolint:paralleltest // See above.
func TestAcquireIsFreedWhenItsProcessEnds(t *testing.T) {
	path := lockPath(t)

	//nolint:gosec // The test binary itself, as the process that holds the lock.
	holder := exec.CommandContext(t.Context(), os.Args[0], holderMarker, path)
	stdin, err := holder.StdinPipe()
	require.NoError(t, err)

	stdout, err := holder.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, holder.Start())

	t.Cleanup(func() {
		_ = stdin.Close()
		_ = holder.Process.Kill()
		_ = holder.Wait()
	})

	line, err := bufio.NewReader(stdout).ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "locked\n", line)

	_, err = lock.Acquire(path)
	held, isHeld := errors.AsType[*lock.HeldError](err)
	require.True(t, isHeld, "another process holds the lock: %v", err)
	assert.Equal(t, holder.Process.Pid, held.PID)

	require.NoError(t, holder.Process.Kill(), "the holder ends without releasing the lock")

	_ = holder.Wait()

	release, err := lock.Acquire(path)
	require.NoError(t, err, "the kernel freed the lock")
	require.NoError(t, release())
}

func TestAcquireDoesNotFollowASymlink(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	//nolint:gosec // A file the group can read, which the lock must leave as it is.
	require.NoError(t, os.WriteFile(target, []byte("untouched\n"), 0o640))

	path := filepath.Join(dir, "run.lock")
	require.NoError(t, os.Symlink(target, path))

	_, err := lock.Acquire(path)
	require.Error(t, err)
	assert.Equal(t, "untouched\n", read(t, target))

	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o640), info.Mode().Perm(), "the target is not changed")
}

func TestAcquireRefusesWhatIsNotARegularFile(t *testing.T) {
	t.Parallel()

	fifo := lockPath(t)
	require.NoError(t, unix.Mkfifo(fifo, 0o600))

	_, err := lock.Acquire(fifo)
	require.ErrorContains(t, err, "is not a regular file")

	dir := lockPath(t)
	require.NoError(t, os.Mkdir(dir, 0o700))

	_, err = lock.Acquire(dir)
	require.Error(t, err)
}

func TestAcquireMakesALockOthersCouldReadTheUsersAlone(t *testing.T) {
	t.Parallel()

	path := lockPath(t)
	//nolint:gosec // A lock others can read, which emergup makes the user's alone.
	require.NoError(t, os.WriteFile(path, nil, 0o644))

	release, err := lock.Acquire(path)
	require.NoError(t, err)
	require.NoError(t, release())

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o600), info.Mode().Perm())
}

func TestHeldErrorSaysWhoHoldsTheLock(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "another emergup run of this user is active: process 1234",
		(&lock.HeldError{PID: 1234}).Error())
	assert.Equal(t, "another emergup run of this user is active", (&lock.HeldError{PID: 0}).Error(),
		"when the holder is not known")
}
