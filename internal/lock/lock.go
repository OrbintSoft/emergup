// SPDX-License-Identifier: GPL-3.0-or-later

// Package lock keeps two runs of one user from overlapping.
package lock

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// errUnsafeLock is the error of a lock file emergup does not use.
var errUnsafeLock = errors.New("the lock is not safe")

// lockMode lets the user alone open the lock.
const lockMode = 0o600

// maxPIDBytes is more than the text of any process id.
const maxPIDBytes = 32

// Locker takes the lock of a run.
type Locker interface {
	// Lock takes the lock and returns how to release it. While another
	// process holds it, Lock fails with a *HeldError.
	Lock(ctx context.Context) (release func() error, err error)
}

// HeldError says that another process holds the lock, and which.
type HeldError struct {
	// PID is the process that holds the lock, 0 when it is not known.
	PID int
}

func (e *HeldError) Error() string {
	if e.PID == 0 {
		return "another emergup run of this user is active"
	}

	return fmt.Sprintf("another emergup run of this user is active: process %d", e.PID)
}

// Acquire takes the lock file at path, made when it is missing, and returns
// how to release it. The kernel holds the lock on the open file and frees
// it when the process ends, however it ends; the file itself is never
// removed, since a process that removed it while another waited would let
// two processes lock two different files. path must not be a symlink, and
// the file must be a regular file of the user's; only the user can open it.
// The lock holds the process id of the run, for the error of a second one.
func Acquire(path string) (func() error, error) {
	//nolint:gosec // The lock file in the user's own state directory, not followed when it is a symlink.
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW, lockMode)
	if err != nil {
		return nil, fmt.Errorf("opening the lock: %w", err)
	}

	if err := prepare(file); err != nil {
		_ = file.Close()

		return nil, err
	}

	if err := flock(file); err != nil {
		holder := readPID(file)
		_ = file.Close()

		if errors.Is(err, unix.EWOULDBLOCK) {
			return nil, &HeldError{PID: holder}
		}

		return nil, fmt.Errorf("taking the lock: %w", err)
	}

	writePID(file, strconv.Itoa(os.Getpid())+"\n")

	return func() error {
		writePID(file, "")

		if err := file.Close(); err != nil {
			return fmt.Errorf("releasing the lock: %w", err)
		}

		return nil
	}, nil
}

// prepare checks the open lock file and lets the user alone open it. Both
// act on the open file, not on its name, which another process could
// change in between.
func prepare(file *os.File) error {
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("checking the lock: %w", err)
	}

	if err := checkLockInfo(info, os.Geteuid()); err != nil {
		return fmt.Errorf("%w: %s %w", errUnsafeLock, file.Name(), err)
	}

	if info.Mode().Perm() != lockMode {
		if err := file.Chmod(lockMode); err != nil {
			return fmt.Errorf("checking the lock: %w", err)
		}
	}

	return nil
}

// Reasons for refusing a lock file.
var (
	errNotRegular  = errors.New("is not a regular file")
	errAnotherUser = errors.New("belongs to another user")
)

// checkLockInfo refuses a lock file that is not a regular file of the user
// euid.
func checkLockInfo(info fs.FileInfo, euid int) error {
	if !info.Mode().IsRegular() {
		return errNotRegular
	}

	stat, isStat := info.Sys().(*syscall.Stat_t)
	if !isStat || int(stat.Uid) != euid {
		return errAnotherUser
	}

	return nil
}

// flock takes the lock of file without waiting for it.
func flock(file *os.File) error {
	raw, err := file.SyscallConn()
	if err != nil {
		return fmt.Errorf("taking the lock: %w", err)
	}

	var lockErr error

	err = raw.Control(func(fd uintptr) {
		lockErr = unix.Flock(int(fd), unix.LOCK_EX|unix.LOCK_NB)
	})
	if err != nil {
		return fmt.Errorf("taking the lock: %w", err)
	}

	return lockErr //nolint:wrapcheck // Acquire tells EWOULDBLOCK from the others, then says what failed.
}

// readPID returns the process id the lock file holds, 0 when it holds
// none.
func readPID(file *os.File) int {
	data := make([]byte, maxPIDBytes)

	count, err := file.ReadAt(data, 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return 0
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data[:count])))
	if err != nil {
		return 0
	}

	return pid
}

// writePID makes text the content of the lock file. The text is only for
// the error of a second run, so a failed write is not an error.
func writePID(file *os.File, text string) {
	_ = file.Truncate(0)
	_, _ = file.WriteAt([]byte(text), 0)
}
