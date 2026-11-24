// SPDX-License-Identifier: GPL-3.0-or-later

package lock

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/user"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

// Lock implements Locker. The socket is not passed on to the commands the
// run starts, so a process they leave behind does not keep the lock.
func (s Socket) Lock(ctx context.Context) (func() error, error) {
	var config net.ListenConfig

	listener, err := config.Listen(ctx, "unix", s.address())
	if errors.Is(err, syscall.EADDRINUSE) {
		return nil, s.holder(ctx)
	}

	if err != nil {
		return nil, fmt.Errorf("taking the lock of the run: %w", err)
	}

	// The connections of the runs that ask who holds the lock are closed at
	// once: connecting is enough to know.
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}

			_ = conn.Close()
		}
	}()

	return func() error {
		if err := listener.Close(); err != nil {
			return fmt.Errorf("releasing the lock of the run: %w", err)
		}

		return nil
	}, nil
}

// address is the socket's address: a leading "@" names the abstract
// namespace.
func (s Socket) address() string {
	return "@" + s.Name
}

// holder returns the *HeldError of the process that holds the lock, which
// the credentials of the socket's other end tell.
func (s Socket) holder(ctx context.Context) error {
	unknown := &HeldError{PID: 0, User: ""}

	var dialer net.Dialer

	conn, err := dialer.DialContext(ctx, "unix", s.address())
	if err != nil {
		return unknown
	}

	defer func() { _ = conn.Close() }()

	unixConn, isUnix := conn.(*net.UnixConn)
	if !isUnix {
		return unknown
	}

	raw, err := unixConn.SyscallConn()
	if err != nil {
		return unknown
	}

	var (
		cred    *unix.Ucred
		credErr error
	)

	err = raw.Control(func(fd uintptr) {
		cred, credErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	})
	if err != nil || credErr != nil {
		return unknown
	}

	return &HeldError{PID: int(cred.Pid), User: userName(cred.Uid)}
}

// userName returns the name of the user uid, or uid itself when it has
// none.
func userName(uid uint32) string {
	id := strconv.FormatUint(uint64(uid), 10)

	found, err := user.LookupId(id)
	if err != nil {
		return id
	}

	return found.Username
}
