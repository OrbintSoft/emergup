// SPDX-License-Identifier: GPL-3.0-or-later

package report

import (
	"fmt"
	"io"
	"os"
)

// The modes of a report's directory and files: only the invoking user can
// read them, as the run directory they are in.
const (
	dirMode  = 0o700
	fileMode = 0o600
)

// Dir is the directory of a report on the file system: its files are
// created inside it and nowhere else.
type Dir struct {
	root *os.Root
}

// OpenDir makes the directory path, which must not exist yet, and returns
// it.
func OpenDir(path string) (*Dir, error) {
	if err := os.Mkdir(path, dirMode); err != nil {
		return nil, fmt.Errorf("could not make the failure report's directory: %w", err)
	}

	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, fmt.Errorf("could not open the failure report's directory: %w", err)
	}

	return &Dir{root: root}, nil
}

// Create implements Files.
func (d *Dir) Create(name string) (io.WriteCloser, error) {
	file, err := d.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fileMode)
	if err != nil {
		return nil, fmt.Errorf("could not create %s: %w", name, err)
	}

	return file, nil
}

// Close closes the directory; the files it created are closed already.
func (d *Dir) Close() error {
	if err := d.root.Close(); err != nil {
		return fmt.Errorf("could not close the failure report's directory: %w", err)
	}

	return nil
}
