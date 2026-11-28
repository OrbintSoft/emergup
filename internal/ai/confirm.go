// SPDX-License-Identifier: GPL-3.0-or-later

package ai

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/OrbintSoft/emergup/internal/ui"
)

// instructionsTitle names emergup's own instructions on the screen.
const instructionsTitle = "emergup's instructions"

// Extras are the parts the user can add to a request, each given when it
// is added; a nil one cannot be added.
type Extras struct {
	// Packages gives the installed packages.
	Packages func(ctx context.Context) (Part, error)
	// Info gives emerge --info.
	Info func(ctx context.Context) (Part, error)
}

// extra is a part the user can add, with the action that adds it.
type extra struct {
	action ui.Action
	// what names it when it cannot be added.
	what string
	give func(ctx context.Context) (Part, error)
}

// Confirm writes on menu what request holds, a line for each part with its
// size, emergup's instructions among them, and asks whether to send it to
// the backend named backend: y sends it, v shows all of it first, p and i
// add the installed packages and emerge --info, n sends nothing. It returns
// the request to send, with the parts added, and whether to send it; Ctrl-C
// or the end of the input is ui.ErrAborted.
func Confirm(ctx context.Context, menu ui.Menu, backend string, request Request, extras Extras) (Request, bool, error) {
	send := ui.Action{Key: 'y', Name: "send them"}
	view := ui.Action{Key: 'v', Name: "show all of it"}
	refuse := ui.Action{Key: 'n', Name: "send nothing"}

	addable := addableExtras(extras)
	question := "Send them to " + backend + "?"

	list(menu.Printer, backend, request)

	for {
		actions := slices.Concat([]ui.Action{send, view}, actionsOf(addable), []ui.Action{refuse})

		action, err := menu.Ask(ctx, question, actions...)

		switch {
		case err != nil:
			return request, false, fmt.Errorf("asking whether to send it: %w", err)
		case action == send:
			return request, true, nil
		case action == refuse:
			return request, false, nil
		case action == view:
			show(menu.Printer, request)

			continue
		}

		index := slices.IndexFunc(addable, func(each extra) bool { return each.action == action })
		added := addable[index]
		addable = slices.Delete(addable, index, index+1)

		part, err := added.give(ctx)
		if err != nil {
			menu.Printer.Notice("could not add " + added.what + ": " + err.Error())

			continue
		}

		request.Parts = append(request.Parts, part)
		list(menu.Printer, backend, request)
	}
}

// addableExtras returns the extras of extras that can be added, in the
// order of their keys on the screen.
func addableExtras(extras Extras) []extra {
	var addable []extra

	if extras.Packages != nil {
		addable = append(addable, extra{
			action: ui.Action{Key: 'p', Name: "add the installed packages"}, what: "the installed packages",
			give: extras.Packages,
		})
	}

	if extras.Info != nil {
		addable = append(addable, extra{
			action: ui.Action{Key: 'i', Name: "add emerge --info"}, what: "emerge --info", give: extras.Info,
		})
	}

	return addable
}

// actionsOf returns the actions that add the extras of addable.
func actionsOf(addable []extra) []ui.Action {
	actions := make([]ui.Action, 0, len(addable))
	for _, each := range addable {
		actions = append(actions, each.action)
	}

	return actions
}

// list writes with printer the parts of request, each with its size.
func list(printer ui.Printer, backend string, request Request) {
	titles := make([]string, 0, 1+len(request.Parts))
	sizes := make([]int, 0, 1+len(request.Parts))

	titles = append(titles, instructionsTitle)
	sizes = append(sizes, lineCount(request.Instructions))

	for _, part := range request.Parts {
		titles = append(titles, part.Title)
		sizes = append(sizes, lineCount(part.Text))
	}

	width := 0
	for _, title := range titles {
		width = max(width, len(title))
	}

	digits := len(strconv.Itoa(slices.Max(sizes)))

	printer.Info("To send to " + backend + ":")

	for index, title := range titles {
		printer.Row(ui.KindPlain, fmt.Sprintf("%-*s  %*d %s", width, title, digits, sizes[index], lines(sizes[index])))
	}
}

// show writes with printer all of request, as the backend would read it.
func show(printer ui.Printer, request Request) {
	for line := range strings.Lines(request.Text()) {
		printer.Row(ui.KindPlain, strings.TrimSuffix(line, "\n"))
	}
}

// lineCount returns how many lines text has, a last one without a newline
// included.
func lineCount(text string) int {
	return strings.Count(withNewline(text), "\n")
}

// lines says "line" or "lines", as count asks.
func lines(count int) string {
	if count == 1 {
		return "line"
	}

	return "lines"
}
