// SPDX-License-Identifier: GPL-3.0-or-later

package steps

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/OrbintSoft/emergup/internal/config"
	"github.com/OrbintSoft/emergup/internal/pipeline"
	"github.com/OrbintSoft/emergup/internal/runner"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// errNoDatabase is the error of a portageq that names no package database.
var errNoDatabase = errors.New("portageq named no database")

// What the live rebuild says when it has nothing to rebuild.
const (
	noLivePackage    = "no live package installed"
	nothingToRebuild = "nothing to rebuild"
)

// excludeLiveBinaries keeps emerge from installing a live package from a
// binary package, an old build of it, instead of building it.
const excludeLiveBinaries = "--usepkg-exclude-live=y"

// The marks of the package database: the property of a live package, and
// the prefix of a package being merged.
const (
	liveProperty  = "live"
	mergingPrefix = "-MERGING-"
)

// liveStep rebuilds the live packages, as its mode says.
type liveStep struct {
	config config.LiveRebuild
}

// Name implements pipeline.Step.
func (liveStep) Name() string { return "live-rebuild" }

// Title implements pipeline.Step.
func (liveStep) Title() string { return "rebuild the live packages" }

// Enabled implements pipeline.Step.
func (l liveStep) Enabled() bool { return l.config.Enabled }

// ConfirmsItself implements pipeline.SelfConfirming: the step lists the
// packages it would rebuild, then asks, and skips without asking when there
// are none.
func (liveStep) ConfirmsItself() bool { return true }

// Pretend implements pipeline.Step. In the smart mode, listing the packages
// updates the clones of their repositories, as root, so it only says what
// the step would do.
func (l liveStep) Pretend(ctx context.Context, env pipeline.Env) ([]string, error) {
	mode, installed, note := l.resolve(env)

	if mode == config.LiveRebuildSmart {
		if !installed {
			return []string{l.smartProgram() + " is not installed, so the step would be skipped"}, nil
		}

		return []string{
			"would list the live packages to rebuild, as root, with: " + strings.Join(l.smartList(), " "),
			"and if there are any run: " + strings.Join(l.command(env, nil), " ") + " followed by them",
		}, nil
	}

	decisions, err := l.plan(ctx, env, mode)
	if err != nil {
		return nil, err
	}

	var lines []string
	if note != "" {
		lines = append(lines, note)
	}

	if len(decisions) == 0 {
		return append(lines, noLivePackage), nil
	}

	for _, row := range rows(decisions) {
		lines = append(lines, row.text)
	}

	due := chosen(decisions)
	if len(due) == 0 {
		return append(lines, nothingToRebuild), nil
	}

	return append(lines, strings.Join(l.command(env, versioned(due)), " ")), nil
}

// Run implements pipeline.Step.
func (l liveStep) Run(ctx context.Context, env pipeline.Env) pipeline.Outcome {
	mode, installed, note := l.resolve(env)

	if mode == config.LiveRebuildSmart {
		return l.runSmart(ctx, env, installed)
	}

	if note != "" {
		env.Printer.Row(ui.KindPlain, note)
	}

	return l.runList(ctx, env, mode)
}

// runSmart asks smart-live-rebuild which live packages to rebuild, as it
// checks their upstreams, then rebuilds them with emerge, which does not
// check the upstreams again; one that is not installed skips the step.
func (l liveStep) runSmart(ctx context.Context, env pipeline.Env, installed bool) pipeline.Outcome {
	if !installed {
		return pipeline.Skip(l.smartProgram() + " is not installed")
	}

	command := runner.Command{Args: l.smartList(), Mode: runner.Capture, Root: true, Env: nil, Watch: nil, Into: nil}

	result, err := env.Runner.Run(ctx, command)

	switch {
	case err != nil:
		return pipeline.Fail(err.Error())
	case result.ExitCode != 0:
		return pipeline.Fail(fmt.Sprintf("%s exited with status %d", l.smartProgram(), result.ExitCode))
	}

	packages := strings.Fields(string(result.Output))
	if len(packages) == 0 {
		return pipeline.Skip(l.smartProgram() + " found nothing to rebuild")
	}

	for _, atom := range packages {
		env.Printer.Row(ui.KindPlain, atom)
	}

	return l.confirmRebuild(ctx, env, packages)
}

// smartList is the command line with which smart-live-rebuild lists the
// live packages to rebuild, without rebuilding them: smart-command with
// --pretend after its program.
func (l liveStep) smartList() []string {
	return slices.Concat(l.config.SmartCommand[:1], []string{pretendOption}, l.config.SmartCommand[1:])
}

// confirmRebuild asks whether to rebuild the packages atoms name, and
// rebuilds them, as root, if so.
func (l liveStep) confirmRebuild(ctx context.Context, env pipeline.Env, atoms []string) pipeline.Outcome {
	rebuild := ui.Action{Key: 'y', Name: "rebuild them"}
	skip := ui.Action{Key: 's', Name: "skip the rebuild"}

	outcome, goOn := confirm(ctx, env, "Rebuild them?", rebuild, skip, notRebuilt)
	if !goOn {
		return outcome
	}

	command := l.command(env, atoms)

	return runElevated(ctx, env, command, command[0])
}

// runList rebuilds the live packages that mode says are due.
func (l liveStep) runList(ctx context.Context, env pipeline.Env, mode config.LiveRebuildMode) pipeline.Outcome {
	decisions, err := l.plan(ctx, env, mode)
	if err != nil {
		return pipeline.Fail(err.Error())
	}

	if len(decisions) == 0 {
		return pipeline.Skip(noLivePackage)
	}

	for _, row := range rows(decisions) {
		env.Printer.Row(row.kind, row.text)
	}

	due := chosen(decisions)
	if len(due) == 0 {
		return pipeline.Skip("every live package was built less than " + l.interval() + " ago")
	}

	return l.confirmRebuild(ctx, env, versioned(due))
}

// runElevated runs command as root, its output shown, and returns how it
// ended; program names it in a failure.
func runElevated(ctx context.Context, env pipeline.Env, command []string, program string) pipeline.Outcome {
	return runElevatedIn(ctx, env, runner.Stream, command, program)
}

// runElevatedIn runs command as root in mode, and returns how it ended;
// program names it in a failure.
func runElevatedIn(ctx context.Context, env pipeline.Env, mode runner.Mode, command []string,
	program string,
) pipeline.Outcome {
	return runCommand(ctx, env, runner.Command{
		Args: command, Mode: mode, Root: true, Env: nil, Watch: nil, Into: nil,
	}, program)
}

// runCommand runs command and returns how it ended; program names it in a
// failure.
func runCommand(ctx context.Context, env pipeline.Env, command runner.Command, program string) pipeline.Outcome {
	result, err := env.Runner.Run(ctx, command)
	if err != nil {
		return pipeline.Fail(err.Error())
	}

	if result.ExitCode != 0 {
		return pipeline.Fail(fmt.Sprintf("%s exited with status %d", program, result.ExitCode))
	}

	return pipeline.Success()
}

// resolve returns the mode the step works by, whether smart-live-rebuild
// is installed, and for auto without it a note that says how the packages
// are rebuilt instead.
func (l liveStep) resolve(env pipeline.Env) (config.LiveRebuildMode, bool, string) {
	_, err := env.LookPath(l.config.SmartCommand[0])
	installed := err == nil

	if l.config.Mode != config.LiveRebuildAuto {
		return l.config.Mode, installed, ""
	}

	if installed {
		return config.LiveRebuildSmart, true, ""
	}

	return config.LiveRebuildInterval, false, l.smartProgram() +
		" is not installed: the live packages built longer ago than " + l.interval() + " are rebuilt"
}

// smartProgram names smart-live-rebuild as its command does.
func (l liveStep) smartProgram() string {
	return filepath.Base(l.config.SmartCommand[0])
}

// interval writes the interval as the configuration does: "24h".
func (l liveStep) interval() string {
	text, _ := l.config.Interval.MarshalText()

	return string(text)
}

// command returns the command line that rebuilds the packages atoms name,
// each from its sources.
func (l liveStep) command(env pipeline.Env, atoms []string) []string {
	return slices.Concat(l.config.Command, []string{colorOption(env), excludeLiveBinaries}, atoms)
}

// versioned returns the atoms of the installed packages due, each at its
// installed version, so that no other version is picked.
func versioned(due []string) []string {
	atoms := make([]string, 0, len(due))
	for _, installed := range due {
		atoms = append(atoms, "="+installed)
	}

	return atoms
}

// plan decides what to do with each live package installed: by interval,
// or with always all of them.
func (l liveStep) plan(ctx context.Context, env pipeline.Env, mode config.LiveRebuildMode) ([]decision, error) {
	database, err := packageDatabase(ctx, env.Runner)
	if err != nil {
		return nil, err
	}

	packages, err := livePackages(env.Root, database)
	if err != nil {
		return nil, err
	}

	decisions := make([]decision, 0, len(packages))
	interval := time.Duration(l.config.Interval)
	now := env.Now()

	for _, installed := range packages {
		switch {
		case mode == config.LiveRebuildAlways, interval == 0:
			decisions = append(decisions, decision{name: installed.atom, act: true, reason: "rebuilt on every run"})
		case !installed.known:
			decisions = append(decisions, decision{name: installed.atom, act: true, reason: "build time not known"})
		default:
			age := max(now.Sub(installed.built), 0)
			built := "built " + ui.FormatDuration(age) + " ago"

			if age >= interval {
				decisions = append(decisions, decision{name: installed.atom, act: true, reason: built})
			} else {
				decisions = append(decisions, decision{name: installed.atom, act: false, reason: built + ", kept"})
			}
		}
	}

	return decisions, nil
}

// packageDatabase asks Portage where the installed packages are; the query
// changes nothing, so it runs unprivileged.
func packageDatabase(ctx context.Context, run runner.Runner) (string, error) {
	command := []string{portageq, "vdb_path"}

	result, err := run.Run(ctx, runner.Command{
		Args: command, Mode: runner.Capture, Root: false, Env: nil, Watch: nil, Into: nil,
	})
	if err != nil {
		return "", fmt.Errorf("could not find the installed packages: %w", err)
	}

	if result.ExitCode != 0 {
		return "", fmt.Errorf("could not find the installed packages: %w",
			exitError{command: command[0], code: result.ExitCode})
	}

	database := strings.TrimSpace(string(result.Output))
	if !path.IsAbs(database) {
		return "", fmt.Errorf("could not find the installed packages: %w", errNoDatabase)
	}

	return database, nil
}

// installedPackage is a live package of the database.
type installedPackage struct {
	// atom is its category and full name with its version: "app-misc/foo-9999".
	atom string
	// built is when it was built, when known.
	built time.Time
	known bool
}

// livePackages returns the live packages of the database at database on
// root: those with the live property, which make up @live-rebuild.
func livePackages(root fs.FS, database string) ([]installedPackage, error) {
	dir := strings.TrimPrefix(path.Clean(database), "/")

	categories, err := fs.ReadDir(root, dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("could not read the installed packages: %w", err)
	}

	var packages []installedPackage

	for _, category := range categories {
		if !category.IsDir() {
			continue
		}

		entries, err := fs.ReadDir(root, path.Join(dir, category.Name()))
		if err != nil {
			return nil, fmt.Errorf("could not read the installed packages: %w", err)
		}

		for _, entry := range entries {
			name := path.Join(category.Name(), entry.Name())
			if entry.IsDir() && !strings.HasPrefix(entry.Name(), mergingPrefix) && isLive(root, path.Join(dir, name)) {
				built, known := buildTime(root, path.Join(dir, name))
				packages = append(packages, installedPackage{atom: name, built: built, known: known})
			}
		}
	}

	return packages, nil
}

// isLive reports whether the package whose entry of the database is dir
// has the live property.
func isLive(root fs.FS, dir string) bool {
	properties, err := fs.ReadFile(root, path.Join(dir, "PROPERTIES"))

	return err == nil && slices.Contains(strings.Fields(string(properties)), liveProperty)
}

// buildTime returns when the package whose entry of the database is dir
// was built, as its BUILD_TIME says, and whether it is known.
func buildTime(root fs.FS, dir string) (time.Time, bool) {
	data, err := fs.ReadFile(root, path.Join(dir, "BUILD_TIME"))
	if err != nil {
		return time.Time{}, false
	}

	seconds, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return time.Time{}, false
	}

	return time.Unix(seconds, 0), true
}
