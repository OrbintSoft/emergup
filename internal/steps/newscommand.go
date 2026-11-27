// SPDX-License-Identifier: GPL-3.0-or-later

package steps

import (
	"context"
	"errors"

	"github.com/OrbintSoft/emergup/internal/config"
	"github.com/OrbintSoft/emergup/internal/pipeline"
	"github.com/OrbintSoft/emergup/internal/runner"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// noNewsItems is what `emergup news --all` says when Portage lists no item.
const noNewsItems = "no news items"

// NewsOptions set up `emergup news`.
type NewsOptions struct {
	// Config is the news step's configuration: its command shows the
	// unread items and marks them read.
	Config config.Step
	// All lists every item, those read too, and asks nothing.
	All bool
	// Elevated returns the runner of the commands run as root, or why
	// there is none. News asks for it only when reading the items needs
	// root, before its question.
	Elevated func() (runner.Runner, error)
}

// News is `emergup news`: it lists the unread news items and asks whether
// to read them, which it does as the news step of a run does; there is no
// run to go on with afterwards. With All it lists every item and asks
// nothing.
func News(ctx context.Context, env pipeline.Env, options NewsOptions) pipeline.Outcome {
	items, err := listNews(ctx, env, options.All)
	if err != nil {
		return pipeline.Fail(err.Error())
	}

	if len(items) == 0 && options.All {
		return pipeline.Skip(noNewsItems)
	}

	if len(items) == 0 {
		return pipeline.Skip(noUnreadNews)
	}

	showNews(env, items)

	if options.All {
		return pipeline.Success()
	}

	root := newsNeedsRoot(env)

	reader, err := newsReader(env, root, options.Elevated)
	if err != nil {
		return pipeline.Fail(err.Error())
	}

	if outcome, goOn := askToReadNews(ctx, env); !goOn {
		return outcome
	}

	return readNews(ctx, reader, options.Config, root)
}

// newsReader returns the environment in which to read the items: env, with
// the runner elevated gives when reading them needs root, or why there is
// none.
func newsReader(env pipeline.Env, root bool, elevated func() (runner.Runner, error)) (pipeline.Env, error) {
	if !root {
		return env, nil
	}

	run, err := elevated()
	if err != nil {
		return env, err
	}

	env.Runner = run

	return env, nil
}

// askToReadNews asks whether to read the items and reports whether to; when
// not, it returns how `emergup news` ends: the items left unread, or the
// command stopped by Ctrl-C or the end of the input.
func askToReadNews(ctx context.Context, env pipeline.Env) (pipeline.Outcome, bool) {
	read := ui.Action{Key: 'r', Name: "read them"}
	quit := ui.Action{Key: 'q', Name: "quit, leaving them unread"}

	action, err := env.Menu.Ask(ctx, "Read them?", read, quit)

	switch {
	case errors.Is(err, ui.ErrAborted):
		return pipeline.Abort(abortedRun), false
	case err != nil:
		return pipeline.Fail(err.Error()), false
	case action == quit:
		return pipeline.Skip(newsLeftUnread), false
	}

	return pipeline.Success(), true
}
