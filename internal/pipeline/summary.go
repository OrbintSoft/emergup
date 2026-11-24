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
// status, and how the run ended.
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
