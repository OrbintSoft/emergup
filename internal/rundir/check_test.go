// SPDX-License-Identifier: GPL-3.0-or-later

package rundir

import (
	"io/fs"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// dirInfo is a fs.FileInfo of a directory of the user uid.
type dirInfo struct {
	mode fs.FileMode
	uid  uint32
}

func (d dirInfo) Name() string       { return "dir" }
func (d dirInfo) Size() int64        { return 0 }
func (d dirInfo) Mode() fs.FileMode  { return d.mode }
func (d dirInfo) ModTime() time.Time { return time.Time{} }
func (d dirInfo) IsDir() bool        { return d.mode.IsDir() }
func (d dirInfo) Sys() any           { return &syscall.Stat_t{Uid: d.uid} }

func TestCheckDir(t *testing.T) {
	t.Parallel()

	const user = 1000

	require.NoError(t, checkDir("/home/mario", dirInfo{mode: fs.ModeDir | 0o700, uid: user}, user))
	require.NoError(t, checkDir("/home", dirInfo{mode: fs.ModeDir | 0o755, uid: 0}, user), "root's")
	require.ErrorContains(t, checkDir("/home/luigi", dirInfo{mode: fs.ModeDir | 0o700, uid: 1001}, user),
		"the state directory is not safe: /home/luigi belongs to another user")
	require.ErrorContains(t, checkDir("/home/mario", dirInfo{mode: fs.ModeDir | 0o720, uid: user}, user),
		"the state directory is not safe: /home/mario can be changed by other users")
	require.ErrorContains(t, checkDir("/home/mario/file", dirInfo{mode: 0o600, uid: user}, user),
		"the state directory is not safe: /home/mario/file is not a directory")
}
