// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/emergup/internal/config"
	"github.com/OrbintSoft/emergup/internal/pipeline"
	"github.com/OrbintSoft/emergup/internal/runner/runnertest"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// The lines a run writes around a step.
const (
	question = "==> Run this step? [Y/s/a/?] "
	withYes  = question + "run the step (--yes)\n"
	summary  = "==> Summary\n"
)

// errBroken is the error of a keyboard that cannot be read.
var errBroken = errors.New("input/output error")

// clock is a fake time that moves only when a step runs.
type clock struct {
	now time.Time
}

func (c *clock) Now() time.Time { return c.now }

// keyboard answers the questions with keys, then with err or io.EOF. When
// silent, a question fails the test.
type keyboard struct {
	t      *testing.T
	keys   []rune
	err    error
	silent bool
}

func (k *keyboard) ReadKey(context.Context) (rune, error) {
	if k.silent {
		k.t.Error("a question was asked")
	}

	if len(k.keys) == 0 {
		if k.err != nil {
			return 0, k.err
		}

		return 0, io.EOF
	}

	key := k.keys[0]
	k.keys = k.keys[1:]

	return key, nil
}

// step is a fake pipeline.Step.
type step struct {
	name     string
	disabled bool
	clock    *clock
	// took is how long Run takes; outcome is what it returns.
	took    time.Duration
	outcome pipeline.Outcome
	// interrupt, when set, is called by Run, as a Ctrl-C would end the
	// context while the step runs.
	interrupt  context.CancelFunc
	pretend    []string
	pretendErr error
	// confirms makes the step ask its own question.
	confirms bool
	ran      bool
}

func (s *step) Name() string         { return s.name }
func (s *step) Title() string        { return "do " + s.name }
func (s *step) Enabled() bool        { return !s.disabled }
func (s *step) ConfirmsItself() bool { return s.confirms }

func (s *step) Pretend(context.Context, pipeline.Env) ([]string, error) {
	return s.pretend, s.pretendErr
}

func (s *step) Run(context.Context, pipeline.Env) pipeline.Outcome {
	s.ran = true
	s.clock.now = s.clock.now.Add(s.took)

	if s.interrupt != nil {
		s.interrupt()
	}

	return s.outcome
}

// testRig runs fake steps and records what the run writes, and the
// commands of its hooks.
type testRig struct {
	out    bytes.Buffer
	clock  clock
	keys   keyboard
	yes    bool
	color  bool
	runner runnertest.Fake
	// hooks and hookEnv are those of the run.
	hooks   []pipeline.Hook
	hookEnv []string
}

func newRig(t *testing.T, keys ...rune) *testRig {
	t.Helper()

	return &testRig{
		clock: clock{now: time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)},
		keys:  keyboard{t: t, keys: keys},
	}
}

// step returns a fake step that takes took and ends with outcome.
func (rig *testRig) step(name string, took time.Duration, outcome pipeline.Outcome) *step {
	return &step{name: name, clock: &rig.clock, took: took, outcome: outcome}
}

func (rig *testRig) env() pipeline.Env {
	printer := ui.Printer{Out: &rig.out, Colors: config.Default().Colors, Color: rig.color}

	return pipeline.Env{
		Runner:  &rig.runner,
		Printer: printer,
		Menu:    ui.Menu{Printer: printer, Keyboard: &rig.keys, Yes: rig.yes},
		Now:     rig.clock.Now,
		Hooks:   rig.hooks,
		HookEnv: rig.hookEnv,
	}
}

func (rig *testRig) run(ctx context.Context, steps ...pipeline.Step) pipeline.Report {
	return pipeline.Run(ctx, rig.env(), steps)
}

func TestRunWithYesRunsEveryEnabledStep(t *testing.T) {
	t.Parallel()

	rig := newRig(t)
	rig.yes = true
	rig.keys.silent = true

	disabled := rig.step("live-rebuild", time.Hour, pipeline.Success())
	disabled.disabled = true

	report := rig.run(t.Context(),
		rig.step("sync", 42*time.Second, pipeline.Success()),
		rig.step("update", 12*time.Minute+3*time.Second, pipeline.Success()),
		disabled,
		rig.step("depclean", 0, pipeline.Skip("nothing to remove")),
	)

	assert.Equal(t, "==> [1/3] sync: do sync\n"+withYes+
		"==> sync succeeded in 42s\n"+
		"==> [2/3] update: do update\n"+withYes+
		"==> update succeeded in 12m03s\n"+
		"==> [3/3] depclean: do depclean\n"+withYes+
		"==> depclean skipped: nothing to remove\n"+
		summary+
		"    sync          succeeded     42s\n"+
		"    update        succeeded  12m03s\n"+
		"    live-rebuild  disabled\n"+
		"    depclean      skipped       <1s  nothing to remove\n"+
		"==> The run finished in 12m45s.\n", rig.out.String())

	assert.False(t, disabled.ran, "a disabled step does not run")
	assert.Equal(t, pipeline.Report{
		Steps: []pipeline.StepReport{
			{Name: "sync", Status: pipeline.Succeeded, Reason: "", Ran: true, Duration: 42 * time.Second},
			{Name: "update", Status: pipeline.Succeeded, Reason: "", Ran: true, Duration: 12*time.Minute + 3*time.Second},
			{Name: "live-rebuild", Status: pipeline.Disabled, Reason: "", Ran: false, Duration: 0},
			{Name: "depclean", Status: pipeline.Skipped, Reason: "nothing to remove", Ran: true, Duration: 0},
		},
		Result:   pipeline.RunFinished,
		Duration: 12*time.Minute + 45*time.Second,
	}, report)
}

func TestRunAsksBeforeEachStep(t *testing.T) {
	t.Parallel()

	rig := newRig(t, ui.KeyEnter, 's', 'y')
	skipped := rig.step("update", time.Minute, pipeline.Success())

	report := rig.run(t.Context(),
		rig.step("sync", time.Second, pipeline.Success()),
		skipped,
		rig.step("depclean", time.Second, pipeline.Success()),
	)

	assert.Equal(t, "==> [1/3] sync: do sync\n"+question+"run the step\n"+
		"==> sync succeeded in 1s\n"+
		"==> [2/3] update: do update\n"+question+"skip the step\n"+
		"==> update skipped: you chose to skip it\n"+
		"==> [3/3] depclean: do depclean\n"+question+"run the step\n"+
		"==> depclean succeeded in 1s\n"+
		summary+
		"    sync      succeeded      1s\n"+
		"    update    skipped            you chose to skip it\n"+
		"    depclean  succeeded      1s\n"+
		"==> The run finished in 2s.\n", rig.out.String())

	assert.False(t, skipped.ran)
	assert.Equal(t, pipeline.RunFinished, report.Result)
	require.Len(t, report.Steps, 3)
	assert.Equal(t, pipeline.StepReport{
		Name: "update", Status: pipeline.Skipped, Reason: "you chose to skip it", Ran: false, Duration: 0,
	}, report.Steps[1])
}

func TestRunStopsWhenTheQuestionIsAborted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		keys []rune
	}{
		{"the abort key", []rune{ui.KeyEnter, 'a'}},
		{"Ctrl-C", []rune{ui.KeyEnter, ui.KeyInterrupt}},
		{"the end of the input", []rune{ui.KeyEnter}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rig := newRig(t, tt.keys...)
			last := rig.step("depclean", time.Second, pipeline.Success())

			report := rig.run(t.Context(),
				rig.step("sync", 42*time.Second, pipeline.Success()),
				rig.step("update", time.Minute, pipeline.Success()),
				last,
			)

			assert.Contains(t, rig.out.String(), "==> [2/3] update: do update\n"+question)
			assert.NotContains(t, rig.out.String(), "depclean: do depclean", "the next step is not announced")
			assert.Contains(t, rig.out.String(), summary+
				"    sync      succeeded     42s\n"+
				"    update    not run\n"+
				"    depclean  not run\n"+
				"==> The run was stopped after 42s.\n")
			assert.False(t, last.ran)
			assert.Equal(t, pipeline.RunStopped, report.Result)
			assert.Equal(t, []pipeline.Status{pipeline.Succeeded, pipeline.NotRun, pipeline.NotRun},
				statuses(report))
		})
	}
}

// statuses returns the status of each step of report.
func statuses(report pipeline.Report) []pipeline.Status {
	all := make([]pipeline.Status, 0, len(report.Steps))
	for _, step := range report.Steps {
		all = append(all, step.Status)
	}

	return all
}

func TestRunStopsAtAFailure(t *testing.T) {
	t.Parallel()

	rig := newRig(t)
	rig.yes = true

	last := rig.step("depclean", time.Second, pipeline.Success())

	report := rig.run(t.Context(),
		rig.step("sync", 42*time.Second, pipeline.Success()),
		rig.step("update", 3*time.Second, pipeline.Fail("emerge exited with status 1")),
		last,
	)

	assert.Contains(t, rig.out.String(), withYes+"==> update failed after 3s: emerge exited with status 1\n"+
		summary+
		"    sync      succeeded     42s\n"+
		"    update    failed         3s  emerge exited with status 1\n"+
		"    depclean  not run\n"+
		"==> The run failed after 45s.\n")
	assert.False(t, last.ran)
	assert.Equal(t, pipeline.RunFailed, report.Result)
}

func TestRunStopsWhenItIsInterrupted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		outcome pipeline.Outcome
		status  pipeline.Status
		ended   string
		row     string
	}{
		{
			"while the step fails", pipeline.Fail("emerge was stopped"), pipeline.Stopped,
			"==> update stopped after 3s: emerge was stopped\n",
			"    update    stopped        3s  emerge was stopped\n",
		},
		{
			"while the step ends well", pipeline.Success(), pipeline.Succeeded,
			"==> update succeeded in 3s\n",
			"    update    succeeded      3s\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			rig := newRig(t)
			rig.yes = true

			interrupted := rig.step("update", 3*time.Second, tt.outcome)
			interrupted.interrupt = cancel
			last := rig.step("depclean", time.Second, pipeline.Success())

			report := rig.run(ctx, rig.step("sync", 42*time.Second, pipeline.Success()), interrupted, last)

			assert.Contains(t, rig.out.String(), withYes+tt.ended+summary+
				"    sync      succeeded     42s\n"+
				tt.row+
				"    depclean  not run\n"+
				"==> The run was stopped after 45s.\n")
			assert.NotContains(t, rig.out.String(), "depclean: do depclean", "the next step is not announced")
			assert.False(t, last.ran)
			assert.Equal(t, pipeline.RunStopped, report.Result)
			require.Len(t, report.Steps, 3)
			assert.Equal(t, tt.status, report.Steps[1].Status)
		})
	}
}

func TestRunFailsWhenTheQuestionCannotBeAsked(t *testing.T) {
	t.Parallel()

	rig := newRig(t)
	rig.keys.err = errBroken

	first := rig.step("sync", time.Second, pipeline.Success())
	report := rig.run(t.Context(), first)

	assert.Contains(t, rig.out.String(), "==> reading the answer: input/output error\n"+summary+
		"    sync  not run\n"+
		"==> The run failed after <1s.\n")
	assert.False(t, first.ran)
	assert.Equal(t, pipeline.RunFailed, report.Result)
}

func TestRunColoursTheSummary(t *testing.T) {
	t.Parallel()

	rig := newRig(t)
	rig.yes = true
	rig.color = true

	disabled := rig.step("news", 0, pipeline.Success())
	disabled.disabled = true

	rig.run(t.Context(),
		rig.step("sync", time.Second, pipeline.Success()),
		disabled,
		rig.step("update", time.Second, pipeline.Skip("nothing to update")),
		rig.step("depclean", time.Second, pipeline.Fail("broken")),
		rig.step("config-files", time.Second, pipeline.Success()),
	)

	assert.Contains(t, rig.out.String(), "\x1b[1;34m==> Summary\x1b[0m\n"+
		"    \x1b[32msync          succeeded      1s\x1b[0m\n"+
		"    \x1b[33mnews          disabled\x1b[0m\n"+
		"    \x1b[33mupdate        skipped        1s  nothing to update\x1b[0m\n"+
		"    \x1b[1;31mdepclean      failed         1s  broken\x1b[0m\n"+
		"    config-files  not run\n"+
		"\x1b[1;31m==> The run failed after 3s.\x1b[0m\n")
}

func TestPretendSaysWhatEachStepWouldDo(t *testing.T) {
	t.Parallel()

	rig := newRig(t)
	rig.keys.silent = true

	sync := rig.step("sync", time.Second, pipeline.Success())
	sync.pretend = []string{"emerge --sync gentoo", "emerge --sync guru"}
	update := rig.step("update", time.Second, pipeline.Success())
	update.disabled = true
	depclean := rig.step("depclean", time.Second, pipeline.Success())
	depclean.pretend = []string{"emerge --depclean"}

	result := pipeline.Pretend(t.Context(), rig.env(), []pipeline.Step{sync, update, depclean})

	assert.Equal(t, "==> [1/2] sync: do sync\n"+
		"    emerge --sync gentoo\n"+
		"    emerge --sync guru\n"+
		"==> update is disabled in the configuration\n"+
		"==> [2/2] depclean: do depclean\n"+
		"    emerge --depclean\n", rig.out.String())
	assert.Equal(t, pipeline.RunFinished, result)
	assert.False(t, sync.ran || update.ran || depclean.ran, "no step runs")
}

func TestPretendReportsAStepThatCannotSayWhatItWouldDo(t *testing.T) {
	t.Parallel()

	rig := newRig(t)

	sync := rig.step("sync", time.Second, pipeline.Success())
	sync.pretendErr = errBroken
	depclean := rig.step("depclean", time.Second, pipeline.Success())
	depclean.pretend = []string{"emerge --depclean"}

	result := pipeline.Pretend(t.Context(), rig.env(), []pipeline.Step{sync, depclean})

	assert.Equal(t, "==> [1/2] sync: do sync\n"+
		"==> could not work out what sync would do: input/output error\n"+
		"==> [2/2] depclean: do depclean\n"+
		"    emerge --depclean\n", rig.out.String())
	assert.Equal(t, pipeline.RunFailed, result)
}

func TestPretendStopsWhenTheContextEnds(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	rig := newRig(t)
	result := pipeline.Pretend(ctx, rig.env(), []pipeline.Step{rig.step("sync", 0, pipeline.Success())})

	assert.Empty(t, rig.out.String())
	assert.Equal(t, pipeline.RunStopped, result)
}

func TestStatusString(t *testing.T) {
	t.Parallel()

	tests := map[pipeline.Status]string{
		pipeline.NotRun:     "not run",
		pipeline.Succeeded:  "succeeded",
		pipeline.Skipped:    "skipped",
		pipeline.Failed:     "failed",
		pipeline.Stopped:    "stopped",
		pipeline.Disabled:   "disabled",
		pipeline.Status(42): "Status(42)",
	}
	for status, want := range tests {
		require.Equal(t, want, status.String())
	}
}

func TestRunDoesNotAskAStepThatConfirmsItself(t *testing.T) {
	t.Parallel()

	rig := newRig(t, ui.KeyEnter)

	asks := rig.step("update", 3*time.Second, pipeline.Success())
	asks.confirms = true

	report := rig.run(t.Context(), asks, rig.step("depclean", time.Second, pipeline.Success()))

	assert.True(t, strings.HasPrefix(rig.out.String(), "==> [1/2] update: do update\n"+
		"==> update succeeded in 3s\n"+
		"==> [2/2] depclean: do depclean\n"+question+"run the step\n"), rig.out.String())
	assert.Equal(t, pipeline.RunFinished, report.Result)
}

func TestAStepCanStopTheRun(t *testing.T) {
	t.Parallel()

	rig := newRig(t)
	rig.yes = true

	aborted := rig.step("update", 3*time.Second, pipeline.Abort("you aborted the run"))
	aborted.confirms = true
	last := rig.step("depclean", time.Second, pipeline.Success())

	report := rig.run(t.Context(), aborted, last)

	assert.Contains(t, rig.out.String(), "==> update stopped after 3s: you aborted the run\n"+summary+
		"    update    stopped        3s  you aborted the run\n"+
		"    depclean  not run\n"+
		"==> The run was stopped after 3s.\n")
	assert.False(t, last.ran)
	assert.Equal(t, pipeline.RunStopped, report.Result)
}

func TestRunWritesTheNotesOfTheStepsAfterTheSummary(t *testing.T) {
	t.Parallel()

	rig := newRig(t)
	rig.yes = true

	notes := []pipeline.Note{{Text: "2 excluded packages have an update:", Rows: []string{"a/b-1.0", "c/d-2.0"}}}
	outcome := pipeline.Success()
	outcome.Notes = notes

	report := rig.run(t.Context(), rig.step("update", time.Second, outcome))

	assert.True(t, strings.HasSuffix(rig.out.String(), summary+
		"    update  succeeded      1s\n"+
		"==> 2 excluded packages have an update:\n"+
		"    a/b-1.0\n"+
		"    c/d-2.0\n"+
		"==> The run finished in 1s.\n"), rig.out.String())
	require.Len(t, report.Steps, 1)
	assert.Equal(t, notes, report.Steps[0].Notes)
}
