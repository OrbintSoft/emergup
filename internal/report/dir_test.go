// SPDX-License-Identifier: GPL-3.0-or-later

package report_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/emergup/internal/report"
)

func TestOpenDirMakesADirectoryOnlyTheUserCanRead(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "failure")

	dir, err := report.OpenDir(path)
	require.NoError(t, err)

	t.Cleanup(func() { assert.NoError(t, dir.Close()) })

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())

	file, err := dir.Create(summary)
	require.NoError(t, err)

	_, err = io.WriteString(file, "text\n")
	require.NoError(t, err)
	require.NoError(t, file.Close())

	data, err := os.ReadFile(filepath.Join(path, summary)) //nolint:gosec // The test's own temporary directory.
	require.NoError(t, err)
	assert.Equal(t, "text\n", string(data))

	info, err = os.Stat(filepath.Join(path, summary))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestADirCreatesOnlyNewFilesInsideIt(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "failure")

	dir, err := report.OpenDir(path)
	require.NoError(t, err)

	t.Cleanup(func() { assert.NoError(t, dir.Close()) })

	file, err := dir.Create(summary)
	require.NoError(t, err)
	require.NoError(t, file.Close())

	_, err = dir.Create(summary)
	require.Error(t, err, "a file is not written twice")

	_, err = dir.Create("../outside.txt")
	require.Error(t, err)
	assert.NoFileExists(t, filepath.Join(filepath.Dir(path), "outside.txt"))
}

func TestOpenDirRefusesADirectoryThatExists(t *testing.T) {
	t.Parallel()

	path := t.TempDir()

	_, err := report.OpenDir(path)
	require.Error(t, err, "a report never mixes with what is there")
}
