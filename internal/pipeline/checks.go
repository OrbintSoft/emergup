// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/OrbintSoft/emergup/internal/ui"
)

// The questions a check asks when it fails.
const (
	remedyQuestion = "Run the remedy and check again?"
	abortQuestion  = "Abort the run?"
)

// afterRemedy is what the reason of a check adds when it fails again after
// its remedy, and --yes does not let it ask again.
const afterRemedy = ", after its remedy"

// The keys of the answers to the question of a check.
const (
	remedyKey = 'y'
	goOnKey   = 'g'
	abortKey  = 'a'
)

// runCheck runs the check hook, and while it fails asks whether to run its
// remedy and check again, to go on, or to abort the run, recording in
// report how it failed, unless it passed in the end, and returns whether
// the run goes on. A failure the remedy is not for, see offersRemedy, only
// asks whether to go on or abort. With --yes the remedy runs once: a check that fails
// again fails the run. Aborting fails the run; the end of ctx, or of the
// input, stops it.
func runCheck(ctx context.Context, env Env, hook Hook, extra []string, report *Report) Result {
	remedied := false

	for {
		end, stopped := runHookCommand(ctx, env, hook, extra)
		reason := end.reason

		switch {
		case stopped:
			return RunStopped
		case reason == "":
			return RunFinished
		case remedied && env.Menu.Yes:
			env.Printer.Failure(hookFailure(hook, reason+afterRemedy))
			recordFailure(report, hook, reason+afterRemedy, false)

			return RunFailed
		}

		env.Printer.Failure(hookFailure(hook, reason))

		offers := hook.offersRemedy(end.status)
		if len(hook.Remedy) > 0 && !offers {
			env.Printer.Notice(fmt.Sprintf("the remedy of hook %s is only for exit status %s",
				hook.Name, statusList(hook.RemedyStatus)))
		}

		if result, done := askAboutCheck(ctx, env, hook, reason, offers, report); done {
			return result
		}

		if runRemedy(ctx, env, hook, extra) {
			return RunStopped
		}

		remedied = true
	}
}

// askAboutCheck asks what to do about the check hook, which failed for
// reason, offering its remedy when offers is set, records in report how it
// failed unless the remedy is to run, and returns whether the run goes on
// and whether the check is done with: false when its remedy is to run.
func askAboutCheck(ctx context.Context, env Env, hook Hook, reason string, offers bool, report *Report) (Result, bool) {
	key, result := askCheck(ctx, env, offers)

	switch {
	case result == RunStopped:
		return RunStopped, true
	case result == RunFailed, key == abortKey:
		recordFailure(report, hook, reason, false)

		return RunFailed, true
	case key == goOnKey:
		recordFailure(report, hook, reason, true)

		return RunFinished, true
	}

	return RunFinished, false
}

// askCheck asks what to do about a failed check, offering its remedy when
// offers is set, and returns the key of the action chosen and whether the
// run goes on: RunFinished when it does.
func askCheck(ctx context.Context, env Env, offers bool) (rune, Result) {
	remedy := ui.Action{Key: remedyKey, Name: "run the remedy and check again"}
	goOn := ui.Action{Key: goOnKey, Name: "go on without it"}
	abort := ui.Action{Key: abortKey, Name: "abort the run"}

	question, actions := abortQuestion, []ui.Action{abort, goOn}
	if offers {
		question, actions = remedyQuestion, []ui.Action{remedy, goOn, abort}
	}

	action, err := env.Menu.Ask(ctx, question, actions...)

	switch {
	case errors.Is(err, ui.ErrAborted), ctx.Err() != nil:
		return action.Key, RunStopped
	case err != nil:
		env.Printer.Failure(err.Error())

		return action.Key, RunFailed
	}

	return action.Key, RunFinished
}

// offersRemedy reports whether the check offers its remedy when its command
// ends with status, 0 for a failure without an exit status: for any failure
// unless the check lists the statuses its remedy is for.
func (h Hook) offersRemedy(status int) bool {
	if len(h.Remedy) == 0 {
		return false
	}

	return len(h.RemedyStatus) == 0 || slices.Contains(h.RemedyStatus, status)
}

// statusList writes exit statuses as a phrase: "1", "1 or 2", "1, 2 or 3".
func statusList(statuses []int) string {
	written := make([]string, len(statuses))
	for i, status := range statuses {
		written[i] = strconv.Itoa(status)
	}

	last := len(written) - 1
	if last == 0 {
		return written[0]
	}

	return strings.Join(written[:last], ", ") + " or " + written[last]
}

// runRemedy runs the remedy of hook, saying so and how it failed, if it
// did, and returns whether ctx ended while it ran. The check runs again
// whether the remedy succeeded or not.
func runRemedy(ctx context.Context, env Env, hook Hook, extra []string) bool {
	env.Printer.Info(fmt.Sprintf("remedy of hook %s (%s)%s: %s",
		hook.Name, hook.place(), asRoot(hook.RemedyRoot), strings.Join(hook.Remedy, " ")))

	end, stopped := runAsHook(ctx, env, hook, hook.Remedy, hook.RemedyRoot, extra)
	if stopped {
		return true
	}

	if end.reason != "" {
		env.Printer.Failure(fmt.Sprintf("the remedy of hook %s failed: %s", hook.Name, end.reason))
	}

	return false
}
