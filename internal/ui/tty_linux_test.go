// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// openTerminal opens a pseudo-terminal and returns its two ends: what is
// written to the controller reaches the terminal as keys typed on it. The
// test is skipped when the system has no pseudo-terminals.
func openTerminal(t *testing.T) (*os.File, *os.File) {
	t.Helper()

	controller, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}

	t.Cleanup(func() { _ = controller.Close() })

	fd := int(controller.Fd())
	require.NoError(t, unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0))

	number, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	require.NoError(t, err)

	terminal, err := os.OpenFile("/dev/pts/"+strconv.Itoa(number), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}

	t.Cleanup(func() { _ = terminal.Close() })

	return controller, terminal
}

// settings returns the settings of terminal, or nil when they cannot be
// read.
func settings(terminal *os.File) *unix.Termios {
	termios, err := unix.IoctlGetTermios(int(terminal.Fd()), unix.TCGETS)
	if err != nil {
		return nil
	}

	return termios
}

func TestNewKeyboardReadsKeysFromATerminal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		typed string
		want  rune
	}{
		{"a letter, without Enter", "s", 's'},
		{"Enter", "\r", KeyEnter},
		{"Ctrl-C, which sends no signal", "\x03", KeyInterrupt},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			controller, terminal := openTerminal(t)
			before := settings(terminal)
			require.NotNil(t, before)

			assert.True(t, IsTerminal(terminal))

			keyboard := NewKeyboard(terminal)
			require.IsType(t, &terminalKeyboard{}, keyboard)

			_, err := controller.WriteString("x")
			require.NoError(t, err, "a key typed before the question")

			type result struct {
				key rune
				err error
			}

			results := make(chan result, 1)

			go func() {
				key, err := keyboard.ReadKey(t.Context())
				results <- result{key: key, err: err}
			}()

			// The key is typed once the terminal is in raw mode, unless a key
			// was already read: the one typed before, which is a failure.
			require.Eventually(t, func() bool {
				now := settings(terminal)

				return len(results) > 0 || now != nil && now.Lflag&unix.ICANON == 0
			}, 5*time.Second, time.Millisecond, "the terminal is put in raw mode")

			_, err = controller.WriteString(tt.typed)
			require.NoError(t, err)

			select {
			case got := <-results:
				require.NoError(t, got.err)
				assert.Equal(t, tt.want, got.key, "the key typed before the question is discarded")
			case <-time.After(5 * time.Second):
				t.Fatal("no key was read")
			}

			assert.Equal(t, before, settings(terminal), "the terminal is put back as it was")
		})
	}
}
