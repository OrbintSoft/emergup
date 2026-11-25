// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"fmt"
	"strings"

	"github.com/OrbintSoft/emergup/internal/ui"
)

// durationWidth is the width of the duration column, enough for "12m03s".
const durationWidth = len("12m03s")

// writeSummary writes a row for each step of report, in the colour of its
// status, the hooks that failed, and how the run ended.
func writeSummary(printer ui.Printer, report Report) {
	printer.Heading("Summary")

	nameWidth := 0
	for _, step := range report.Steps {
		nameWidth = max(nameWidth, len(step.Name))
	}

	statusWidth := len(Succeeded.String())

	for _, step := range report.Steps {
		took := ""
		if step.Ran {
			took = ui.FormatDuration(step.Duration)
		}

		row := fmt.Sprintf("%-*s  %-*s  %*s  %s",
			nameWidth, step.Name, statusWidth, step.Status, durationWidth, took, step.Reason)
		printer.Row(statusKind(step.Status), strings.TrimRight(row, " "))
	}

	writeFailedHooks(printer, report.FailedHooks)
	writeNotes(printer, report.Steps)

	took := ui.FormatDuration(report.Duration)

	switch report.Result {
	case RunFinished:
		printer.Success("The run finished in " + took + ".")
	case RunFailed:
		printer.Failure("The run failed after " + took + ".")
	case RunStopped:
		printer.Failure("The run was stopped after " + took + ".")
	}
}

// writeFailedHooks writes a line for each hook of hooks, a failure or, for
// a hook after which the run went on, a notice.
func writeFailedHooks(printer ui.Printer, hooks []FailedHook) {
	for _, hook := range hooks {
		failure := fmt.Sprintf("The hook %s (%s) failed: %s", hook.Name, hook.Place, hook.Reason)

		if hook.WentOn {
			printer.Notice(failure + "; the run went on.")
		} else {
			printer.Failure(failure + ".")
		}
	}
}

// writeNotes writes the notes of steps, each with its rows.
func writeNotes(printer ui.Printer, steps []StepReport) {
	for _, step := range steps {
		for _, note := range step.Notes {
			printer.Notice(note.Text)

			for _, row := range note.Rows {
				printer.Row(ui.KindPlain, row)
			}
		}
	}
}

// statusKind returns the kind of row of a step with status.
func statusKind(status Status) ui.Kind {
	switch status {
	case Succeeded:
		return ui.KindSuccess
	case Skipped, Disabled:
		return ui.KindSkip
	case Failed, Stopped:
		return ui.KindFailure
	case NotRun:
	}

	return ui.KindPlain
}
