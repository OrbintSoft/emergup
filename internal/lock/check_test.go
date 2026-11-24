// SPDX-License-Identifier: GPL-3.0-or-later

package lock

import (
	"io/fs"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fileInfo is a fs.FileInfo of a file of the user uid.
type fileInfo struct {
	mode fs.FileMode
	uid  uint32
}

func (f fileInfo) Name() string       { return "run.lock" }
func (f fileInfo) Size() int64        { return 0 }
func (f fileInfo) Mode() fs.FileMode  { return f.mode }
func (f fileInfo) ModTime() time.Time { return time.Time{} }
func (f fileInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fileInfo) Sys() any           { return &syscall.Stat_t{Uid: f.uid} }

func TestCheckLockInfo(t *testing.T) {
	t.Parallel()

	const user = 1000

	require.NoError(t, checkLockInfo(fileInfo{mode: 0o600, uid: user}, user))
	require.ErrorContains(t, checkLockInfo(fileInfo{mode: 0o600, uid: 1001}, user), "belongs to another user")
	require.ErrorContains(t, checkLockInfo(fileInfo{mode: 0o600, uid: 0}, user), "belongs to another user",
		"not even root's: the user could not take it")
	require.ErrorContains(t, checkLockInfo(fileInfo{mode: fs.ModeNamedPipe | 0o600, uid: user}, user),
		"is not a regular file")
	require.ErrorContains(t, checkLockInfo(fileInfo{mode: fs.ModeDevice | 0o600, uid: user}, user),
		"is not a regular file")
}
