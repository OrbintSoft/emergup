// SPDX-License-Identifier: GPL-3.0-or-later

// Package lock keeps two emergup runs from overlapping on one system.
package lock

import (
	"context"
	"fmt"
)

// Locker takes the lock of a run.
type Locker interface {
	// Lock takes the lock and returns how to release it. While another
	// process holds it, Lock fails with a *HeldError.
	Lock(ctx context.Context) (release func() error, err error)
}

// Socket is the lock of the system: the abstract Unix socket Name, which
// only one process can hold at a time and which the kernel frees when that
// process ends.
type Socket struct {
	// Name is the socket's name in the abstract namespace, without the
	// leading "@".
	Name string
}

// HeldError says that another process holds the lock, and which.
type HeldError struct {
	// PID is the process that holds the lock, 0 when it is not known; User
	// is its user's name, or its user id when the name is not known.
	PID  int
	User string
}

func (e *HeldError) Error() string {
	if e.PID == 0 {
		return "another emergup run is active"
	}

	return fmt.Sprintf("another emergup run is active: process %d of user %s", e.PID, e.User)
}
