// SPDX-License-Identifier: GPL-3.0-or-later

package steps

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/OrbintSoft/emergup/internal/config"
	"github.com/OrbintSoft/emergup/internal/pipeline"
	"github.com/OrbintSoft/emergup/internal/runner"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// The reasons of an update that does not take place.
const (
	nothingToUpdate = "nothing to update"
	notUpdated      = "you chose not to update"
)

// nothingToMerge matches the line with which emerge --pretend says that it
// would merge nothing.
var nothingToMerge = regexp.MustCompile(`(?m)^Total: 0 packages\b`)

// updateStep updates the target, @world by default: it shows what emerge
// would do with --pretend, asks, then does it.
type updateStep struct {
	config config.Update
}

// Name implements pipeline.Step.
func (updateStep) Name() string { return "update" }

// Title implements pipeline.Step.
func (u updateStep) Title() string { return "update " + u.config.Target }

// Enabled implements pipeline.Step.
func (u updateStep) Enabled() bool { return u.config.Enabled }

// ConfirmsItself implements pipeline.SelfConfirming: the update asks once
// the pretend has shown what it would do.
func (updateStep) ConfirmsItself() bool { return true }

// Pretend implements pipeline.Step.
func (u updateStep) Pretend(_ context.Context, env pipeline.Env) ([]string, error) {
	lines := u.work().lines(env)

	if atoms, _ := u.excluded(); len(atoms) > 0 {
		lines = append(lines, "then lists the excluded packages with an update: "+
			strings.Join(u.heldBackCommand(atoms), " "))
	}

	return lines, nil
}

// Run implements pipeline.Step. Once the update has updated, or had
// nothing to update, it says which excluded packages have an update, so
// that none is held back without the user knowing.
func (u updateStep) Run(ctx context.Context, env pipeline.Env) pipeline.Outcome {
	outcome := u.work().run(ctx, env)

	updated := outcome.Status == pipeline.Succeeded ||
		outcome.Status == pipeline.Skipped && outcome.Reason == nothingToUpdate
	if updated {
		outcome.Notes = u.heldBack(ctx, env)
	}

	return outcome
}

// excluded returns the excluded entries that are atoms, and those with a
// wildcard, which emerge accepts in --exclude but not as a package to
// check.
func (u updateStep) excluded() ([]string, []string) {
	var atoms, wildcards []string

	for _, entry := range u.config.Exclude {
		if strings.Contains(entry, "*") {
			wildcards = append(wildcards, entry)
		} else {
			atoms = append(atoms, entry)
		}
	}

	return atoms, wildcards
}

// heldBackCommand returns the command line that asks emerge which of atoms
// it would update, each alone.
func (u updateStep) heldBackCommand(atoms []string) []string {
	return slices.Concat(u.config.Command[:1], []string{pretendOption, "--update", "--nodeps", "--color=n"}, atoms)
}

// heldBack returns the notes that name the excluded packages with an
// update. The check changes nothing, so it runs unprivileged; its output
// is not shown, only read.
func (u updateStep) heldBack(ctx context.Context, env pipeline.Env) []pipeline.Note {
	atoms, wildcards := u.excluded()

	var notes []pipeline.Note
	if len(wildcards) > 0 {
		notes = append(notes, pipeline.Note{
			Text: "excluded entries with a wildcard, not checked for an update:", Rows: wildcards,
		})
	}

	if len(atoms) == 0 {
		return notes
	}

	command := u.heldBackCommand(atoms)

	result, err := env.Runner.Run(ctx, runner.Command{
		Args: command, Mode: runner.Capture, Root: false, Env: nil, Watch: nil, Into: nil,
	})

	switch {
	case err != nil:
		return append(notes, cannotCheck(err.Error()))
	case result.ExitCode != 0:
		return append(notes, cannotCheck(fmt.Sprintf("%s --pretend exited with status %d", command[0], result.ExitCode)))
	}

	held := heldBackPackages(runner.StripEscapes(string(result.Output)))

	switch len(held) {
	case 0:
		return notes
	case 1:
		return append(notes, pipeline.Note{Text: "1 excluded package has an update:", Rows: held})
	}

	return append(notes, pipeline.Note{Text: fmt.Sprintf("%d excluded packages have an update:", len(held)), Rows: held})
}

// cannotCheck is the note of a check of the excluded packages that failed.
func cannotCheck(reason string) pipeline.Note {
	return pipeline.Note{Text: "could not check the excluded packages for an update: " + reason, Rows: nil}
}

// mergeLine matches a line of emerge --pretend: its flags and its package.
var mergeLine = regexp.MustCompile(`(?m)^\[(?:ebuild|binary)([^\]]*)\]\s+(\S+)`)

// heldBackPackages returns the packages of a pretend that are updates: an
// upgrade, or a version in a new slot such as a kernel's; not a new
// install, a downgrade or a rebuild.
func heldBackPackages(output string) []string {
	var held []string

	for _, match := range mergeLine.FindAllStringSubmatch(output, -1) {
		flags := match[1]

		upgrade := strings.Contains(flags, "U") && !strings.Contains(flags, "D")
		newSlot := strings.Contains(flags, "N") && strings.Contains(flags, "S")

		if upgrade || newSlot {
			atom, _, _ := strings.Cut(match[2], ":")
			held = append(held, atom)
		}
	}

	return held
}

// work is what the update does: --pretend, then emergup's question.
func (u updateStep) work() pretended {
	return pretended{
		command:     u.command,
		nothing:     nothingToMerge,
		nothingDone: nothingToUpdate,
		question:    "Update these packages?",
		goOn:        ui.Action{Key: 'y', Name: "update them"},
		skip:        ui.Action{Key: 's', Name: "skip the update"},
		skipped:     notUpdated,
	}
}

// command returns the update's command line, with --pretend or not: the
// configured command and options, the colours of emergup, an --exclude for
// each excluded package, and the target.
func (u updateStep) command(env pipeline.Env, pretend bool) []string {
	command := slices.Concat(u.config.Command, u.config.Options)
	if pretend {
		command = append(command, pretendOption)
	}

	command = append(command, colorOption(env))

	for _, excluded := range u.config.Exclude {
		command = append(command, "--exclude", excluded)
	}

	return append(command, u.config.Target)
}

// pretendOption asks emerge, or smart-live-rebuild, what it would do
// without doing it.
const pretendOption = "--pretend"

// colorOption returns the option that makes emerge write in colour when
// emergup does: its output reaches the terminal through a pipe, where it
// would not by itself.
func colorOption(env pipeline.Env) string {
	if env.Printer.Color {
		return "--color=y"
	}

	return "--color=n"
}
