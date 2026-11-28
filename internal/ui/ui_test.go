// SPDX-License-Identifier: GPL-3.0-or-later

package ui_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/OrbintSoft/emergup/internal/config"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// The environment variables that turn the colours off.
const (
	noColor = "NO_COLOR"
	term    = "TERM"
)

// lessThanASecond is how a duration under half a second is written.
const lessThanASecond = "<1s"

// environment returns a getenv that finds only vars.
func environment(vars map[string]string) func(string) string {
	return func(name string) string { return vars[name] }
}

func TestColorEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		terminal bool
		vars     map[string]string
		want     bool
	}{
		{"a terminal", true, map[string]string{term: "xterm-256color"}, true},
		{"a terminal without TERM", true, nil, true},
		{"not a terminal", false, map[string]string{term: "xterm-256color"}, false},
		{"NO_COLOR set", true, map[string]string{noColor: "1"}, false},
		{"NO_COLOR empty", true, map[string]string{noColor: ""}, true},
		{"a dumb terminal", true, map[string]string{term: "dumb"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, ui.ColorEnabled(tt.terminal, environment(tt.vars)))
		})
	}
}

// printed returns what each kind of line of printer writes for "text".
func printed(printer ui.Printer, out *bytes.Buffer) map[string]string {
	lines := map[string]func(string){
		"heading": printer.Heading,
		"success": printer.Success,
		"skip":    printer.Skip,
		"failure": printer.Failure,
		"notice":  printer.Notice,
	}

	written := make(map[string]string, len(lines))

	for kind, line := range lines {
		out.Reset()
		line("text")

		written[kind] = out.String()
	}

	return written
}

func TestPrinterColoursEachKindOfLine(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	printer := ui.Printer{Out: &out, Colors: config.Default().Colors, Color: true}
	assert.Equal(t, map[string]string{
		"heading": "\x1b[1;34m==> text\x1b[0m\n",
		"success": "\x1b[32m==> text\x1b[0m\n",
		"skip":    "\x1b[33m==> text\x1b[0m\n",
		"failure": "\x1b[1;31m==> text\x1b[0m\n",
		"notice":  "\x1b[35m==> text\x1b[0m\n",
	}, printed(printer, &out))
}

func TestPrinterWithoutColours(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	printer := ui.Printer{Out: &out, Colors: config.Default().Colors, Color: false}
	for kind, line := range printed(printer, &out) {
		assert.Equal(t, "==> text\n", line, kind)
	}
}

func TestPrinterLeavesAPlainKindPlain(t *testing.T) {
	t.Parallel()

	var (
		out   bytes.Buffer
		plain config.Style
	)

	colors := config.Default().Colors
	colors.Skip = plain

	printer := ui.Printer{Out: &out, Colors: colors, Color: true}
	written := printed(printer, &out)
	assert.Equal(t, "==> text\n", written["skip"])
	assert.Equal(t, "\x1b[32m==> text\x1b[0m\n", written["success"], "the other kinds keep their colour")
}

func TestFormatDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		duration time.Duration
		want     string
	}{
		{0, lessThanASecond},
		{499 * time.Millisecond, lessThanASecond},
		{500 * time.Millisecond, "1s"},
		{42*time.Second + 400*time.Millisecond, "42s"},
		{59*time.Second + 600*time.Millisecond, "1m00s"},
		{12*time.Minute + 3*time.Second, "12m03s"},
		{59*time.Minute + 59*time.Second + 600*time.Millisecond, "1h00m"},
		{2*time.Hour + 3*time.Minute + 29*time.Second, "2h03m"},
		{2*time.Hour + 3*time.Minute + 30*time.Second, "2h04m"},
		{26*time.Hour + 5*time.Minute, "26h05m"},
		{-time.Second, lessThanASecond},
	}
	for _, tt := range tests {
		t.Run(tt.duration.String(), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, ui.FormatDuration(tt.duration))
		})
	}
}

func TestPrinterRow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind    ui.Kind
		colored string
	}{
		{ui.KindPlain, "    text\n"},
		{ui.KindSuccess, "    \x1b[32mtext\x1b[0m\n"},
		{ui.KindSkip, "    \x1b[33mtext\x1b[0m\n"},
		{ui.KindFailure, "    \x1b[1;31mtext\x1b[0m\n"},
	}
	for _, tt := range tests {
		var out bytes.Buffer

		ui.Printer{Out: &out, Colors: config.Default().Colors, Color: true}.Row(tt.kind, "text")
		assert.Equal(t, tt.colored, out.String(), "coloured, kind %d", tt.kind)

		out.Reset()
		ui.Printer{Out: &out, Colors: config.Default().Colors, Color: false}.Row(tt.kind, "text")
		assert.Equal(t, "    text\n", out.String(), "without colours, kind %d", tt.kind)
	}
}

func TestPrinterCopiesEveryLineToTheLogWithoutColours(t *testing.T) {
	t.Parallel()

	var out, log bytes.Buffer

	printer := ui.Printer{Out: &out, Colors: config.Default().Colors, Color: true, Log: &log}
	printer.Heading("heading")
	printer.Notice("notice")
	printer.Row(ui.KindFailure, "row")
	printer.Info("info")

	assert.Equal(t, "\x1b[1;34m==> heading\x1b[0m\n"+
		"\x1b[35m==> notice\x1b[0m\n"+
		"    \x1b[1;31mrow\x1b[0m\n"+
		"==> info\n", out.String(), "an info line has no colour")
	assert.Equal(t, "==> heading\n==> notice\n    row\n==> info\n", log.String())
}

func TestPrinterWritesTextAsItIs(t *testing.T) {
	t.Parallel()

	var out, log bytes.Buffer

	printer := ui.Printer{Out: &out, Colors: config.Default().Colors, Color: true, Log: &log}
	printer.Text("The build ")
	printer.Text("fails.\n")

	assert.Equal(t, "The build fails.\n", out.String(), "no prefix, no colour, no newline of its own")
	assert.Equal(t, "The build fails.\n", log.String())
}
