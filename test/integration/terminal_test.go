// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package integration_test

import (
	"bytes"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// The size of the terminals the tests open.
const (
	terminalRows    = 24
	terminalColumns = 80
)

// openTerminal opens a pseudo-terminal and returns its two sides: the
// keyboard and screen the test uses, and the terminal a program runs on.
func openTerminal(t *testing.T) (*os.File, *os.File) {
	t.Helper()

	keyboard, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	require.NoError(t, err)

	raw, err := keyboard.SyscallConn()
	require.NoError(t, err)

	var (
		number   uint32
		setupErr error
	)

	require.NoError(t, raw.Control(func(descriptor uintptr) {
		// The terminal side of a new pseudo-terminal opens only once unlocked.
		setupErr = unix.IoctlSetPointerInt(int(descriptor), unix.TIOCSPTLCK, 0)
		if setupErr == nil {
			setupErr = unix.IoctlSetWinsize(int(descriptor), unix.TIOCSWINSZ,
				&unix.Winsize{Row: terminalRows, Col: terminalColumns, Xpixel: 0, Ypixel: 0})
		}

		if setupErr == nil {
			number, setupErr = unix.IoctlGetUint32(int(descriptor), unix.TIOCGPTN)
		}
	}))
	require.NoError(t, setupErr)

	terminal, err := os.OpenFile("/dev/pts/"+strconv.FormatUint(uint64(number), 10), os.O_RDWR|unix.O_NOCTTY, 0)
	require.NoError(t, err)

	return keyboard, terminal
}

// emergupKeyboard starts emergup with args as tester on a terminal of its
// own, whose keys the test presses with send, and with NO_COLOR set: the
// colours are another test's. A terminal has a single output, each line
// ending with "\r\n".
func (c *container) emergupKeyboard(args ...string) *background {
	c.t.Helper()

	keyboard, terminal := openTerminal(c.t)

	//nolint:gosec // The tests choose what runs.
	cmd := exec.CommandContext(c.t.Context(), c.docker,
		c.execArgs(tester, []string{interactive, "--tty", "--env", "NO_COLOR=1"},
			append([]string{program}, args...))...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = terminal, terminal, terminal

	output := &syncBuffer{mutex: sync.Mutex{}, buffer: bytes.Buffer{}}
	copied := make(chan struct{})

	run := &background{
		t: c.t, cmd: cmd, input: keyboard, endInput: nil, output: output, copied: copied,
		ended: nil, finished: false,
	}
	run.begin(func() {
		<-copied

		_ = keyboard.Close()
	})

	// The terminal is the container command's now: once that ends, reading
	// the keyboard side ends too.
	require.NoError(c.t, terminal.Close())

	go func() {
		defer close(copied)

		data := make([]byte, 4096)

		for {
			count, err := keyboard.Read(data)
			_, _ = output.Write(data[:count])

			if err != nil {
				return
			}
		}
	}()

	return run
}
