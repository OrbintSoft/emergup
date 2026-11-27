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
	"strings"

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
	// LookupEnv looks a variable of emergup's environment up, as
	// os.LookupEnv does.
	LookupEnv(name string) (string, bool)
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

// LookupEnv implements System.
func (OS) LookupEnv(name string) (string, bool) {
	return os.LookupEnv(name)
}

// IsRoot reports whether emergup runs as root on system.
func IsRoot(system System) bool {
	return system.Geteuid() == rootUID
}

// Resolve returns the program that runs a command as root with method on
// system: "" when emergup is root already. With ElevationAuto it is the
// first of sudo, doas and run0 that is installed; a method set by name must
// be installed.
func Resolve(method config.ElevationMethod, system System) (string, error) {
	if IsRoot(system) {
		return "", nil
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
			return program, nil
		}
	}

	if method == config.ElevationAuto {
		return "", errNoTool
	}

	return "", fmt.Errorf("elevation method %s %w", method, errNotInstalled)
}

// Runner runs the commands marked Root through Program, and every command
// through Next.
type Runner struct {
	Next runner.Runner
	// Program is sudo, doas or run0, as Resolve returns it; "" runs every
	// command as it is, as emergup does when it is root already.
	Program string
	// Keep names the variables of emergup's environment that a command run
	// as root keeps, besides those of its own Env.
	Keep []string
	// LookupEnv looks a variable of emergup's environment up, as
	// System.LookupEnv does; run0 needs it.
	LookupEnv func(name string) (string, bool)
}

// Run implements runner.Runner.
func (r Runner) Run(ctx context.Context, cmd runner.Command) (runner.Result, error) {
	if cmd.Root && r.Program != "" {
		cmd.Args = slices.Concat([]string{r.Program}, r.keeping(cmd.Env), cmd.Args)
	}

	return r.Next.Run(ctx, cmd) //nolint:wrapcheck // a transparent wrapper: the error is the command's own
}

// keeping returns the options of Program with which a command run as root,
// with env on top of emergup's environment, keeps the variables of Keep and
// those of env. sudo keeps them by name; run0 is given the value of each
// that is set; doas keeps only what doas.conf keeps.
func (r Runner) keeping(env []string) []string {
	names := slices.Clone(r.Keep)

	for _, variable := range env {
		name, _, _ := strings.Cut(variable, "=")
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}

	var options []string

	switch r.Program {
	case config.ElevationSudo.String():
		if len(names) > 0 {
			options = append(options, "--preserve-env="+strings.Join(names, ","))
		}
	case config.ElevationRun0.String():
		for _, name := range names {
			if value, isSet := r.lookup(env, name); isSet {
				options = append(options, "--setenv="+name+"="+value)
			}
		}
	}

	return options
}

// lookup returns the value of the variable name in env, the last one when
// env sets it more than once, as the command gets it, or else in emergup's
// environment.
func (r Runner) lookup(env []string, name string) (string, bool) {
	for _, variable := range slices.Backward(env) {
		if value, found := strings.CutPrefix(variable, name+"="); found {
			return value, true
		}
	}

	return r.LookupEnv(name)
}
