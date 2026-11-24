// SPDX-License-Identifier: GPL-3.0-or-later

package steps

import "fmt"

// portageq is Portage's query tool, which the steps ask about the system.
const portageq = "portageq"

// exitError is a command that ended with a status other than 0.
type exitError struct {
	command string
	code    int
}

func (e exitError) Error() string {
	return fmt.Sprintf("%s exited with status %d", e.command, e.code)
}
