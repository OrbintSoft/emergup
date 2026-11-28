// SPDX-License-Identifier: GPL-3.0-or-later

package steps

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/OrbintSoft/emergup/internal/config"
	"github.com/OrbintSoft/emergup/internal/pipeline"
	"github.com/OrbintSoft/emergup/internal/runner"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// The reasons of a config-files step that does not run its tool.
const (
	nothingToMergeIn = "no configuration file to update"
	notMerged        = "you chose not to update them"
)

// pendingPattern is the name find looks for: ._cfg0000_name is the update
// of name that Portage left, as CONFIG_PROTECT asks.
const pendingPattern = "._cfg????_*"

// pendingName matches the name of a pending update, and gives the name of
// the file it would replace.
var pendingName = regexp.MustCompile(`^\._cfg[0-9]{4}_(.+)$`)

// configFilesStep has the configured tool, dispatch-conf by default, merge
// the configuration files that updates left pending.
type configFilesStep struct {
	config config.Step
}

// Name implements pipeline.Step.
func (configFilesStep) Name() string { return "config-files" }

// Title implements pipeline.Step.
func (configFilesStep) Title() string { return "update the configuration files" }

// Enabled implements pipeline.Step.
func (c configFilesStep) Enabled() bool { return c.config.Enabled }

// ConfirmsItself implements pipeline.SelfConfirming: the step asks once it
// knows there is something to update, and skips without asking otherwise.
func (configFilesStep) ConfirmsItself() bool { return true }

// Pretend implements pipeline.Step. Looking for the pending files needs
// root, which a pretend does not use: it says what the step would do.
func (c configFilesStep) Pretend(context.Context, pipeline.Env) ([]string, error) {
	return []string{"would look for the pending configuration files, as root, then run: " +
		strings.Join(c.config.Command, " ")}, nil
}

// Run implements pipeline.Step. The tool reads the user's keys, so it is
// attached to the terminal; with --yes nobody is there to answer, and it
// does not run.
func (c configFilesStep) Run(ctx context.Context, env pipeline.Env) pipeline.Outcome {
	pending, err := pendingFiles(ctx, env)
	if err != nil {
		return pipeline.Fail(err.Error())
	}

	if len(pending) == 0 {
		return pipeline.Skip(nothingToMergeIn)
	}

	for _, file := range pending {
		env.Printer.Row(ui.KindPlain, file)
	}

	tool := filepath.Base(c.config.Command[0])

	if env.Menu.Yes {
		return pipeline.Skip(tool + " is interactive: run it to update them")
	}

	merge := ui.Action{Key: 'y', Name: "update them"}
	skip := ui.Action{Key: 's', Name: "skip the update"}

	if outcome, goOn := confirm(ctx, env, "Update them with "+tool+"?", merge, skip, notMerged); !goOn {
		return outcome
	}

	return runElevatedIn(ctx, env, runner.Terminal, c.config.Command, tool)
}

// pendingFiles returns the files that a pending update would replace, in
// order. The protected directories are searched as root: some of them can
// be read by root alone, and an update in them would be missed otherwise.
// The search changes nothing.
func pendingFiles(ctx context.Context, env pipeline.Env) ([]string, error) {
	protected, masked, err := protectedPaths(ctx, env.Runner)
	if err != nil {
		return nil, err
	}

	var dirs, files []string

	for _, protectedPath := range protected {
		info, err := fs.Stat(env.Root, strings.TrimPrefix(path.Clean(protectedPath), "/"))

		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err == nil && !info.IsDir():
			files = append(files, protectedPath)
		default:
			dirs = append(dirs, protectedPath)
		}
	}

	found, err := findPending(ctx, env.Runner, dirs, false)
	if err != nil {
		return nil, err
	}

	nextToFiles, err := findPending(ctx, env.Runner, parents(files), true)
	if err != nil {
		return nil, err
	}

	pending := slices.Concat(filterPending(found, masked, nil), filterPending(nextToFiles, masked, files))
	slices.Sort(pending)

	return slices.Compact(pending), nil
}

// protectedPaths asks Portage which paths CONFIG_PROTECT protects and
// which CONFIG_PROTECT_MASK leaves out; the query runs unprivileged.
func protectedPaths(ctx context.Context, run runner.Runner) ([]string, []string, error) {
	command := []string{portageq, "envvar", "CONFIG_PROTECT", "CONFIG_PROTECT_MASK"}

	result, err := run.Run(ctx, runner.Command{
		Args: command, Mode: runner.Capture, Root: false, Env: nil, Watch: nil, Into: nil,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("could not find the protected configuration: %w", err)
	}

	if result.ExitCode != 0 {
		return nil, nil, fmt.Errorf("could not find the protected configuration: %w",
			exitError{command: command[0], code: result.ExitCode})
	}

	protected, masked, _ := strings.Cut(string(result.Output), "\n")

	return strings.Fields(protected), strings.Fields(masked), nil
}

// parents returns the directories of files, each once.
func parents(files []string) []string {
	dirs := make([]string, 0, len(files))
	for _, file := range files {
		dirs = append(dirs, path.Dir(file))
	}

	slices.Sort(dirs)

	return slices.Compact(dirs)
}

// findPending runs find as root for the pending updates in dirs, only in
// dirs themselves when shallow, and returns the paths it prints.
func findPending(ctx context.Context, run runner.Runner, dirs []string, shallow bool) ([]string, error) {
	if len(dirs) == 0 {
		return nil, nil
	}

	command := append([]string{"find"}, dirs...)
	if shallow {
		command = append(command, "-maxdepth", "1")
	}

	command = append(command, "-name", pendingPattern, "-print")

	result, err := run.Run(ctx, runner.Command{
		Args: command, Mode: runner.Capture, Root: true, Env: nil, Watch: nil, Into: nil,
	})
	if err != nil {
		return nil, fmt.Errorf("could not look for the configuration files to update: %w", err)
	}

	if result.ExitCode != 0 {
		return nil, fmt.Errorf("could not look for the configuration files to update: %w",
			exitError{command: command[0], code: result.ExitCode})
	}

	return strings.Fields(string(result.Output)), nil
}

// filterPending keeps the pending updates of found that Portage counts, as
// the files they would replace: not under a masked path, not an editor's
// copy or a backup, and, when files is not nil, only those of files.
func filterPending(found, masked, files []string) []string {
	var pending []string

	for _, update := range found {
		match := pendingName.FindStringSubmatch(path.Base(update))

		switch {
		case match == nil, strings.HasSuffix(update, "~"), strings.HasSuffix(strings.ToLower(update), ".bak"):
			continue
		case slices.ContainsFunc(masked, func(mask string) bool { return isUnder(update, mask) }):
			continue
		}

		target := path.Join(path.Dir(update), match[1])
		if files == nil || slices.Contains(files, target) {
			pending = append(pending, target)
		}
	}

	return pending
}

// isUnder reports whether file is dir or is in it.
func isUnder(file, dir string) bool {
	dir = path.Clean(dir)

	return file == dir || strings.HasPrefix(file, dir+"/")
}
