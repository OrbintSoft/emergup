// SPDX-License-Identifier: GPL-3.0-or-later

package steps_test

import (
	"bytes"
	"os/exec"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/emergup/internal/config"
	"github.com/OrbintSoft/emergup/internal/pipeline"
	"github.com/OrbintSoft/emergup/internal/runner"
	"github.com/OrbintSoft/emergup/internal/runner/runnertest"
	"github.com/OrbintSoft/emergup/internal/steps"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// The custom step of the tests, after the preserved-library rebuild.
const (
	haskellName    = "haskell"
	haskellUpdater = "haskell-updater"
)

// haskellCommand returns the command of the custom step of the tests.
func haskellCommand() []string {
	const quiet = "--quiet"

	return []string{haskellUpdater, quiet, "--", "--verbose"}
}

// haskell returns the custom step of the tests, changed by change.
func haskell(change func(*config.CustomStep)) config.CustomStep {
	step := config.NewCustomStep()
	step.After, step.Command = preservedName, haskellCommand()

	if change != nil {
		change(&step)
	}

	return step
}

// customRun is a run of one custom step, on a system where installed are
// the programs on PATH.
type customRun struct {
	installed []string
	runner    runnertest.Fake
	out       bytes.Buffer
}

// step returns the custom step that cfg sets up, as the run has it.
//
//nolint:ireturn // The step as the pipeline sees it.
func (r *customRun) step(t *testing.T, cfg config.CustomStep) pipeline.Step {
	t.Helper()

	all := config.Default()
	all.CustomSteps = config.CustomSteps{haskellName: cfg}

	for _, step := range steps.All(all, steps.Options{ForceSync: false}) {
		if step.Name() == haskellName {
			return step
		}
	}

	t.Fatal("no custom step")

	return nil
}

func (r *customRun) env() pipeline.Env {
	printer := ui.Printer{Out: &r.out, Colors: config.Default().Colors, Color: false, Log: nil}

	return pipeline.Env{
		Runner:  &r.runner,
		Printer: printer,
		Menu:    ui.Menu{Printer: printer, Keyboard: nil, Yes: true},
		Now:     time.Now,
		Root:    nil,
		LookPath: func(file string) (string, error) {
			if slices.Contains(r.installed, file) {
				return "/usr/bin/" + file, nil
			}

			return "", exec.ErrNotFound
		},
		Hooks:   nil,
		HookEnv: nil,
	}
}

func TestAllPlacesTheCustomStepsAfterTheirBuiltInStep(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	after := func(step string) config.CustomStep {
		custom := config.NewCustomStep()
		custom.After, custom.Command = step, []string{"run-" + step}

		return custom
	}

	const offName = "off"

	off := after(updateName)
	off.Enabled = false

	cfg.CustomSteps = config.CustomSteps{
		"b-report": after(syncName), "a-report": after(syncName), "audit": after(depcleanName), offName: off,
	}

	all := steps.All(cfg, steps.Options{ForceSync: false})

	assert.Equal(t, []string{
		syncName, "a-report", "b-report", newsName, updateName, offName, liveName, preservedName, configFilesName,
		depcleanName, "audit",
	}, names(all))
	assert.False(t, all[5].Enabled(), "a custom step turned off is in the run, as disabled")
	assert.True(t, all[1].Enabled())
}

func TestAllWithoutCustomStepsIsTheBuiltInSteps(t *testing.T) {
	t.Parallel()

	assert.Equal(t, names(steps.BuiltIn(config.Default(), steps.Options{ForceSync: false})),
		names(steps.All(config.Default(), steps.Options{ForceSync: false})))
}

func TestACustomStepTitle(t *testing.T) {
	t.Parallel()

	var run customRun

	assert.Equal(t, "run haskell-updater", run.step(t, haskell(nil)).Title())
	assert.Equal(t, "rebuild the Haskell packages",
		run.step(t, haskell(func(s *config.CustomStep) { s.Title = "rebuild the Haskell packages" })).Title())
}

func TestACustomStepRunsItsCommand(t *testing.T) {
	t.Parallel()

	for _, elevated := range []bool{false, true} {
		var run customRun

		step := run.step(t, haskell(func(s *config.CustomStep) { s.Elevated = elevated }))

		assert.Equal(t, pipeline.Success(), step.Run(t.Context(), run.env()))
		assert.Equal(t, []runner.Command{{Args: haskellCommand(), Mode: runner.Stream, Root: elevated, Env: nil}},
			run.runner.Commands(), "elevated: %v", elevated)
	}
}

func TestACustomStepWhoseCommandFailsFails(t *testing.T) {
	t.Parallel()

	var run customRun

	run.runner.Script(runnertest.Answer{Result: runner.Result{ExitCode: 3, Output: nil, Duration: 0}, Err: nil})

	assert.Equal(t, pipeline.Fail("haskell-updater exited with status 3"),
		run.step(t, haskell(nil)).Run(t.Context(), run.env()))
}

func TestACustomStepThatCannotStartFails(t *testing.T) {
	t.Parallel()

	var run customRun

	run.runner.Script(runnertest.Answer{Result: runner.Result{ExitCode: 0, Output: nil, Duration: 0}, Err: errCannotStart})

	assert.Equal(t, pipeline.Fail(errCannotStart.Error()), run.step(t, haskell(nil)).Run(t.Context(), run.env()))
}

func TestOnlyAnOptionalCustomStepWhoseProgramIsMissingIsUnavailable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		optional  bool
		installed []string
		want      string
	}{
		{"optional and missing", true, nil, "haskell-updater is not installed"},
		{"optional and installed", true, []string{haskellUpdater}, ""},
		{"missing", false, nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			run := customRun{installed: tt.installed}
			step := run.step(t, haskell(func(s *config.CustomStep) { s.Optional = tt.optional }))

			gated, isGated := step.(pipeline.MayBeUnavailable)
			require.True(t, isGated)
			assert.Equal(t, tt.want, gated.Unavailable(run.env()))
		})
	}
}

func TestACustomStepPretends(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		change func(*config.CustomStep)
		want   string
	}{
		{"as the invoking user", nil, "would run: haskell-updater --quiet -- --verbose"},
		{
			"as root", func(s *config.CustomStep) { s.Elevated = true },
			"would run as root: haskell-updater --quiet -- --verbose",
		},
		{
			"optional and missing", func(s *config.CustomStep) { s.Optional = true },
			"haskell-updater is not installed, so the step would be skipped",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var run customRun

			lines, err := run.step(t, haskell(tt.change)).Pretend(t.Context(), run.env())
			require.NoError(t, err)
			assert.Equal(t, []string{tt.want}, lines)
			assert.Empty(t, run.runner.Commands())
		})
	}
}
