// SPDX-License-Identifier: GPL-3.0-or-later

package lock_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/emergup/internal/lock"
)

// holderMarker, as the first argument of the test binary, makes it a child
// process that takes the lock named by the second argument, writes
// "locked" and holds it until its standard input ends.
const holderMarker = "-emergup-test-holder"

func TestMain(m *testing.M) {
	if len(os.Args) > 2 && os.Args[1] == holderMarker {
		os.Exit(hold(os.Args[2]))
	}

	os.Exit(m.Run())
}

func hold(name string) int {
	release, err := lock.Socket{Name: name}.Lock(context.Background())
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)

		return 1
	}

	_, _ = fmt.Fprintln(os.Stdout, "locked")
	_, _ = io.Copy(io.Discard, os.Stdin)

	_ = release()

	return 0
}

// socketName returns a name of the abstract namespace that only this test
// uses.
func socketName(t *testing.T) string {
	t.Helper()

	return fmt.Sprintf("emergup-test/%d/%s", os.Getpid(), t.Name())
}

func TestSocketIsHeldByOneLockAtATime(t *testing.T) {
	t.Parallel()

	socket := lock.Socket{Name: socketName(t)}

	release, err := socket.Lock(t.Context())
	require.NoError(t, err)

	_, err = socket.Lock(t.Context())
	held, isHeld := errors.AsType[*lock.HeldError](err)
	require.True(t, isHeld, "a second lock fails: %v", err)

	current, err := user.Current()
	require.NoError(t, err)
	assert.Equal(t, &lock.HeldError{PID: os.Getpid(), User: current.Username}, held, "it says who holds the lock")

	require.NoError(t, release())

	release, err = socket.Lock(t.Context())
	require.NoError(t, err, "released, the lock can be taken again")
	require.NoError(t, release())
}

// TestSocketIsFreedWhenItsProcessEnds starts a process, so it does not run
// alongside the other tests: between its fork and its exec the new process
// holds a copy of every descriptor, a lock released meanwhile among them,
// whose name stays taken until the exec.
//
//nolint:paralleltest // See above.
func TestSocketIsFreedWhenItsProcessEnds(t *testing.T) {
	name := socketName(t)

	//nolint:gosec // The test binary itself, as the process that holds the lock.
	holder := exec.CommandContext(t.Context(), os.Args[0], holderMarker, name)
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

	_, err = lock.Socket{Name: name}.Lock(t.Context())
	held, isHeld := errors.AsType[*lock.HeldError](err)
	require.True(t, isHeld, "another process holds the lock: %v", err)
	assert.Equal(t, holder.Process.Pid, held.PID)

	require.NoError(t, holder.Process.Kill(), "the holder ends without releasing the lock")

	_ = holder.Wait()

	release, err := lock.Socket{Name: name}.Lock(t.Context())
	require.NoError(t, err, "the kernel freed the lock")
	require.NoError(t, release())
}
