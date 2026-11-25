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
	// Hooks are the hooks of the run, in the order they run at each place.
	Hooks []Hook
	// HookEnv holds the variables, NAME=value, that every hook gets, such as
	// the run directory.
	HookEnv []string
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

// Run runs the enabled steps in order, with their hooks and those of the
// run, and writes the summary. Before each step it writes the step's
// heading, runs its before hooks and asks whether to run it, then writes how
// it ended and how long it took, and runs its after hooks. A failure, an
// abort or the end of ctx stops the run; the hooks after the run still run
// after a failure.
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
// went, until one stops the run, and returns how the run goes on.
func runSteps(ctx context.Context, env Env, steps []Step, report *Report) Result {
	headings := newHeadings(steps)

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
// it went, and returns whether the run goes on: RunFinished when it does.
func askAndRun(ctx context.Context, env Env, step Step, report *StepReport) Result {
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

	begin := env.Now()
	outcome := step.Run(ctx, env)
	report.Ran, report.Duration = true, env.Now().Sub(begin)
	report.Status, report.Reason, report.Notes = outcome.Status, outcome.Reason, outcome.Notes

	if ctx.Err() != nil && outcome.Status != Succeeded {
		report.Status = Stopped
	}

	writeEnd(env.Printer, *report)

	switch report.Status {
	case Failed:
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
