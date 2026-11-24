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
	// Status is Succeeded, Skipped or Failed.
	Status Status
	// Reason says why the step was skipped or failed.
	Reason string
}

// Success is the Outcome of a step that did its work.
func Success() Outcome {
	return Outcome{Status: Succeeded, Reason: ""}
}

// Skip is the Outcome of a step that had nothing to do, and says why.
func Skip(reason string) Outcome {
	return Outcome{Status: Skipped, Reason: reason}
}

// Fail is the Outcome of a step that failed, and says why.
func Fail(reason string) Outcome {
	return Outcome{Status: Failed, Reason: reason}
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

// Result is how a run ended.
type Result int

// The results.
const (
	// RunFinished: every step succeeded, was skipped or was disabled.
	RunFinished Result = iota
	// RunFailed: a step failed, or a question could not be asked.
	RunFailed
	// RunStopped: the run was interrupted or aborted.
	RunStopped
)

// StepReport is how one step went.
type StepReport struct {
	Name   string
	Status Status
	Reason string
	// Ran reports whether the step's Run was called; Duration is how long
	// it took.
	Ran      bool
	Duration time.Duration
}

// Report is how a run went.
type Report struct {
	// Steps has a StepReport for every step, in order.
	Steps    []StepReport
	Result   Result
	Duration time.Duration
}

// The question asked before each step.
const runQuestion = "Run this step?"

// skippedAtTheQuestion is the reason of a step skipped at its question.
const skippedAtTheQuestion = "you chose to skip it"

// Run runs the enabled steps in order and writes the summary. Before each
// step it writes the step's heading and asks whether to run it, then writes
// how it ended and how long it took. A failure, an abort or the end of ctx
// stops the run.
func Run(ctx context.Context, env Env, steps []Step) Report {
	start := env.Now()
	report := Report{Steps: make([]StepReport, 0, len(steps)), Result: RunFinished, Duration: 0}

	for _, step := range steps {
		status := NotRun
		if !step.Enabled() {
			status = Disabled
		}

		report.Steps = append(report.Steps, StepReport{
			Name: step.Name(), Status: status, Reason: "", Ran: false, Duration: 0,
		})
	}

	headings := newHeadings(steps)

	for index, step := range steps {
		if !step.Enabled() {
			continue
		}

		if ctx.Err() != nil {
			report.Result = RunStopped

			break
		}

		report.Result = runStep(ctx, env, step, headings.next(step), &report.Steps[index])
		if report.Result != RunFinished {
			break
		}
	}

	report.Duration = env.Now().Sub(start)
	writeSummary(env.Printer, report)

	return report
}

// runStep writes heading, asks whether to run step and runs it, recording
// in report how it went, and returns whether the run goes on: RunFinished
// when it does.
func runStep(ctx context.Context, env Env, step Step, heading string, report *StepReport) Result {
	env.Printer.Heading(heading)

	switch run, result := askToRun(ctx, env); {
	case result != RunFinished:
		return result
	case !run:
		report.Status, report.Reason = Skipped, skippedAtTheQuestion
		writeEnd(env.Printer, *report)

		return RunFinished
	}

	begin := env.Now()
	outcome := step.Run(ctx, env)
	report.Ran, report.Duration = true, env.Now().Sub(begin)
	report.Status, report.Reason = outcome.Status, outcome.Reason

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

// Pretend writes the heading of each enabled step and what it would do,
// without asking anything or running a step. It returns RunFailed when a
// step cannot say what it would do.
func Pretend(ctx context.Context, env Env, steps []Step) Result {
	result := RunFinished
	headings := newHeadings(steps)

	for _, step := range steps {
		if ctx.Err() != nil {
			return RunStopped
		}

		if !step.Enabled() {
			env.Printer.Skip(step.Name() + " is disabled in the configuration")

			continue
		}

		env.Printer.Heading(headings.next(step))

		lines, err := step.Pretend(ctx, env)
		if err != nil {
			env.Printer.Failure(fmt.Sprintf("could not work out what %s would do: %v", step.Name(), err))

			result = RunFailed

			continue
		}

		for _, line := range lines {
			env.Printer.Row(ui.KindPlain, line)
		}
	}

	return result
}
