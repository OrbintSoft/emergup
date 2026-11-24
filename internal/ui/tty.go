// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"os"

	"golang.org/x/term"
)

// fileTTY is a terminal opened as a file, such as the standard input.
type fileTTY struct {
	file *os.File
	fd   int
}

// newFileTTY returns file as a tty, and whether it is a terminal.
func newFileTTY(file *os.File) (fileTTY, bool) {
	terminal := fileTTY{file: file, fd: int(file.Fd())}

	return terminal, term.IsTerminal(terminal.fd)
}

// Flush implements tty.
func (t fileTTY) Flush() error { return flushInput(t.fd) }

// MakeRaw implements tty.
func (t fileTTY) MakeRaw() (func() error, error) {
	state, err := term.MakeRaw(t.fd)
	if err != nil {
		return nil, err //nolint:wrapcheck // The terminalKeyboard says what failed.
	}

	return func() error { return term.Restore(t.fd, state) }, nil
}

// Read implements tty.
func (t fileTTY) Read(data []byte) (int, error) {
	return t.file.Read(data) //nolint:wrapcheck // Ask says what failed.
}
