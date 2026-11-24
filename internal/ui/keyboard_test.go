// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errBroken is the error of a terminal that does not work.
var errBroken = errors.New("input/output error")

// What a fakeTTY is asked to do.
const (
	callFlush   = "flush"
	callRaw     = "raw"
	callRead    = "read"
	callRestore = "restore"
)

func TestDecodeKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
		want rune
		err  error
	}{
		{"a letter", "s", 's', nil},
		{"a capital letter", "S", 'S', nil},
		{"a question mark", "?", '?', nil},
		{"Enter", "\r", KeyEnter, nil},
		{"Enter as a newline", "\n", KeyEnter, nil},
		{"Ctrl-C", "\x03", KeyInterrupt, nil},
		{"Ctrl-D", "\x04", 0, io.EOF},
		{"Escape", "\x1b", unknownKey, nil},
		{"an arrow", "\x1b[A", unknownKey, nil},
		{"a character of two bytes", "é", 'é', nil},
		{"not UTF-8", "\xff", unknownKey, nil},
		{"two keys at once", "sa", 's', nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			key, err := decodeKey([]byte(tt.data))
			assert.Equal(t, tt.want, key)
			assert.Equal(t, tt.err, err)
		})
	}
}

// readAll reads keys from keyboard until it returns an error, and returns
// the keys and the error.
func readAll(t *testing.T, keyboard Keyboard) ([]rune, error) {
	t.Helper()

	var keys []rune

	for range 10 {
		key, err := keyboard.ReadKey(t.Context())
		if err != nil {
			return keys, err //nolint:wrapcheck // The tests look at the error as ReadKey returns it.
		}

		keys = append(keys, key)
	}

	t.Fatal("the keyboard did not stop")

	return nil, nil
}

func TestLineKeyboard(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []rune
	}{
		{"a letter", "s\n", []rune{'s'}},
		{"an empty line", "\n", []rune{KeyEnter}},
		{"spaces around", "  S \r\n", []rune{'S'}},
		{"a word", "skip\n", []rune{unknownKey}},
		{"several lines", "x\n\n?\n", []rune{'x', KeyEnter, '?'}},
		{"no newline at the end", "s", []rune{'s'}},
		{"a character of two bytes", "é\n", []rune{'é'}},
		{"nothing", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			keys, err := readAll(t, &lineKeyboard{reader: bufio.NewReader(strings.NewReader(tt.input))})
			require.ErrorIs(t, err, io.EOF)
			assert.Equal(t, tt.want, keys)
		})
	}
}

func TestLineKeyboardReportsAReadError(t *testing.T) {
	t.Parallel()

	keyboard := &lineKeyboard{reader: bufio.NewReader(iotest.ErrReader(syscall.EIO))}
	_, err := keyboard.ReadKey(t.Context())
	require.ErrorIs(t, err, syscall.EIO)
	assert.NotErrorIs(t, err, io.EOF)
}

func TestLineKeyboardStopsWaitingWhenTheContextIsCancelled(t *testing.T) {
	t.Parallel()

	reader, writer := io.Pipe()

	t.Cleanup(func() { _ = writer.Close() })

	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(10*time.Millisecond, cancel)

	keyboard := &lineKeyboard{reader: bufio.NewReader(reader)}
	_, err := keyboard.ReadKey(ctx)
	require.ErrorIs(t, err, context.Canceled)
}

func TestNewKeyboardReadsLinesWhenTheInputIsNotATerminal(t *testing.T) {
	t.Parallel()

	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close() })

	assert.False(t, IsTerminal(reader))

	keyboard := NewKeyboard(reader)
	require.IsType(t, &lineKeyboard{}, keyboard)

	_, err = writer.WriteString("s\n")
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	keys, err := readAll(t, keyboard)
	require.ErrorIs(t, err, io.EOF)
	assert.Equal(t, []rune{'s'}, keys)
}

// fakeTTY is a tty that records what it is asked to do.
type fakeTTY struct {
	mutex sync.Mutex
	calls []string
	// data is what Read returns.
	data                                  string
	flushErr, rawErr, readErr, restoreErr error
	// onRead, when set, is called by Read, which then waits until the test
	// ends.
	onRead func()
	ended  chan struct{}
}

func (f *fakeTTY) Flush() error {
	f.record(callFlush)

	return f.flushErr
}

func (f *fakeTTY) MakeRaw() (func() error, error) {
	f.record(callRaw)

	if f.rawErr != nil {
		return nil, f.rawErr
	}

	return func() error {
		f.record(callRestore)

		return f.restoreErr
	}, nil
}

func (f *fakeTTY) Read(data []byte) (int, error) {
	f.record(callRead)

	if f.onRead != nil {
		f.onRead()
		<-f.ended
	}

	return copy(data, f.data), f.readErr
}

func (f *fakeTTY) record(call string) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	f.calls = append(f.calls, call)
}

func (f *fakeTTY) recorded() []string {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	return f.calls
}

func TestTerminalKeyboard(t *testing.T) {
	t.Parallel()

	every := []string{callFlush, callRaw, callRead, callRestore}

	tests := []struct {
		name  string
		tty   *fakeTTY
		want  rune
		err   error
		calls []string
	}{
		{"a key", &fakeTTY{data: "s"}, 's', nil, every},
		{"Ctrl-D", &fakeTTY{data: "\x04"}, 0, io.EOF, every},
		{"a key and an error", &fakeTTY{data: "s", readErr: errBroken}, 's', nil, every},
		{"a read error", &fakeTTY{readErr: errBroken}, 0, errBroken, every},
		{"a flush error", &fakeTTY{flushErr: errBroken}, 0, errBroken, []string{callFlush}},
		{"a raw mode error", &fakeTTY{rawErr: errBroken}, 0, errBroken, []string{callFlush, callRaw}},
		{"a restore error", &fakeTTY{data: "s", restoreErr: errBroken}, 0, errBroken, every},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			key, err := (&terminalKeyboard{tty: tt.tty}).ReadKey(t.Context())
			if tt.err == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.err)
			}

			assert.Equal(t, tt.want, key)
			assert.Equal(t, tt.calls, tt.tty.recorded())
		})
	}
}

func TestTerminalKeyboardRestoresTheTerminalWhenTheContextIsCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	tty := &fakeTTY{onRead: cancel, ended: make(chan struct{})}

	t.Cleanup(func() { close(tty.ended) })

	_, err := (&terminalKeyboard{tty: tty}).ReadKey(ctx)
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, []string{callFlush, callRaw, callRead, callRestore}, tty.recorded())
}
