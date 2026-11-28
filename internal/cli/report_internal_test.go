// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/emergup/internal/ai"
	"github.com/OrbintSoft/emergup/internal/pipeline"
	"github.com/OrbintSoft/emergup/internal/report"
	"github.com/OrbintSoft/emergup/internal/runner"
	"github.com/OrbintSoft/emergup/internal/runner/runnertest"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// TestAStepStartingForgetsTheCommandsBeforeIt keeps the report of a step
// that fails before it runs a command of its own from naming one of the
// step or the hooks before it.
func TestAStepStartingForgetsTheCommandsBeforeIt(t *testing.T) {
	t.Parallel()

	var fake runnertest.Fake

	failures := &failureReport{recorder: &report.Recorder{Next: &fake}}

	_, err := failures.recorder.Run(t.Context(), runner.Command{Args: []string{"/usr/local/bin/check"}})
	require.NoError(t, err)

	failures.Started(nil)

	_, ran := failures.recorder.Last()
	assert.False(t, ran)
}

// TestAFailureWithoutAReportIsExplainedFromItsStep keeps the explanation
// of a failure whose report could not be written from using the report of
// an earlier failure.
func TestAFailureWithoutAReportIsExplainedFromItsStep(t *testing.T) {
	t.Parallel()

	var fake runnertest.Fake

	failures := &failureReport{
		recorder: &report.Recorder{Next: &fake},
		open:     func(string) (ReportDir, error) { return nil, errNoPlace },
		printer:  ui.Printer{Out: &bytes.Buffer{}},
		kept:     map[string]*bytes.Buffer{report.SummaryFile: bytes.NewBufferString("an earlier failure\n")},
	}

	step := namedStep{name: "update", title: "update @world"}
	failed := pipeline.StepReport{Name: step.name, Reason: "emerge exited with status 1"}
	failures.Failed(t.Context(), step, failed)

	assert.Equal(t, []ai.Part{{
		Title: "The failed step",
		Text:  "Step:     update (update @world)\nFailure:  emerge exited with status 1\n",
	}}, failures.material(step, failed))
}

// errNoPlace is the error of a report directory that cannot be made.
var errNoPlace = errors.New("permission denied")

// namedStep is a pipeline.Step with a name and a title only.
type namedStep struct {
	pipeline.Step

	name, title string
}

func (s namedStep) Name() string  { return s.name }
func (s namedStep) Title() string { return s.title }
