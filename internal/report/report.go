// SPDX-License-Identifier: GPL-3.0-or-later

package report

import (
	"context"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/OrbintSoft/emergup/internal/runner"
)

// The files of a report that do not depend on a package.
const (
	summaryFile = "summary.txt"
	outputFile  = "output.txt"
)

// compressions are the suffixes of a build log that Portage compressed,
// which its copy keeps.
var compressions = []string{".gz", ".bz2", ".xz", ".zst", ".lz4", ".lzma", ".lz"} //nolint:gochecknoglobals // A table.

// Failure is a step that failed, as its report tells it.
type Failure struct {
	// Step is the step's name, Title what it does: update, "update @world".
	Step  string
	Title string
	// Reason is why it failed, as the summary of the run says.
	Reason string
	// Ran is the last command the step ran, when RanAny says it ran one.
	Ran    Ran
	RanAny bool
	// Log is the path of the run's log.
	Log string
}

// Files is where a report is written: a directory of its own, whose files
// only the invoking user can read.
type Files interface {
	// Create makes the file name, which must not exist yet, and opens it
	// for writing.
	Create(name string) (io.WriteCloser, error)
}

// Write writes the report of failure into files: for each package emerge
// says failed, its build log, copied as root with run, and the output of
// emerge --info and emerge -pqv, run unprivileged; the end of the command's
// output; and last a summary of it all. It returns what it could not
// collect, which the summary says too.
func Write(ctx context.Context, files Files, run runner.Runner, failure Failure) []string {
	collector := &collector{files: files, run: run, problems: nil}

	var packages []collected

	if failure.RanAny {
		for _, pkg := range failure.Ran.Output.Packages() {
			packages = append(packages, collector.collect(ctx, pkg))
		}

		collector.write(outputFile, strings.Join(failure.Ran.Output.Lines(), ""))
	}

	problems := collector.problems
	if !collector.write(summaryFile, summaryText(failure, packages, problems)) {
		return collector.problems
	}

	return problems
}

// collector writes the files of a report and keeps what it could not
// collect.
type collector struct {
	files    Files
	run      runner.Runner
	problems []string
}

// collected is what the report has of a package: the names of its files,
// "" for one not collected.
type collected struct {
	pkg      Package
	buildLog string
	info     string
	pqv      string
}

// collect writes the files of pkg.
func (c *collector) collect(ctx context.Context, pkg Package) collected {
	base := fileName(pkg.CPV)

	atom := "=" + pkg.CPV
	if pkg.Repository != "" {
		atom += "::" + pkg.Repository
	}

	files := collected{pkg: pkg, buildLog: "", info: "", pqv: ""}

	if pkg.Log == "" {
		c.problems = append(c.problems, "emerge named no build log for "+pkg.CPV)
	} else {
		name := base + ".build.log" + compression(pkg.Log)
		command := runner.Command{
			Args: []string{"cat", "--", pkg.Log}, Mode: runner.Save, Root: true, Env: nil, Watch: nil, Into: nil,
		}
		files.buildLog = c.save(ctx, name, command, "the build log of "+pkg.CPV+" could not be copied")
	}

	info := runner.Command{
		Args: []string{"emerge", "--info", atom}, Mode: runner.Save, Root: false, Env: nil, Watch: nil, Into: nil,
	}
	files.info = c.save(ctx, base+".emerge-info.txt", info, "emerge --info for "+pkg.CPV+" could not run")

	pqv := runner.Command{
		Args: []string{"emerge", "-pqv", atom}, Mode: runner.Save, Root: false, Env: nil, Watch: nil, Into: nil,
	}
	files.pqv = c.save(ctx, base+".emerge-pqv.txt", pqv, "emerge -pqv for "+pkg.CPV+" could not run")

	return files
}

// save runs command with its standard output going to the file name, and
// returns name, or "" when the file is not complete; failed says what
// failed, before why.
func (c *collector) save(ctx context.Context, name string, command runner.Command, failed string) string {
	file, err := c.files.Create(name)
	if err != nil {
		c.problems = append(c.problems, fmt.Sprintf("could not write %s: %v", name, err))

		return ""
	}

	command.Into = file
	result, err := c.run.Run(ctx, command)

	closeErr := file.Close()

	switch {
	case err != nil:
		c.problems = append(c.problems, failed+": "+err.Error())
	case result.ExitCode != 0:
		c.problems = append(c.problems, fmt.Sprintf("%s: %s exited with status %d", failed, command.Args[0], result.ExitCode))
	case closeErr != nil:
		c.problems = append(c.problems, fmt.Sprintf("could not write %s: %v", name, closeErr))
	default:
		return name
	}

	return ""
}

// write writes text to the file name, and reports whether it could.
func (c *collector) write(name, text string) bool {
	file, err := c.files.Create(name)
	if err == nil {
		_, err = io.WriteString(file, text)
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
	}

	if err != nil {
		c.problems = append(c.problems, fmt.Sprintf("could not write %s: %v", name, err))

		return false
	}

	return true
}

// summaryText returns the summary of the report of failure, whose packages
// are those collected and problems what could not be.
func summaryText(failure Failure, packages []collected, problems []string) string {
	var text strings.Builder

	text.WriteString("The failure report of an emergup run.\n\n")
	fmt.Fprintf(&text, "Step:     %s (%s)\n", failure.Step, failure.Title)
	fmt.Fprintf(&text, "Failure:  %s\n", failure.Reason)

	if failure.RanAny {
		command := strings.Join(failure.Ran.Command.Args, " ")
		if failure.Ran.Command.Root {
			command += ", as root"
		}

		fmt.Fprintf(&text, "Command:  %s\n", command)

		if failure.Ran.Err != nil {
			fmt.Fprintf(&text, "Stopped:  %v\n", failure.Ran.Err)
		}
	} else {
		text.WriteString("Command:  none: the step failed before it ran one\n")
	}

	fmt.Fprintf(&text, "Run log:  %s\n", failure.Log)

	if failure.RanAny {
		writePackages(&text, packages)
		text.WriteString("\nThe end of the command's output, its last 1000 lines at most, without colours,\n" +
			"is in " + outputFile + "; the run log has all of it.\n")
	}

	if len(problems) > 0 {
		text.WriteString("\nNot collected:\n")

		for _, problem := range problems {
			fmt.Fprintf(&text, "  %s\n", problem)
		}
	}

	return text.String()
}

// writePackages writes to text what the report has of each package.
func writePackages(text *strings.Builder, packages []collected) {
	if len(packages) == 0 {
		text.WriteString("\nemerge named no package that failed.\n")

		return
	}

	text.WriteString("\nThe packages that failed:\n")

	for _, files := range packages {
		pkg := files.pkg

		heading := pkg.CPV
		if pkg.Repository != "" {
			heading += "::" + pkg.Repository
		}

		if pkg.Phase != "" {
			heading += ", in its " + pkg.Phase + " phase"
		}

		fmt.Fprintf(text, "  %s\n", heading)

		switch {
		case pkg.Log == "":
			text.WriteString("    build log      not named by emerge\n")
		case files.buildLog == "":
			fmt.Fprintf(text, "    build log      %s, not copied\n", pkg.Log)
		default:
			fmt.Fprintf(text, "    build log      %s, copied to %s\n", pkg.Log, files.buildLog)
		}

		fmt.Fprintf(text, "    emerge --info  %s\n", orNotCollected(files.info))
		fmt.Fprintf(text, "    emerge -pqv    %s\n", orNotCollected(files.pqv))
	}
}

func orNotCollected(name string) string {
	if name == "" {
		return "not collected"
	}

	return name
}

// fileName returns the start of the names of the files of the package cpv:
// app-misc/foo-1.2 gives app-misc_foo-1.2. Anything but letters, digits and
// ".+-_" becomes "_", so that no name leaves the report's directory.
func fileName(cpv string) string {
	return strings.Map(func(char rune) rune {
		isLetterOrDigit := unicode.IsLetter(char) || unicode.IsDigit(char)
		if char < unicode.MaxASCII && (isLetterOrDigit || strings.ContainsRune(".+-_", char)) {
			return char
		}

		return '_'
	}, cpv)
}

// compression returns the suffix of log when Portage compressed it, "" when
// it did not.
func compression(log string) string {
	for _, suffix := range compressions {
		if strings.HasSuffix(log, suffix) {
			return suffix
		}
	}

	return ""
}
