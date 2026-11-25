// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/OrbintSoft/emergup/internal/runner"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// When a hook runs, before or after its step.
const (
	Before = "before"
	After  = "after"
)

// RunStep is the step of a Hook that runs before or after the whole run.
const RunStep = "run"

// The variables a hook gets.
const (
	envRunDir     = "EMERGUP_RUN_DIR"
	envLog        = "EMERGUP_LOG"
	envHook       = "EMERGUP_HOOK"
	envWhen       = "EMERGUP_WHEN"
	envStep       = "EMERGUP_STEP"
	envStepStatus = "EMERGUP_STEP_STATUS"
	envResult     = "EMERGUP_RESULT"
)

// Hook is a command a run starts before or after the whole run or a step.
// It runs as the invoking user, without a question.
type Hook struct {
	// Name is the hook's name in the configuration.
	Name string
	// When is Before or After; Step is the name of a step, or RunStep.
	When string
	Step string
	// Command is the program and its arguments.
	Command []string
	// GoesOn makes a failure of the hook a notice, after which the run goes
	// on.
	GoesOn bool
}

// RunDirEnv returns the variables, for Env.HookEnv, that give every hook
// the run directory dir and the log of the run.
func RunDirEnv(dir, log string) []string {
	return []string{envRunDir + "=" + dir, envLog + "=" + log}
}

// place says where the hook runs: "before update", "after the run".
func (h Hook) place() string {
	if h.Step == RunStep {
		return h.When + " the run"
	}

	return h.When + " " + h.Step
}

// FailedHook is a hook that failed in a run.
type FailedHook struct {
	Name string
	// Place says where it ran: "before update", "after the run".
	Place  string
	Reason string
	// WentOn reports whether the run went on, as the hook is configured to.
	WentOn bool
}

// runHooks runs the hooks of env at when and step, in order, each with
// extra on top of its variables, records in report those that fail, and
// returns whether the run goes on: RunFinished when it does. A before hook
// that fails stops the hooks after it; every after hook runs, whatever an
// earlier one did. No hook starts once ctx has ended.
func runHooks(ctx context.Context, env Env, when, step string, extra []string, report *Report) Result {
	result := RunFinished

	for _, hook := range env.Hooks {
		if hook.When != when || hook.Step != step {
			continue
		}

		if ctx.Err() != nil {
			return RunStopped
		}

		switch runHook(ctx, env, hook, extra, report) {
		case RunStopped:
			return RunStopped
		case RunFailed:
			result = RunFailed

			if when == Before {
				return RunFailed
			}
		case RunFinished:
		}
	}

	return result
}

// runHook runs hook with the variables of env, its own and extra, records
// in report how it failed, if it did, and returns whether the run goes on.
func runHook(ctx context.Context, env Env, hook Hook, extra []string, report *Report) Result {
	env.Printer.Info(fmt.Sprintf("hook %s (%s): %s", hook.Name, hook.place(), strings.Join(hook.Command, " ")))

	variables := slices.Concat(env.HookEnv, []string{
		envHook + "=" + hook.Name,
		envWhen + "=" + hook.When,
		envStep + "=" + hook.Step,
	}, extra)

	command := runner.Command{Args: hook.Command, Mode: runner.Stream, Root: false, Env: variables}
	result, err := env.Runner.Run(ctx, command)

	var reason string

	switch {
	case ctx.Err() != nil:
		return RunStopped
	case err != nil:
		reason = err.Error()
	case result.ExitCode == 0:
		return RunFinished
	case result.ExitCode < 0:
		reason = "ended by a signal"
	default:
		reason = fmt.Sprintf("exit status %d", result.ExitCode)
	}

	report.FailedHooks = append(report.FailedHooks, FailedHook{
		Name: hook.Name, Place: hook.place(), Reason: reason, WentOn: hook.GoesOn,
	})

	failure := fmt.Sprintf("hook %s (%s) failed: %s", hook.Name, hook.place(), reason)

	if hook.GoesOn {
		env.Printer.Notice(failure + "; the run goes on")

		return RunFinished
	}

	env.Printer.Failure(failure)

	return RunFailed
}

// pretendHooks writes a line for each hook of env at when and step.
func pretendHooks(env Env, when, step string) {
	for _, hook := range env.Hooks {
		if hook.When == when && hook.Step == step {
			env.Printer.Row(ui.KindPlain,
				fmt.Sprintf("hook %s (%s): %s", hook.Name, hook.place(), strings.Join(hook.Command, " ")))
		}
	}
}

// worse returns the result of a run that ended earlier with result, then
// with later: a stop over a failure, a failure over the end, the order in
// which the results are declared.
func worse(result, later Result) Result {
	return max(result, later)
}
