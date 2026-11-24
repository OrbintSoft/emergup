// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

// The keys a Keyboard returns for Enter and Ctrl-C.
const (
	KeyEnter     = '\n'
	KeyInterrupt = '\x03'
)

// unknownKey stands for a key that is not a character, such as an arrow,
// and for a line of more than one character.
const unknownKey = utf8.RuneError

// The bytes a terminal in raw mode sends for Enter, Ctrl-D and the keys
// that are not characters.
const (
	carriageReturn = '\r'
	endOfInput     = '\x04'
	escape         = '\x1b'
)

// maxKeyBytes is the most a key sends: a whole escape sequence, such as an
// arrow's, arrives in one read, and is one key.
const maxKeyBytes = 64

// Keyboard reads the keys that answer the questions.
type Keyboard interface {
	// ReadKey returns the next key: a character, KeyEnter or KeyInterrupt.
	// At the end of the input it returns io.EOF, and when ctx is cancelled
	// an error that wraps ctx's; the Keyboard is not used again after that.
	ReadKey(ctx context.Context) (rune, error)
}

// NewKeyboard returns the Keyboard that reads from in. On a terminal it
// reads a key at a time, without Enter, and discards first what was typed
// before it was asked; otherwise it reads a line for each key.
//
//nolint:ireturn // Which Keyboard depends on what in is.
func NewKeyboard(in *os.File) Keyboard {
	if terminal, isTerminal := newFileTTY(in); isTerminal {
		return &terminalKeyboard{tty: terminal}
	}

	return &lineKeyboard{reader: bufio.NewReader(in)}
}

// tty is the terminal a terminalKeyboard reads from.
type tty interface {
	// Flush discards the input not read yet.
	Flush() error
	// MakeRaw makes the terminal pass each key on as it is pressed, and
	// returns how to put it back as it was.
	MakeRaw() (restore func() error, err error)
	Read(data []byte) (int, error)
}

// terminalKeyboard reads a key at a time from a terminal.
type terminalKeyboard struct {
	tty tty
}

// ReadKey implements Keyboard. The keys typed before it was called are
// discarded: they were meant for something else, such as a command that
// had no input. The terminal is in raw mode only while the key is read.
func (k *terminalKeyboard) ReadKey(ctx context.Context) (rune, error) {
	if err := k.tty.Flush(); err != nil {
		return 0, fmt.Errorf("discarding the keys typed before the question: %w", err)
	}

	restore, err := k.tty.MakeRaw()
	if err != nil {
		return 0, fmt.Errorf("putting the terminal in raw mode: %w", err)
	}

	key, err := readContext(ctx, k.readKey)

	if restoreErr := restore(); restoreErr != nil {
		return 0, errors.Join(err, fmt.Errorf("restoring the terminal: %w", restoreErr))
	}

	return key, err
}

// readKey reads one key from the terminal in raw mode.
func (k *terminalKeyboard) readKey() (rune, error) {
	data := make([]byte, maxKeyBytes)

	count, err := k.tty.Read(data)
	if count == 0 {
		if err == nil {
			err = io.EOF
		}

		return 0, err
	}

	return decodeKey(data[:count])
}

// decodeKey returns the key that a terminal in raw mode sent as data: the
// first one, when data holds more.
func decodeKey(data []byte) (rune, error) {
	switch data[0] {
	case carriageReturn, KeyEnter:
		return KeyEnter, nil
	case endOfInput:
		return 0, io.EOF
	case escape:
		return unknownKey, nil
	}

	key, _ := utf8.DecodeRune(data)

	return key, nil
}

// lineKeyboard reads a line for each key: an empty line is Enter, a line of
// one character that key.
type lineKeyboard struct {
	reader *bufio.Reader
}

// ReadKey implements Keyboard.
func (k *lineKeyboard) ReadKey(ctx context.Context) (rune, error) {
	return readContext(ctx, k.readLine)
}

// readLine reads one line and returns its key.
func (k *lineKeyboard) readLine() (rune, error) {
	line, err := k.reader.ReadString('\n')
	if err != nil && (line == "" || !errors.Is(err, io.EOF)) {
		return 0, err //nolint:wrapcheck // Ask says what failed.
	}

	text := strings.TrimSpace(line)

	switch utf8.RuneCountInString(text) {
	case 0:
		return KeyEnter, nil
	case 1:
		key, _ := utf8.DecodeRuneInString(text)

		return key, nil
	}

	return unknownKey, nil
}

// readContext returns what read returns, or an error that wraps ctx's as
// soon as ctx is cancelled: read goes on in the background, and its result
// is dropped.
func readContext(ctx context.Context, read func() (rune, error)) (rune, error) {
	if ctx.Err() != nil {
		return 0, cancelled(ctx)
	}

	type result struct {
		key rune
		err error
	}

	results := make(chan result, 1)

	go func() {
		key, err := read()
		results <- result{key: key, err: err}
	}()

	select {
	case <-ctx.Done():
		return 0, cancelled(ctx)
	case got := <-results:
		return got.key, got.err
	}
}

// cancelled returns the error of a read stopped because ctx was cancelled.
func cancelled(ctx context.Context) error {
	return fmt.Errorf("waiting for a key: %w", ctx.Err())
}
