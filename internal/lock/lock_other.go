// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !linux

package lock

import "context"

// Lock implements Locker. This system has no abstract sockets, so there is
// no lock: Lock always succeeds.
func (Socket) Lock(context.Context) (func() error, error) {
	return func() error { return nil }, nil
}
