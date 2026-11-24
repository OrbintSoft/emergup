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
	// Log, when set, receives a copy of everything written to Out, without
	// colours.
	Log io.Writer
}

// Info writes a line without colour.
func (p Printer) Info(text string) { p.styled("", nil, linePrefix+text, "\n") }

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

// Kind is a kind of row, each in the colour of the outcome it reports.
type Kind int

// The kinds of row.
const (
	// KindPlain has no colour.
	KindPlain Kind = iota
	KindSuccess
	KindSkip
	KindFailure
)

// rowIndent starts every row, which sets them under the line before.
const rowIndent = "    "

// Row writes text as an indented row of a table, such as the summary of a
// run, in the colour of kind.
func (p Printer) Row(kind Kind, text string) {
	p.styled(rowIndent, p.kindStyle(kind), text, "\n")
}

// kindStyle returns the style of kind, nil for KindPlain.
func (p Printer) kindStyle(kind Kind) *config.Style {
	switch kind {
	case KindSuccess:
		return &p.Colors.Success
	case KindSkip:
		return &p.Colors.Skip
	case KindFailure:
		return &p.Colors.Failure
	case KindPlain:
	}

	return nil
}

// line writes text as a line in style.
func (p Printer) line(style *config.Style, text string) {
	p.styled("", style, linePrefix+text, "\n")
}

// prompt writes text as a question in the prompt's style, followed by a
// space and without a newline: the answer ends the line.
func (p Printer) prompt(text string) {
	p.styled("", &p.Colors.Prompt, linePrefix+text, " ")
}

// styled writes text in style, nil for none, between before and after,
// which keep no style: a background colour must not run on to the next
// line.
func (p Printer) styled(before string, style *config.Style, text, after string) {
	sgr := ""
	if p.Color && style != nil {
		sgr = style.SGR()
	}

	p.copyToLog(before + text + after)

	if sgr == "" {
		_, _ = fmt.Fprint(p.Out, before, text, after)

		return
	}

	_, _ = fmt.Fprintf(p.Out, "%s\x1b[%sm%s\x1b[0m%s", before, sgr, text, after)
}

// write writes text as it is, without a style.
func (p Printer) write(text string) {
	p.copyToLog(text)

	_, _ = fmt.Fprint(p.Out, text)
}

// copyToLog writes text to the log, when there is one.
func (p Printer) copyToLog(text string) {
	if p.Log != nil {
		_, _ = fmt.Fprint(p.Log, text)
	}
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
