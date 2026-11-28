// SPDX-License-Identifier: GPL-3.0-or-later

// Package pipeline runs the steps of an update one after the other: it
// announces each, asks whether to run it, reports how it ended and how long
// it took, and closes the run with a summary.
package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"time"

	"github.com/OrbintSoft/emergup/internal/runner"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// Status is how a step ended in a run.
type Status int

// The statuses.
const (
	// NotRun: the run ended before the step.
	NotRun Status = iota
	Succeeded
	Skipped
	Failed
	// Stopped: the run was interrupted while the step ran.
	Stopped
	// Disabled: the configuration turns the step off.
	Disabled
)

// String returns the status as the summary writes it.
func (s Status) String() string {
	switch s {
	case NotRun:
		return "not run"
	case Succeeded:
		return "succeeded"
	case Skipped:
		return "skipped"
	case Failed:
		return "failed"
	case Stopped:
		return "stopped"
	case Disabled:
		return "disabled"
	}

	return fmt.Sprintf("Status(%d)", int(s))
}

// Outcome is how a step's Run ended.
type Outcome struct {
	// Status is Succeeded, Skipped, Failed, or Stopped when the step ends
	// the run.
	Status Status
	// Reason says why the step was skipped or failed.
	Reason string
	// Notes are what the step has to tell at the end of the run.
	Notes []Note
}

// Note is something a step tells at the end of the run, after the
// summary: a line, and the rows under it.
type Note struct {
	Text string
	Rows []string
}

// Success is the Outcome of a step that did its work.
func Success() Outcome {
	return Outcome{Status: Succeeded, Reason: "", Notes: nil}
}

// Skip is the Outcome of a step that had nothing to do, and says why.
func Skip(reason string) Outcome {
	return Outcome{Status: Skipped, Reason: reason, Notes: nil}
}

// Abort is the Outcome of a step at whose question the user stopped the
// run, and says why.
func Abort(reason string) Outcome {
	return Outcome{Status: Stopped, Reason: reason, Notes: nil}
}

// Fail is the Outcome of a step that failed, and says why.
func Fail(reason string) Outcome {
	return Outcome{Status: Failed, Reason: reason, Notes: nil}
}

// Env is what the pipeline and its steps work with.
type Env struct {
	// Runner runs the commands, elevating those that need root.
	Runner runner.Runner
	// Printer writes emergup's own lines, Menu asks the questions.
	Printer ui.Printer
	Menu    ui.Menu
	// Now gives the time, for the durations and the steps' intervals.
	Now func() time.Time
	// Root is the system's root file system, from which the steps read
	// the state of the system, such as when a repository was synced.
	Root fs.FS
	// LookPath finds a program on PATH, as exec.LookPath does, for the
	// steps whose tool may not be installed.
	LookPath func(file string) (string, error)
	// LookupEnv looks a variable of the invoking user's environment up,
	// as os.LookupEnv does, such as the locale to read the news in.
	LookupEnv func(name string) (string, bool)
	// CanWrite reports whether the invoking user can write the file or
	// directory at the absolute path name, so that a command that writes
	// only there runs elevated only when it must.
	CanWrite func(name string) bool
	// Hooks are the hooks of the run, in the order they run at each place.
	Hooks []Hook
	// HookEnv holds the variables, NAME=value, that every hook gets, such as
	// the run directory.
	HookEnv []string
	// Observer, when set, is told how the steps of a run go, as a failure
	// report needs to know.
	Observer StepObserver
	// ExplainFailure, when set, explains a step that failed, as report
	// says, at the question after it, which then offers it; it returns an
	// error that wraps ui.ErrAborted when the user stops the run there.
	ExplainFailure func(ctx context.Context, step Step, report StepReport) error
	// ChatAboutFailure, when set, opens a chat about a step that failed, at
	// the question after it, which then offers it; its errors are as those
	// of ExplainFailure.
	ChatAboutFailure func(ctx context.Context, step Step, report StepReport) error
}

// StepObserver is told how the steps of a run go, as they go.
type StepObserver interface {
	// Started is told that step is about to run: its hooks before it have
	// run, and it is not skipped.
	Started(step Step)
	// Failed is told that step failed, as report says, as soon as it has:
	// the hooks after it have not run yet. A step stopped by an
	// interruption has not failed.
	Failed(ctx context.Context, step Step, report StepReport)
}

// Step is one step of a run.
type Step interface {
	// Name is the step's key in the configuration, such as "update".
	Name() string
	// Title says what the step does, for its heading: "update @world".
	Title() string
	// Enabled reports whether the configuration turns the step on.
	Enabled() bool
	// Pretend returns what Run would do, a line each, without changing the
	// system.
	Pretend(ctx context.Context, env Env) ([]string, error)
	// Run does the step's work. When ctx ends, the step stops as soon as it
	// can.
	Run(ctx context.Context, env Env) Outcome
}

// SelfConfirming is a Step that asks the user itself, once it knows what
// it would do, such as an update after its --pretend: the pipeline does
// not ask its own question before it.
type SelfConfirming interface {
	// ConfirmsItself reports whether the step asks its own question.
	ConfirmsItself() bool
}

// MayBeUnavailable is a Step that can find, once the run reaches it, that it
// cannot run, such as an optional step whose program is not installed: the
// pipeline then skips it without asking whether to run it, and says why.
type MayBeUnavailable interface {
	// Unavailable returns why the step cannot run, or "" when it can.
	Unavailable(env Env) string
}

// Result is how a run ended.
type Result int

// The results, from the best to the worst.
const (
	// RunFinished: every step succeeded, was skipped or was disabled.
	RunFinished Result = iota
	// RunFailed: a step or a hook failed, or a question could not be asked.
	RunFailed
	// RunStopped: the run was interrupted or aborted.
	RunStopped
)

// String returns the result as the hooks after the run get it.
func (r Result) String() string {
	switch r {
	case RunFinished:
		return "finished"
	case RunFailed:
		return "failed"
	case RunStopped:
		return "stopped"
	}

	return fmt.Sprintf("Result(%d)", int(r))
}

// StepReport is how one step went.
type StepReport struct {
	Name   string
	Status Status
	Reason string
	// Ran reports whether the step's Run was called; Duration is how long
	// it took.
	Ran      bool
	Duration time.Duration
	// Notes are what the step had to tell at the end of the run.
	Notes []Note
}

// Report is how a run went.
type Report struct {
	// Steps has a StepReport for every step, in order.
	Steps []StepReport
	// FailedHooks are the hooks that failed, in the order they ran.
	FailedHooks []FailedHook
	Result      Result
	Duration    time.Duration
}

// The question asked before each step.
const runQuestion = "Run this step?"

// skippedAtTheQuestion is the reason of a step skipped at its question.
const skippedAtTheQuestion = "you chose to skip it"

// The question asked after a step fails.
const againQuestion = "Run this step again?"

// Run runs the enabled steps in order, with their hooks and those of the
// run, and writes the summary. Before each step it writes the step's
// heading, runs its before hooks and asks whether to run it, then writes how
// it ended and how long it took, and runs its after hooks. After a step
// fails it asks whether to run it again, to go on with the next step or to
// end the run, which a run that answers by itself does. A failure that ends
// the run, an abort or the end of ctx ends it; the hooks after the run
// still run after a failure, not after an abort or the end of ctx.
func Run(ctx context.Context, env Env, steps []Step) Report {
	start := env.Now()
	report := Report{
		Steps: make([]StepReport, 0, len(steps)), FailedHooks: nil, Result: RunFinished, Duration: 0,
	}

	for _, step := range steps {
		status := NotRun
		if !step.Enabled() {
			status = Disabled
		}

		report.Steps = append(report.Steps, StepReport{
			Name: step.Name(), Status: status, Reason: "", Ran: false, Duration: 0, Notes: nil,
		})
	}

	report.Result = runHooks(ctx, env, Before, RunStep, nil, &report)
	if report.Result == RunFinished {
		report.Result = runSteps(ctx, env, steps, &report)
	}

	if report.Result != RunStopped {
		after := runHooks(ctx, env, After, RunStep, []string{envResult + "=" + report.Result.String()}, &report)
		report.Result = worse(report.Result, after)
	}

	report.Duration = env.Now().Sub(start)
	writeSummary(env.Printer, report)

	return report
}

// runSteps runs the enabled steps in order, recording in report how each
// went, until one stops the run, and returns how the run goes on: a step
// that failed and was passed over fails the run once the steps are done.
func runSteps(ctx context.Context, env Env, steps []Step, report *Report) Result {
	headings := newHeadings(steps)
	passedOver := false

	for index, step := range steps {
		if !step.Enabled() {
			continue
		}

		if ctx.Err() != nil {
			return RunStopped
		}

		result := runStep(ctx, env, step, headings.next(step), &report.Steps[index], report)
		if result != RunFinished {
			return result
		}

		passedOver = passedOver || report.Steps[index].Status == Failed
	}

	if passedOver {
		return RunFailed
	}

	return RunFinished
}

// runStep writes heading, runs the before hooks of step, asks whether to
// run it, runs it and its after hooks, recording in stepReport how it went
// and in report the hooks that failed, and returns whether the run goes on:
// RunFinished when it does. A step whose before hooks fail does not run,
// and runs no after hook.
func runStep(ctx context.Context, env Env, step Step, heading string, stepReport *StepReport, report *Report) Result {
	env.Printer.Heading(heading)

	result := runHooks(ctx, env, Before, step.Name(), nil, report)
	if result != RunFinished {
		return result
	}

	result = askAndRun(ctx, env, step, stepReport)
	if result == RunStopped || stepReport.Status == NotRun {
		return result
	}

	after := runHooks(ctx, env, After, step.Name(), []string{envStepStatus + "=" + stepReport.Status.String()}, report)

	return worse(result, after)
}

// askAndRun asks whether to run step and runs it, recording in report how
// it went, and returns whether the run goes on: RunFinished when it does. A
// step that cannot run is skipped without a question.
func askAndRun(ctx context.Context, env Env, step Step, report *StepReport) Result {
	if reason := unavailable(env, step); reason != "" {
		report.Status, report.Reason = Skipped, reason
		writeEnd(env.Printer, *report)

		return RunFinished
	}

	if !confirmsItself(step) {
		switch run, result := askToRun(ctx, env); {
		case result != RunFinished:
			return result
		case !run:
			report.Status, report.Reason = Skipped, skippedAtTheQuestion
			writeEnd(env.Printer, *report)

			return RunFinished
		}
	}

	return runTheStep(ctx, env, step, report)
}

// runTheStep runs step, and runs it again for as long as it fails and the
// user asks for it, recording in report how it went, with the time of all
// its runs. It returns whether the run goes on: RunFinished when it does, a
// failed step passed over included.
func runTheStep(ctx context.Context, env Env, step Step, report *StepReport) Result {
	for {
		result := runOnce(ctx, env, step, report)
		if report.Status != Failed {
			return result
		}

		if again, result := askAfterFailure(ctx, env, step, *report); !again {
			return result
		}
	}
}

// runOnce runs step once, recording in report how it ended and adding the
// time it took, and tells the observer. It returns whether the run goes on:
// RunFinished when it does.
func runOnce(ctx context.Context, env Env, step Step, report *StepReport) Result {
	if env.Observer != nil {
		env.Observer.Started(step)
	}

	begin := env.Now()
	outcome := step.Run(ctx, env)
	took := env.Now().Sub(begin)
	report.Ran, report.Duration = true, report.Duration+took
	report.Status, report.Reason, report.Notes = outcome.Status, outcome.Reason, outcome.Notes

	if ctx.Err() != nil && outcome.Status != Succeeded {
		report.Status = Stopped
	}

	end := *report
	end.Duration = took
	writeEnd(env.Printer, end)

	switch report.Status {
	case Failed:
		if env.Observer != nil {
			env.Observer.Failed(ctx, step, *report)
		}

		return RunFailed
	case Stopped:
		return RunStopped
	case NotRun, Succeeded, Skipped, Disabled:
	}

	return RunFinished
}

// confirmsItself reports whether step asks its own question.
func confirmsItself(step Step) bool {
	confirming, isConfirming := step.(SelfConfirming)

	return isConfirming && confirming.ConfirmsItself()
}

// unavailable returns why step cannot run, or "" when it can.
func unavailable(env Env, step Step) string {
	gated, mayBeUnavailable := step.(MayBeUnavailable)
	if !mayBeUnavailable {
		return ""
	}

	return gated.Unavailable(env)
}

// askToRun asks whether to run the step, and returns whether to, and
// whether the run goes on: RunFinished when it does.
func askToRun(ctx context.Context, env Env) (bool, Result) {
	run := ui.Action{Key: 'y', Name: "run the step"}
	skip := ui.Action{Key: 's', Name: "skip the step"}
	abort := ui.Action{Key: 'a', Name: "abort the run"}

	action, err := env.Menu.Ask(ctx, runQuestion, run, skip, abort)

	switch {
	case errors.Is(err, ui.ErrAborted), ctx.Err() != nil, err == nil && action == abort:
		return false, RunStopped
	case err != nil:
		env.Printer.Failure(err.Error())

		return false, RunFailed
	}

	return action == run, RunFinished
}

// askAfterFailure asks what to do after step failed, as report says, and
// returns whether to run it again and, when not, how the run goes on:
// RunFinished to go on with the next step, RunFailed to end it as a
// failure, as a run that answers by itself does, and RunStopped when the
// question is aborted. The AI actions of env are offered too, and the
// question is asked again after one.
func askAfterFailure(ctx context.Context, env Env, step Step, report StepReport) (bool, Result) {
	again := ui.Action{Key: 'r', Name: "run it again"}
	goOn := ui.Action{Key: 's', Name: "go on with the next step"}
	end := ui.Action{Key: 'a', Name: "end the run"}

	helps := failureHelps(env)

	actions := make([]ui.Action, 0, len(helps)+3) //nolint:mnd // again, goOn, end.
	actions = append(actions, again, goOn, end)

	for _, each := range helps {
		actions = append(actions, each.action)
	}

	for {
		action, err := env.Menu.Choose(ctx, againQuestion, actions...)

		switch {
		case errors.Is(err, ui.ErrNoUsualAction):
			return false, RunFailed
		case errors.Is(err, ui.ErrAborted), ctx.Err() != nil:
			return false, RunStopped
		case err != nil:
			env.Printer.Failure(err.Error())

			return false, RunFailed
		case action == again:
			return true, RunFinished
		case action == goOn:
			return false, RunFinished
		case action == end:
			return false, RunFailed
		}

		chosen := helps[slices.IndexFunc(helps, func(each aiHelp) bool { return each.action == action })]
		if !assist(ctx, env, chosen.help, step, report) {
			return false, RunStopped
		}
	}
}

// aiHelp is an AI action of the question after a failure, and what does it.
type aiHelp struct {
	action ui.Action
	help   func(ctx context.Context, step Step, report StepReport) error
}

// failureHelps returns the AI actions env offers after a failure, in the
// order of their keys.
func failureHelps(env Env) []aiHelp {
	var helps []aiHelp

	if env.ExplainFailure != nil {
		helps = append(helps, aiHelp{
			action: ui.Action{Key: 'e', Name: "explain the failure"}, help: env.ExplainFailure,
		})
	}

	if env.ChatAboutFailure != nil {
		helps = append(helps, aiHelp{
			action: ui.Action{Key: 'c', Name: "chat about the failure"}, help: env.ChatAboutFailure,
		})
	}

	return helps
}

// assist does an AI action of env, help, about step, which failed as report
// says, and returns whether the run goes on: an action that fails is said,
// and is no failure of the run, but an abort stops it.
func assist(ctx context.Context, env Env, help func(ctx context.Context, step Step, report StepReport) error,
	step Step, report StepReport,
) bool {
	err := help(ctx, step, report)

	switch {
	case errors.Is(err, ui.ErrAborted), ctx.Err() != nil:
		return false
	case err != nil:
		env.Printer.Notice(err.Error())
	}

	return true
}

// writeEnd writes how the step of report ended.
func writeEnd(printer ui.Printer, report StepReport) {
	took := ui.FormatDuration(report.Duration)

	switch report.Status {
	case Succeeded:
		printer.Success(report.Name + " succeeded in " + took)
	case Skipped:
		printer.Skip(withReason(report.Name+" skipped", report.Reason))
	case Failed:
		printer.Failure(withReason(report.Name+" failed after "+took, report.Reason))
	case Stopped:
		printer.Failure(withReason(report.Name+" stopped after "+took, report.Reason))
	case NotRun, Disabled:
	}
}

// withReason returns text followed by reason, when there is one.
func withReason(text, reason string) string {
	if reason == "" {
		return text
	}

	return text + ": " + reason
}

// headings numbers the headings of the enabled steps.
type headings struct {
	number, total int
}

func newHeadings(steps []Step) *headings {
	counter := &headings{number: 0, total: 0}

	for _, step := range steps {
		if step.Enabled() {
			counter.total++
		}
	}

	return counter
}

// next returns the heading of step, the next enabled one: "[2/6] update:
// update @world".
func (h *headings) next(step Step) string {
	h.number++

	return fmt.Sprintf("[%d/%d] %s: %s", h.number, h.total, step.Name(), step.Title())
}

// Pretend writes the heading of each enabled step and what it would do, its
// hooks and those of the run included, without asking anything or running a
// step or a hook. It returns RunFailed when a step cannot say what it would
// do.
func Pretend(ctx context.Context, env Env, steps []Step) Result {
	result := RunFinished
	headings := newHeadings(steps)

	pretendHooks(env, Before, RunStep)

	for _, step := range steps {
		if ctx.Err() != nil {
			return RunStopped
		}

		if !step.Enabled() {
			env.Printer.Skip(step.Name() + " is disabled in the configuration")

			continue
		}

		env.Printer.Heading(headings.next(step))
		pretendHooks(env, Before, step.Name())

		lines, err := step.Pretend(ctx, env)
		if err != nil {
			env.Printer.Failure(fmt.Sprintf("could not work out what %s would do: %v", step.Name(), err))

			result = RunFailed
		}

		for _, line := range lines {
			env.Printer.Row(ui.KindPlain, line)
		}

		pretendHooks(env, After, step.Name())
	}

	pretendHooks(env, After, RunStep)

	return result
}
