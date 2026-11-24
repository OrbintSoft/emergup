// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import "golang.org/x/sys/unix"

// flushInput discards what the terminal fd has received and not yet passed
// on to a reader.
func flushInput(fd int) error {
	return unix.IoctlSetInt(fd, unix.TCFLSH, unix.TCIFLUSH) //nolint:wrapcheck // The terminalKeyboard says what failed.
}
