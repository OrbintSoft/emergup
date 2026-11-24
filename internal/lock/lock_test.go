// SPDX-License-Identifier: GPL-3.0-or-later

package lock_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/OrbintSoft/emergup/internal/lock"
)

func TestHeldErrorSaysWhoHoldsTheLock(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "another emergup run is active: process 1234 of user mario",
		(&lock.HeldError{PID: 1234, User: "mario"}).Error())
	assert.Equal(t, "another emergup run is active", (&lock.HeldError{PID: 0, User: ""}).Error(),
		"when the holder is not known")
}
