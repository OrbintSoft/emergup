// SPDX-License-Identifier: GPL-3.0-or-later

// Package cli is the emergup command line: subcommand dispatch, argument
// checking and the usage text. It is a package rather than the main package so
// that all of it can be exercised from tests; cmd/emergup holds nothing but the
// process's os.Exit.
package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/OrbintSoft/emergup/internal/config"
	"github.com/OrbintSoft/emergup/internal/elevation"
	"github.com/OrbintSoft/emergup/internal/lock"
	"github.com/OrbintSoft/emergup/internal/pipeline"
	"github.com/OrbintSoft/emergup/internal/report"
	"github.com/OrbintSoft/emergup/internal/rundir"
	"github.com/OrbintSoft/emergup/internal/runner"
	"github.com/OrbintSoft/emergup/internal/steps"
	"github.com/OrbintSoft/emergup/internal/ui"
	"github.com/OrbintSoft/emergup/internal/version"
)

// Exit statuses.
const (
	exitOK      = 0
	exitProblem = 1
	exitUsage   = 2
	// exitStopped is 128 plus the number of SIGINT, as a shell reports a
	// command that Ctrl-C ended.
	exitStopped = 130
)

// runningAsRoot is the notice of a run started as root.
const runningAsRoot = "Running as root: the commands run without sudo, doas or run0, " +
	"and AI assistance is off."

// usage is the help text; %[1]s is the name the program was invoked as.
const usage = `usage: %[1]s <command>

Drive the routine Gentoo update workflow.

commands:
  run            update the system, step by step
    -y, --yes      take the usual answer to every question
    -p, --pretend  show the steps and what they would run, and run nothing
    --force-sync   sync every repository, however recently it was synced
  news           list the unread news items and offer to read them
    --all          list every news item, those read too, and ask nothing
  config check   validate the configuration files and report each problem
  config show    print the configuration in effect and where each value comes from
  version        print the emergup version (also --version)
  help           show this help (also -h, --help)
`

// Env is what an invocation reads and writes besides its arguments, given
// from outside so that every path is testable without touching the real
// standard streams or files.
type Env struct {
	Stdout io.Writer
	Stderr io.Writer
	// Root is the system's root file system, from which the configuration is
	// read.
	Root fs.FS
	// Keyboard reads the answers to the questions.
	Keyboard ui.Keyboard
	// NewRunner returns the runner of the commands of the steps, which
	// copies them and their output to log, nil for none.
	NewRunner func(log io.Writer) runner.Runner
	// Runs makes the run directories; System is what the elevation looks
	// at.
	Runs   RunStore
	System elevation.System
	// Lock keeps two runs from overlapping.
	Lock lock.Locker
	// Color turns emergup's colours on, see ui.ColorEnabled.
	Color bool
	// Now gives the time; nil means time.Now.
	Now func() time.Time
	// OpenReport makes the directory of a failure report at path, which
	// must not exist yet, and opens it; nil means report.OpenDir.
	OpenReport func(path string) (ReportDir, error)
}

// ReportDir is the directory of a failure report, open to write its files.
type ReportDir interface {
	report.Files
	Close() error
}

// reportDirName is the directory of the first failure report of a run, in
// the run directory.
const reportDirName = "failure"

// Main runs one invocation and returns the process exit status.
func Main(ctx context.Context, env Env, invokedAs string, args []string) int {
	stdout, stderr := env.Stdout, env.Stderr
	prog := filepath.Base(invokedAs)

	if len(args) == 0 {
		printUsage(stderr, prog)

		return exitUsage
	}

	command, rest := args[0], args[1:]
	switch command {
	case "version", "--version":
		if len(rest) > 0 {
			return unexpectedArgument(stderr, prog, command, rest[0])
		}

		_, _ = fmt.Fprintf(stdout, "emergup %s\n", version.String())

		return exitOK
	case "help", "-h", "--help":
		if len(rest) > 0 {
			return unexpectedArgument(stderr, prog, command, rest[0])
		}

		printUsage(stdout, prog)

		return exitOK
	case "run":
		return runCommand(ctx, env, prog, rest)
	case "news":
		return newsCommand(ctx, env, prog, rest)
	case "config":
		return configCommand(ctx, env, prog, rest)
	default:
		return unknownCommand(stderr, prog, command)
	}
}

// RunStore makes the directories of the runs, each with its log.
type RunStore interface {
	// Create makes the directory of a run that starts at now, and opens its
	// log.
	Create(now time.Time) (rundir.Run, error)
	// Prune removes the oldest run directories, so that keep of them are
	// left, current among them; 0 keeps them all.
	Prune(keep int, current string) error
}

// runOptions are the options of `run`.
type runOptions struct {
	yes       bool
	pretend   bool
	forceSync bool
}

// runCommand runs `run`: the built-in steps, one after the other, or with
// --pretend what they would do.
func runCommand(ctx context.Context, env Env, prog string, args []string) int {
	options, valid := parseRunOptions(env.Stderr, prog, args)
	if !valid {
		return exitUsage
	}

	cfg, err := config.Load(ctx, env.Root)
	if err != nil {
		return reportProblems(env.Stderr, err)
	}

	printer := ui.Printer{Out: env.Stdout, Colors: cfg.Colors, Color: env.Color, Log: nil}

	if options.pretend {
		return pretend(ctx, env, cfg, printer, options)
	}

	return runSteps(ctx, env, cfg, printer, options)
}

// pretend shows what a run would do, without the lock or a run directory:
// it changes nothing.
func pretend(ctx context.Context, env Env, cfg config.Config, printer ui.Printer, options runOptions) int {
	if elevation.IsRoot(env.System) {
		printer.Notice(runningAsRoot)
	}

	program, err := elevation.Resolve(cfg.Elevation.Method, env.System)
	if err != nil {
		printer.Notice(err.Error() + "; a run would stop before its first step")
	}

	// --pretend runs nothing as root, so there is nothing to keep.
	stepsEnv := newStepsEnv(env, printer, program, nil, nil, false)
	stepsEnv.Hooks = steps.Hooks(cfg.Hooks)

	sayWhatDoasMustKeep(printer, program, cfg.Elevation.KeepEnv, stepsEnv.Hooks)

	return exitStatus(pipeline.Pretend(ctx, stepsEnv, stepsOf(cfg, options)))
}

// runSteps runs the built-in steps while it holds the lock, with a log in
// a run directory of their own.
func runSteps(ctx context.Context, env Env, cfg config.Config, printer ui.Printer, options runOptions) int {
	release, err := env.Lock.Lock(ctx)
	if err != nil {
		printer.Failure(err.Error())

		return exitProblem
	}

	// The kernel frees the lock when emergup ends anyway.
	defer func() { _ = release() }()

	run, err := env.Runs.Create(env.now()())
	if err != nil {
		printer.Failure(err.Error())

		return exitProblem
	}

	// The log is best effort, as the copy the runner writes to it.
	defer func() { _ = run.Log.Close() }()

	printer.Log = run.Log

	// Every run that has a directory ends by saying where its log is.
	defer printer.Info("The log is in " + run.LogPath)

	if err := env.Runs.Prune(cfg.Runs.Keep, run.Dir); err != nil {
		printer.Notice("could not remove the oldest run directories: " + err.Error())
	}

	if elevation.IsRoot(env.System) {
		printer.Notice(runningAsRoot)
	}

	// Without a way to run the commands that need root, a run stops before
	// its first step rather than halfway through.
	program, err := elevation.Resolve(cfg.Elevation.Method, env.System)
	if err != nil {
		printer.Failure(err.Error())

		return exitProblem
	}

	stepsEnv := newStepsEnv(env, printer, program, cfg.Elevation.KeepEnv, run.Log, options.yes)
	stepsEnv.Hooks = steps.Hooks(cfg.Hooks)
	stepsEnv.HookEnv = pipeline.RunDirEnv(run.Dir, run.LogPath)

	failures := newFailureReport(env, stepsEnv.Runner, run, printer)
	stepsEnv.Runner = failures.recorder
	stepsEnv.Observer = failures
	stepsEnv.ExplainFailure = failureExplainer(cfg.AI, env, stepsEnv, failures)

	sayWhatDoasMustKeep(printer, program, cfg.Elevation.KeepEnv, stepsEnv.Hooks)

	result := pipeline.Run(ctx, stepsEnv, stepsOf(cfg, options)).Result

	for _, dir := range failures.written {
		printer.Info("The failure report is in " + dir)
	}

	return exitStatus(result)
}

// newFailureReport returns the failure report of run, whose steps run
// their commands with next. Every command goes through its recorder, so
// that the report of a step that fails has its command and output; the
// report's own queries run through it too, as the steps' commands do.
func newFailureReport(env Env, next runner.Runner, run rundir.Run, printer ui.Printer) *failureReport {
	return &failureReport{
		recorder: &report.Recorder{Next: next},
		open:     env.openReport(),
		dir:      filepath.Join(run.Dir, reportDirName),
		log:      run.LogPath,
		printer:  printer,
		failures: 0,
		written:  nil,
		kept:     nil,
	}
}

// failureReport writes a failure report for each failure of a step of a
// run: the first in dir, the next ones in dir-2, dir-3 and so on. It is the
// run's pipeline.StepObserver.
type failureReport struct {
	recorder *report.Recorder
	open     func(path string) (ReportDir, error)
	// dir is where the first report goes, log the path of the run's log.
	dir string
	log string
	// printer says what a report could not have.
	printer ui.Printer
	// failures counts the failures; written are the directories of the
	// reports written, in order.
	failures int
	written  []string
	// kept holds the files of the last report that an explanation of its
	// failure reads, by name; none when it could not be written.
	kept map[string]*bytes.Buffer
}

// Started implements pipeline.StepObserver: what ran before the step, its
// hooks before it included, is not the step's own.
func (f *failureReport) Started(pipeline.Step) {
	f.recorder.Forget()
}

// Failed implements pipeline.StepObserver: it writes the report of step,
// whose last command, if it ran one, is the recorder's last.
func (f *failureReport) Failed(ctx context.Context, step pipeline.Step, stepReport pipeline.StepReport) {
	f.failures++

	path := f.dir
	if f.failures > 1 {
		path = f.dir + "-" + strconv.Itoa(f.failures)
	}

	f.kept = nil

	dir, err := f.open(path)
	if err != nil {
		f.printer.Notice("no failure report: " + err.Error())

		return
	}

	ran, ranAny := f.recorder.Last()
	kept := map[string]*bytes.Buffer{}

	problems := report.Write(ctx, keepingFiles{Files: dir, kept: kept}, f.recorder, report.Failure{
		Step: step.Name(), Title: step.Title(), Reason: stepReport.Reason, Ran: ran, RanAny: ranAny, Log: f.log,
	})

	if err := dir.Close(); err != nil {
		problems = append(problems, err.Error())
	}

	for _, problem := range problems {
		f.printer.Notice("not in the failure report: " + problem)
	}

	f.written = append(f.written, path)
	f.kept = kept
}

// sayWhatDoasMustKeep says, when program is doas, which variables doas.conf
// must keep for the commands run as root: those of keep and, when one of
// hooks runs as root, its own; doas has no option to keep them itself.
func sayWhatDoasMustKeep(printer ui.Printer, program string, keep []string, hooks []pipeline.Hook) {
	if program != config.ElevationDoas.String() {
		return
	}

	names := slices.Clone(keep)
	if slices.ContainsFunc(hooks, func(hook pipeline.Hook) bool { return hook.Root || hook.RemedyRoot }) {
		names = append(names, pipeline.HookVariables()...)
	}

	if len(names) > 0 {
		printer.Notice("doas keeps only the variables doas.conf keeps: give its rule setenv { " +
			strings.Join(names, " ") + " } for the commands run as root to get them")
	}
}

// stepsOf returns the steps of the run, the built-in ones and the custom
// ones, as cfg and the options of the run set them up.
func stepsOf(cfg config.Config, options runOptions) []pipeline.Step {
	return steps.All(cfg, steps.Options{ForceSync: options.forceSync})
}

// newStepsEnv returns what the steps work with: commands elevated with
// program, keeping the variables keep, and copied to log, nil for none, and
// printer for the lines and the questions; the caller adds the hooks.
func newStepsEnv(env Env, printer ui.Printer, program string, keep []string, log io.Writer, yes bool) pipeline.Env {
	return pipeline.Env{
		Runner: elevation.Runner{
			Next: env.NewRunner(log), Program: program, Keep: keep, LookupEnv: env.System.LookupEnv,
		},
		Printer:   printer,
		Menu:      ui.Menu{Printer: printer, Keyboard: env.Keyboard, Yes: yes},
		Now:       env.now(),
		Root:      env.Root,
		LookPath:  env.System.LookPath,
		LookupEnv: env.System.LookupEnv,
		CanWrite:  env.System.CanWrite,
		Hooks:     nil,
		HookEnv:   nil,
		Observer:  nil,

		ExplainFailure: nil,
	}
}

// parseRunOptions reads the options of `run`; on a mistake it writes it with
// the usage and returns false.
func parseRunOptions(stderr io.Writer, prog string, args []string) (runOptions, bool) {
	options := runOptions{yes: false, pretend: false, forceSync: false}

	for _, arg := range args {
		switch arg {
		case "-y", "--yes":
			options.yes = true
		case "-p", "--pretend":
			options.pretend = true
		case "--force-sync":
			options.forceSync = true
		default:
			if strings.HasPrefix(arg, "-") {
				unknownOption(stderr, prog, "run", arg)
			} else {
				unexpectedArgument(stderr, prog, "run", arg)
			}

			return options, false
		}
	}

	return options, true
}

// exitStatus returns the exit status of a run that ended with result.
func exitStatus(result pipeline.Result) int {
	switch result {
	case pipeline.RunFinished:
		return exitOK
	case pipeline.RunFailed:
		return exitProblem
	case pipeline.RunStopped:
		return exitStopped
	}

	return exitProblem
}

// openReport returns how env opens the directory of a failure report.
func (env Env) openReport() func(path string) (ReportDir, error) {
	if env.OpenReport != nil {
		return env.OpenReport
	}

	return func(path string) (ReportDir, error) { return report.OpenDir(path) }
}

// now returns env's clock.
func (env Env) now() func() time.Time {
	if env.Now != nil {
		return env.Now
	}

	return time.Now
}

// newsCommand runs `news`: the unread news items, read if the user says
// so, or with --all every item. It keeps no log and takes no lock: it
// changes only Portage's record of the news read.
func newsCommand(ctx context.Context, env Env, prog string, args []string) int {
	all, valid := parseNewsOptions(env.Stderr, prog, args)
	if !valid {
		return exitUsage
	}

	cfg, err := config.Load(ctx, env.Root)
	if err != nil {
		return reportProblems(env.Stderr, err)
	}

	printer := ui.Printer{Out: env.Stdout, Colors: cfg.Colors, Color: env.Color, Log: nil}

	if elevation.IsRoot(env.System) {
		printer.Notice(runningAsRoot)
	}

	// Only the read can need root, and it runs through the elevated runner:
	// the commands of this environment run as they are.
	stepsEnv := newStepsEnv(env, printer, "", nil, nil, false)

	outcome := steps.News(ctx, stepsEnv, steps.NewsOptions{
		Config: cfg.Steps.News,
		All:    all,
		Elevated: func() (runner.Runner, error) {
			return newsElevation(env, printer, cfg.Elevation)
		},
	})

	switch outcome.Status {
	case pipeline.Skipped:
		printer.Skip(outcome.Reason)
	case pipeline.Failed:
		printer.Failure(outcome.Reason)

		return exitProblem
	case pipeline.Stopped:
		return exitStopped
	case pipeline.NotRun, pipeline.Succeeded, pipeline.Disabled:
	}

	return exitOK
}

// newsElevation returns the runner of the read of the news as root, through
// the method of settings, or why there is none. `news` asks for it only
// when the read needs root, so that a user who needs none can read without
// an elevation tool.
func newsElevation(env Env, printer ui.Printer, settings config.Elevation) (elevation.Runner, error) {
	program, err := elevation.Resolve(settings.Method, env.System)
	if err != nil {
		return elevation.Runner{}, fmt.Errorf("marking them read needs root: %w", err)
	}

	sayWhatDoasMustKeep(printer, program, settings.KeepEnv, nil)

	return elevation.Runner{
		Next: env.NewRunner(nil), Program: program, Keep: settings.KeepEnv, LookupEnv: env.System.LookupEnv,
	}, nil
}

// parseNewsOptions reads the options of `news` and reports whether --all is
// given; on a mistake it writes it with the usage and returns false.
func parseNewsOptions(stderr io.Writer, prog string, args []string) (bool, bool) {
	all := false

	for _, arg := range args {
		switch {
		case arg == "--all":
			all = true
		case strings.HasPrefix(arg, "-"):
			unknownOption(stderr, prog, "news", arg)

			return all, false
		default:
			unexpectedArgument(stderr, prog, "news", arg)

			return all, false
		}
	}

	return all, true
}

// configCommand runs `config check` or `config show`.
func configCommand(ctx context.Context, env Env, prog string, args []string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintf(env.Stderr, "%s config: missing subcommand, expected check or show\n\n", prog)
		printUsage(env.Stderr, prog)

		return exitUsage
	}

	subcommand, rest := args[0], args[1:]
	if subcommand != "check" && subcommand != "show" {
		_, _ = fmt.Fprintf(env.Stderr, "%s config: unknown subcommand %q\n\n", prog, subcommand)
		printUsage(env.Stderr, prog)

		return exitUsage
	}

	if len(rest) > 0 {
		return unexpectedArgument(env.Stderr, prog, "config "+subcommand, rest[0])
	}

	cfg, origins, err := config.Inspect(ctx, env.Root)
	if err != nil {
		return reportProblems(env.Stderr, err)
	}

	if subcommand == "show" {
		err := config.WriteTOML(env.Stdout, cfg, origins)
		if err != nil {
			_, _ = fmt.Fprintf(env.Stderr, "%s config show: %v\n", prog, err)

			return exitProblem
		}

		return exitOK
	}

	if len(origins.Files) == 0 {
		_, _ = fmt.Fprintln(env.Stdout, "No configuration file: the generic defaults apply.")
	}

	for _, file := range origins.Files {
		_, _ = fmt.Fprintf(env.Stdout, "Read %s\n", file)
	}

	_, _ = fmt.Fprintln(env.Stdout, "The configuration is valid.")

	return exitOK
}

// reportProblems writes each problem that err joins on its own line, then how
// many there are.
func reportProblems(stderr io.Writer, err error) int {
	problems := []error{err}

	var joined interface{ Unwrap() []error }
	if errors.As(err, &joined) {
		problems = joined.Unwrap()
	}

	for _, problem := range problems {
		_, _ = fmt.Fprintln(stderr, problem)
	}

	noun := "problems"
	if len(problems) == 1 {
		noun = "problem"
	}

	_, _ = fmt.Fprintf(stderr, "%d %s in the configuration.\n", len(problems), noun)

	return exitProblem
}

func unknownCommand(stderr io.Writer, prog, command string) int {
	kind := "command"
	if strings.HasPrefix(command, "-") {
		kind = "option"
	}

	_, _ = fmt.Fprintf(stderr, "%s: unknown %s %q\n\n", prog, kind, command)
	printUsage(stderr, prog)

	return exitUsage
}

func unknownOption(stderr io.Writer, prog, command, option string) {
	_, _ = fmt.Fprintf(stderr, "%s %s: unknown option %q\n\n", prog, command, option)
	printUsage(stderr, prog)
}

func unexpectedArgument(stderr io.Writer, prog, command, arg string) int {
	_, _ = fmt.Fprintf(stderr, "%s %s: unexpected argument %q\n\n", prog, strings.TrimLeft(command, "-"), arg)
	printUsage(stderr, prog)

	return exitUsage
}

func printUsage(w io.Writer, prog string) {
	_, _ = fmt.Fprintf(w, usage, prog)
}
