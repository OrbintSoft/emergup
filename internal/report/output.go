// SPDX-License-Identifier: GPL-3.0-or-later

// Package report collects the failure report of a run: what a failed step
// ran, the end of its output, and for each package emerge says failed its
// build log and the queries a Gentoo bug report asks for.
package report

import (
	"bytes"
	"regexp"
	"slices"
	"strings"
)

// outputLines is how many lines of a command's output a report keeps.
const outputLines = 1000

// The lines of emerge's output that name a package that failed, once the
// colours are taken off.
var (
	// failedLine is emerge's own: ">>> Failed to emerge cat/pkg-1.0, Log
	// file:", the package followed by the root it went to when that is not
	// /, and the path of the log on a line of its own when there is one.
	failedLine = regexp.MustCompile(`^>>> Failed to \S+ (\S+?)(?: (?:to|from) \S+)?(, Log file:)?\s*$`)
	// logPathLine is the line that gives the path of that log: ">>>
	// '/var/tmp/portage/cat/pkg-1.0/temp/build.log'".
	logPathLine = regexp.MustCompile(`^>>>  '(.+)'\s*$`)
	// errorLine is the ebuild's: " * ERROR: cat/pkg-1.0::repo failed
	// (compile phase):".
	errorLine = regexp.MustCompile(`^\s*\* ERROR: (\S+)::(\S+) failed \((.+?) phase\):`)
)

// Package is a package that emerge says failed.
type Package struct {
	// CPV is the package and its version, as emerge names it:
	// app-misc/foo-1.2.
	CPV string
	// Repository is the repository it comes from, "" when emerge did not
	// say.
	Repository string
	// Phase is the phase of the ebuild that failed, such as compile, ""
	// when emerge did not say.
	Phase string
	// Log is the path of its build log, "" when emerge gave none.
	Log string
}

// Output watches the output of a command as it comes: it keeps its last
// lines, without carriage returns, and the packages emerge says failed.
// It is an io.Writer, a command's Watch.
type Output struct {
	// lines holds the last lines, a ring once it is full: next is where
	// the oldest one is.
	lines []string
	next  int
	// partial is the part of a line not ended yet.
	partial  []byte
	packages []Package
	// awaitingLog is the package whose log the next line that is not
	// empty may give, -1 for none.
	awaitingLog int
}

// NewOutput returns an Output that has seen nothing yet.
func NewOutput() *Output {
	return &Output{lines: make([]string, 0, outputLines), next: 0, partial: nil, packages: nil, awaitingLog: -1}
}

// Write implements io.Writer and always reports success.
func (o *Output) Write(data []byte) (int, error) {
	o.partial = append(o.partial, data...)

	for {
		end := bytes.IndexByte(o.partial, '\n')
		if end < 0 {
			break
		}

		o.line(strings.TrimRight(string(o.partial[:end]), "\r"))
		o.partial = o.partial[end+1:]
	}

	return len(data), nil
}

// Lines returns the last lines of the output, oldest first, each with its
// newline: a last line without one gets one.
func (o *Output) Lines() []string {
	lines := slices.Concat(o.lines[o.next:], o.lines[:o.next])

	if len(o.partial) > 0 {
		lines = append(lines, strings.TrimRight(string(o.partial), "\r")+"\n")
		if len(lines) > outputLines {
			lines = lines[1:]
		}
	}

	return lines
}

// Packages returns the packages that emerge says failed, in the order it
// first names them.
func (o *Output) Packages() []Package {
	return slices.Clone(o.packages)
}

// line keeps text, a line without its newline, and reads it.
func (o *Output) line(text string) {
	if len(o.lines) < outputLines {
		o.lines = append(o.lines, text+"\n")
	} else {
		o.lines[o.next] = text + "\n"
		o.next = (o.next + 1) % outputLines
	}

	if o.awaitingLog >= 0 && text != "" {
		waiting := o.awaitingLog
		o.awaitingLog = -1

		if found := logPathLine.FindStringSubmatch(text); found != nil {
			o.packages[waiting].Log = found[1]

			return
		}
	}

	if found := failedLine.FindStringSubmatch(text); found != nil {
		index := o.packageIndex(found[1])
		if found[2] != "" {
			o.awaitingLog = index
		}

		return
	}

	if found := errorLine.FindStringSubmatch(text); found != nil {
		index := o.packageIndex(found[1])
		o.packages[index].Repository, o.packages[index].Phase = found[2], found[3]
	}
}

// packageIndex returns where cpv is among the packages, added when it is
// not there yet.
func (o *Output) packageIndex(cpv string) int {
	if index := slices.IndexFunc(o.packages, func(pkg Package) bool { return pkg.CPV == cpv }); index >= 0 {
		return index
	}

	o.packages = append(o.packages, Package{CPV: cpv, Repository: "", Phase: "", Log: ""})

	return len(o.packages) - 1
}
