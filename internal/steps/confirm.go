// SPDX-License-Identifier: GPL-3.0-or-later

package steps

import (
	"context"
	"errors"

	"github.com/OrbintSoft/emergup/internal/pipeline"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// abortedRun is the reason of a step at whose question the user stopped
// the run.
const abortedRun = "you aborted the run"

// confirm asks a step's own question, with goOn as the usual action, skip
// and abort the run, and reports whether to go on; when not, it returns the
// step's outcome: skipped saying skipped, or stopped. Ctrl-C and the end of
// the input stop the run, as the abort does.
func confirm(ctx context.Context, env pipeline.Env, question string, goOn, skip ui.Action,
	skipped string,
) (pipeline.Outcome, bool) {
	abort := ui.Action{Key: 'a', Name: "abort the run"}

	action, err := env.Menu.Ask(ctx, question, goOn, skip, abort)

	switch {
	case errors.Is(err, ui.ErrAborted), err == nil && action == abort:
		return pipeline.Abort(abortedRun), false
	case err != nil:
		return pipeline.Fail(err.Error()), false
	case action == skip:
		return pipeline.Skip(skipped), false
	}

	return pipeline.Success(), true
}
