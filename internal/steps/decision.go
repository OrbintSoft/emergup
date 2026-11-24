// SPDX-License-Identifier: GPL-3.0-or-later

package steps

import (
	"fmt"

	"github.com/OrbintSoft/emergup/internal/ui"
)

// decision is what a step does with one of the things it works on, such as
// a repository or a package, and why.
type decision struct {
	name string
	// act is whether the step syncs, rebuilds… it.
	act    bool
	reason string
}

// chosen returns the names of the decisions to act on.
func chosen(decisions []decision) []string {
	var names []string

	for _, decision := range decisions {
		if decision.act {
			names = append(names, decision.name)
		}
	}

	return names
}

// row is the line that says what is done with one thing.
type row struct {
	kind ui.Kind
	text string
}

// rows returns a row for each decision: the name and what is done with it,
// one left as it is in the colour of a skip.
func rows(decisions []decision) []row {
	width := 0
	for _, decision := range decisions {
		width = max(width, len(decision.name))
	}

	rows := make([]row, 0, len(decisions))

	for _, decision := range decisions {
		kind := ui.KindPlain
		if !decision.act {
			kind = ui.KindSkip
		}

		rows = append(rows, row{kind: kind, text: fmt.Sprintf("%-*s  %s", width, decision.name, decision.reason)})
	}

	return rows
}
