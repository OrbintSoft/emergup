// SPDX-License-Identifier: GPL-3.0-or-later

// Package elevation runs the commands that need root through sudo, doas or
// run0, or directly when emergup is root already.
package elevation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"

	"github.com/OrbintSoft/emergup/internal/config"
	"github.com/OrbintSoft/emergup/internal/runner"
)

// Errors of Resolve.
var (
	errNoTool       = errors.New("no elevation tool found: install sudo, doas or run0, or run emergup as root")
	errNotInstalled = errors.New("is not installed")
)

// rootUID is the user id of root.
const rootUID = 0

// System is what elevation looks at on the running system.
type System interface {
	// LookPath finds a program on PATH, as exec.LookPath does.
	LookPath(file string) (string, error)
	// Geteuid returns the effective user id, as os.Geteuid does.
	Geteuid() int
}

// OS is the System emergup runs on.
type OS struct{}

// LookPath implements System.
func (OS) LookPath(file string) (string, error) {
	path, err := exec.LookPath(file)
	if err != nil {
		return "", fmt.Errorf("looking for %s: %w", file, err)
	}

	return path, nil
}

// Geteuid implements System.
func (OS) Geteuid() int {
	return os.Geteuid()
}

// IsRoot reports whether emergup runs as root on system.
func IsRoot(system System) bool {
	return system.Geteuid() == rootUID
}

// Resolve returns the command line that runs a command as root with method
// on system: empty when emergup is root already. With ElevationAuto it is
// the first of sudo, doas and run0 that is installed; a method set by name
// must be installed.
func Resolve(method config.ElevationMethod, system System) ([]string, error) {
	if IsRoot(system) {
		return nil, nil
	}

	candidates := []string{method.String()}
	if method == config.ElevationAuto {
		candidates = []string{
			config.ElevationSudo.String(),
			config.ElevationDoas.String(),
			config.ElevationRun0.String(),
		}
	}

	for _, program := range candidates {
		if _, err := system.LookPath(program); err == nil {
			return []string{program}, nil
		}
	}

	if method == config.ElevationAuto {
		return nil, errNoTool
	}

	return nil, fmt.Errorf("elevation method %s %w", method, errNotInstalled)
}

// Runner runs the commands marked Root through Prefix, and every command
// through Next. With an empty Prefix, as for root, it runs every command as
// it is.
type Runner struct {
	Next   runner.Runner
	Prefix []string
}

// Run implements runner.Runner.
func (r Runner) Run(ctx context.Context, cmd runner.Command) (runner.Result, error) {
	if cmd.Root && len(r.Prefix) > 0 {
		cmd.Args = append(slices.Clone(r.Prefix), cmd.Args...)
	}

	return r.Next.Run(ctx, cmd) //nolint:wrapcheck // a transparent wrapper: the error is the command's own
}
