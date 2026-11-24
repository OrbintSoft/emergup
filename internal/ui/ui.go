// SPDX-License-Identifier: GPL-3.0-or-later

// Package ui writes emergup's own output on the terminal: a line for each
// kind of message, in the colour the configuration gives that kind.
package ui

import (
	"fmt"
	"io"
	"time"

	"github.com/OrbintSoft/emergup/internal/config"
)

// linePrefix starts every line emergup writes, which sets them apart from
// the output of the commands it runs.
const linePrefix = "==> "

// ColorEnabled reports whether the output is coloured: only on a terminal,
// and neither when NO_COLOR is set to anything but "" (no-color.org) nor
// when TERM says the terminal cannot show colours. getenv is os.Getenv.
func ColorEnabled(terminal bool, getenv func(string) string) bool {
	return terminal && getenv("NO_COLOR") == "" && getenv("TERM") != "dumb"
}

// Printer writes emergup's own lines to Out, each in the style Colors gives
// its kind when Color is set. A failed write is not reported: the lines are
// for the person watching, and the log records what happened.
type Printer struct {
	Out    io.Writer
	Colors config.Colors
	// Color turns the colours on; see ColorEnabled.
	Color bool
}

// Heading writes the heading of a step.
func (p Printer) Heading(text string) { p.line(&p.Colors.Heading, text) }

// Success writes that something succeeded.
func (p Printer) Success(text string) { p.line(&p.Colors.Success, text) }

// Skip writes that something was skipped.
func (p Printer) Skip(text string) { p.line(&p.Colors.Skip, text) }

// Failure writes that something failed.
func (p Printer) Failure(text string) { p.line(&p.Colors.Failure, text) }

// Notice writes a warning that is not a failure.
func (p Printer) Notice(text string) { p.line(&p.Colors.Notice, text) }

// line writes text as a line in style. The style ends before the newline,
// so that a background colour does not run on to the next line.
func (p Printer) line(style *config.Style, text string) {
	sgr := style.SGR()
	if !p.Color || sgr == "" {
		_, _ = fmt.Fprintf(p.Out, "%s%s\n", linePrefix, text)

		return
	}

	_, _ = fmt.Fprintf(p.Out, "\x1b[%sm%s%s\x1b[0m\n", sgr, linePrefix, text)
}

// FormatDuration writes a duration the way the step times show it: "<1s",
// "42s", "12m03s", and from an hour on to the minute, "2h03m".
func FormatDuration(duration time.Duration) string {
	duration = duration.Round(time.Second)
	if duration >= time.Hour {
		duration = duration.Round(time.Minute)
	}

	switch {
	case duration <= 0:
		return "<1s"
	case duration < time.Minute:
		return fmt.Sprintf("%ds", duration/time.Second)
	case duration < time.Hour:
		return fmt.Sprintf("%dm%02ds", duration/time.Minute, duration%time.Minute/time.Second)
	}

	return fmt.Sprintf("%dh%02dm", duration/time.Hour, duration%time.Hour/time.Minute)
}
