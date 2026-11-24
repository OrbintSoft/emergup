// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !linux

package ui

// flushInput would discard what the terminal fd has received and not yet
// passed on to a reader; on this system the input is kept.
func flushInput(int) error {
	return nil
}
