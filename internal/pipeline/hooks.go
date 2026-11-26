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

// Hook is a command a run starts before or after the whole run or a step,
// without a question.
type Hook struct {
	// Name is the hook's name in the configuration.
	Name string
	// When is Before or After; Step is the name of a step, or RunStep.
	When string
	Step string
	// Command is the program and its arguments; Root runs it as root, and
	// it runs as the invoking user otherwise.
	Command []string
	Root    bool
	// GoesOn makes a failure of the hook a notice, after which the run goes
	// on.
	GoesOn bool
	// Asks makes the hook a check: its failure asks whether to run Remedy,
	// when there is one, and run the hook again, to go on, or to abort the
	// run. RemedyRoot runs Remedy as root.
	Asks       bool
	Remedy     []string
	RemedyRoot bool
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
	if hook.Asks {
		return runCheck(ctx, env, hook, extra, report)
	}

	reason, stopped := runHookCommand(ctx, env, hook, extra)

	switch {
	case stopped:
		return RunStopped
	case reason == "":
		return RunFinished
	}

	failure := hookFailure(hook, reason)

	if hook.GoesOn {
		env.Printer.Notice(failure + "; the run goes on")
		recordFailure(report, hook, reason, true)

		return RunFinished
	}

	env.Printer.Failure(failure)
	recordFailure(report, hook, reason, false)

	return RunFailed
}

// runHookCommand writes the line of hook and runs its command, and returns
// why it failed, "" when it did not, or whether ctx ended while it ran.
func runHookCommand(ctx context.Context, env Env, hook Hook, extra []string) (string, bool) {
	env.Printer.Info(hookLine(hook))

	return runAsHook(ctx, env, hook, hook.Command, hook.Root, extra)
}

// runAsHook runs args, as root when root is set, with the variables of env,
// those of hook and extra, and returns why it failed, "" when it did not,
// or whether ctx ended while it ran.
func runAsHook(ctx context.Context, env Env, hook Hook, args []string, root bool, extra []string) (string, bool) {
	variables := slices.Concat(env.HookEnv, []string{
		envHook + "=" + hook.Name,
		envWhen + "=" + hook.When,
		envStep + "=" + hook.Step,
	}, extra)

	command := runner.Command{Args: args, Mode: runner.Stream, Root: root, Env: variables}
	result, err := env.Runner.Run(ctx, command)

	switch {
	case ctx.Err() != nil:
		return "", true
	case err != nil:
		return err.Error(), false
	case result.ExitCode == 0:
		return "", false
	case result.ExitCode < 0:
		return "ended by a signal", false
	}

	return fmt.Sprintf("exit status %d", result.ExitCode), false
}

// hookLine says which hook runs, where and what: "hook snapshot (before
// update): snap pre", with ", as root" after the place of a hook that runs
// as root.
func hookLine(hook Hook) string {
	return fmt.Sprintf("hook %s (%s)%s: %s", hook.Name, hook.place(), asRoot(hook.Root), strings.Join(hook.Command, " "))
}

// asRoot is what a line adds for a command run as root.
func asRoot(root bool) string {
	if root {
		return ", as root"
	}

	return ""
}

// hookFailure says that hook failed, and why.
func hookFailure(hook Hook, reason string) string {
	return fmt.Sprintf("hook %s (%s) failed: %s", hook.Name, hook.place(), reason)
}

// recordFailure records in report that hook failed for reason, and whether
// the run went on.
func recordFailure(report *Report, hook Hook, reason string, wentOn bool) {
	report.FailedHooks = append(report.FailedHooks, FailedHook{
		Name: hook.Name, Place: hook.place(), Reason: reason, WentOn: wentOn,
	})
}

// pretendHooks writes a line for each hook of env at when and step, and for
// a check what it would offer when it fails.
func pretendHooks(env Env, when, step string) {
	for _, hook := range env.Hooks {
		if hook.When != when || hook.Step != step {
			continue
		}

		line := hookLine(hook)

		switch {
		case hook.Asks && len(hook.Remedy) > 0:
			line += "; if it fails, offers to run" + asRootWord(hook.RemedyRoot) + ": " + strings.Join(hook.Remedy, " ")
		case hook.Asks:
			line += "; if it fails, asks whether to go on"
		}

		env.Printer.Row(ui.KindPlain, line)
	}
}

// asRootWord is what a phrase adds for a command run as root.
func asRootWord(root bool) string {
	if root {
		return " as root"
	}

	return ""
}

// worse returns the result of a run that ended earlier with result, then
// with later: a stop over a failure, a failure over the end, the order in
// which the results are declared.
func worse(result, later Result) Result {
	return max(result, later)
}
