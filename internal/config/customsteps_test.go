// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// haskellStep is a custom step that a file adds, which the tests change.
const haskellStep = "[custom-steps.haskell]\nafter = \"preserved-rebuild\"\n" +
	"command = [\"haskell-updater\", \"--quiet\"]\n"

// The values of haskellStep.
const (
	afterPreserved = "preserved-rebuild"
	haskellName    = "haskell"
)

// haskellCommand returns the command of haskellStep.
func haskellCommand() []string {
	return []string{"haskell-updater", "--quiet"}
}

// customStepWith returns NewCustomStep after the step after, running
// command, changed by change.
func customStepWith(after string, command []string, change func(*CustomStep)) CustomStep {
	step := NewCustomStep()
	step.After, step.Command = after, command

	if change != nil {
		change(&step)
	}

	return step
}

func TestLoadAddsACustomStepWithItsDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := Load(t.Context(), fstest.MapFS{dropIn("10-haskell.toml"): file(haskellStep)})
	require.NoError(t, err)

	assert.Equal(t, CustomSteps{
		haskellName: {
			Enabled: true, After: afterPreserved, Title: "", Command: haskellCommand(),
			Elevated: false, Optional: false,
		},
	}, cfg.CustomSteps)
}

func TestLoadChangesOnlyTheValuesAFileSetsOfACustomStep(t *testing.T) {
	t.Parallel()

	cfg, err := Load(t.Context(), fstest.MapFS{
		MainFile: file(haskellStep + "title = \"rebuild the Haskell packages\"\n" +
			"elevated = true\noptional = true\n"),
		dropIn("10-report.toml"):  file("[custom-steps.report]\nafter = \"sync\"\ncommand = [\"report\"]\n"),
		dropIn("20-disable.toml"): file("[custom-steps.haskell]\nenabled = false\n"),
	})
	require.NoError(t, err)

	assert.Equal(t, CustomSteps{
		haskellName: customStepWith(afterPreserved, haskellCommand(), func(s *CustomStep) {
			s.Enabled, s.Title, s.Elevated, s.Optional = false, "rebuild the Haskell packages", true, true
		}),
		"report": customStepWith("sync", []string{"report"}, nil),
	}, cfg.CustomSteps)
}

func TestLoadAcceptsATurnedOffCustomStepAnotherFileDoesNotAdd(t *testing.T) {
	t.Parallel()

	cfg, err := Load(t.Context(), fstest.MapFS{
		dropIn("20-disable.toml"): file("[custom-steps.haskell]\nenabled = false\n"),
	})
	require.NoError(t, err)
	assert.False(t, cfg.CustomSteps[haskellName].Enabled)
}

func TestLoadAcceptsACustomStepAfterEveryBuiltInStep(t *testing.T) {
	t.Parallel()

	for _, step := range StepNames() {
		cfg, err := Load(t.Context(), fstest.MapFS{
			MainFile: file("[custom-steps.s]\nafter = \"" + step + "\"\ncommand = [\"x\"]\n"),
		})
		require.NoError(t, err, step)
		assert.Equal(t, step, cfg.CustomSteps["s"].After)
	}
}

func TestLoadReportsWhereEachCustomStepProblemIs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		line    int
		column  int
		key     string
		want    string
	}{
		{
			"unknown step", "[custom-steps.s]\nafter = \"upgrade\"\ncommand = [\"x\"]\n",
			2, 1, "custom-steps.s.after", `unknown step "upgrade", expected a built-in step: sync, news, ` +
				"update, live-rebuild, preserved-rebuild, config-files, depclean",
		},
		{
			"integer for a step", "[custom-steps.s]\nafter = 1\ncommand = [\"x\"]\n",
			2, 9, "custom-steps.s.after", "expected a string, found an integer",
		},
		{
			emptyProgram, "[custom-steps.s]\nafter = \"sync\"\ncommand = [\"\", \"x\"]\n",
			3, 1, "custom-steps.s.command", emptyProgram,
		},
		{
			"unknown key", "[custom-steps.s]\nafter = \"sync\"\ncommand = [\"x\"]\nat = \"after-sync\"\n",
			4, 1, "custom-steps.s.at", unknownKey,
		},
		{
			"name with capitals", "[custom-steps.Haskell]\nafter = \"sync\"\ncommand = [\"x\"]\n",
			2, 1, "custom-steps.Haskell", `custom step name "Haskell": use lowercase letters, digits and dashes`,
		},
		{
			"name of a built-in step", "[custom-steps.sync]\nafter = \"update\"\ncommand = [\"x\"]\n",
			2, 1, "custom-steps.sync", `custom step name "sync": a built-in step has it`,
		},
		{
			"name of the whole run", "[custom-steps.run]\nafter = \"update\"\ncommand = [\"x\"]\n",
			2, 1, "custom-steps.run", `custom step name "run": hooks name the whole run so`,
		},
		{
			"value for a custom step", "[custom-steps]\nhaskell = \"preserved-rebuild\"\n",
			2, 11, "custom-steps.haskell", "expected a table, found a string",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := Load(t.Context(), fstest.MapFS{dropIn("10-broken.toml"): file(tt.content)})
			assert.Equal(t, Config{}, cfg)

			list := problems(t, err)
			require.Len(t, list, 1)
			assert.Equal(t, brokenFile, list[0].File)
			assert.Equal(t, tt.line, list[0].Line, "line")
			assert.Equal(t, tt.column, list[0].Column, "column")
			assert.Equal(t, tt.key, list[0].Key)
			assert.ErrorContains(t, list[0], tt.want)
		})
	}
}

func TestLoadReportsACustomStepThatRunsWithoutItsPlaceOrACommand(t *testing.T) {
	t.Parallel()

	_, err := Load(t.Context(), fstest.MapFS{
		MainFile:                 file("[custom-steps.haskell]\nenabled = false\nafter = \"preserved-rebuild\"\n"),
		dropIn("10-enable.toml"): file("[custom-steps.haskell]\nenabled = true\n\n[custom-steps.bare]\n"),
	})

	got := make([]string, 0, len(problems(t, err)))
	for _, problem := range problems(t, err) {
		got = append(got, problem.Error())
	}

	const enabling = "/etc/emergup/config.d/10-enable.toml"

	assert.Equal(t, []string{
		enabling + `: custom-steps.bare.after: missing: say which built-in step it follows, ` +
			`such as after = "preserved-rebuild"`,
		enabling + ": custom-steps.bare.command: missing: give the program the step runs and its arguments",
		enabling + ": custom-steps.haskell.command: missing: give the program the step runs and its arguments",
	}, got)
}

func TestLoadAcceptsAHookAtACustomStepOfAnyFile(t *testing.T) {
	t.Parallel()

	cfg, err := Load(t.Context(), fstest.MapFS{
		MainFile: file("[hooks.h]\nat = \"before-haskell\"\ncommand = [\"x\"]\n\n" +
			"[hooks.off]\nat = \"after-report\"\ncommand = [\"x\"]\n"),
		dropIn("10-haskell.toml"): file(haskellStep),
		dropIn("20-report.toml"):  file("[custom-steps.report]\nenabled = false\n"),
	})
	require.NoError(t, err)

	assert.Equal(t, haskellName, cfg.Hooks["h"].At.Step())
	assert.Equal(t, "report", cfg.Hooks["off"].At.Step(), "a hook can name a custom step that is turned off")
}

func TestLoadReportsAHookAtAStepThatNoFileAdds(t *testing.T) {
	t.Parallel()

	_, err := Load(t.Context(), fstest.MapFS{
		MainFile: file(haskellStep),
		dropIn("10-broken.toml"): file("[hooks.ok]\nat = \"after-haskell\"\ncommand = [\"x\"]\n\n" +
			"[hooks.h]\nenabled = false\nat = \"before-haskel\"\n"),
	})

	list := problems(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, brokenFile, list[0].File)
	assert.Equal(t, 7, list[0].Line, "line")
	assert.Equal(t, 1, list[0].Column, "column")
	assert.Equal(t, keyHookAt, list[0].Key)
	assert.EqualError(t, list[0].Err, `unknown hook place "before-haskel", expected "before-" or "after-" `+
		"followed by run, a built-in step (sync, news, update, live-rebuild, preserved-rebuild, config-files, depclean) "+
		"or a custom step (haskell)")
}

func TestInspectSaysWhereTheValuesOfACustomStepComeFrom(t *testing.T) {
	t.Parallel()

	_, origins, err := Inspect(t.Context(), fstest.MapFS{
		MainFile:          file(haskellStep),
		dropIn("10.toml"): file("[custom-steps.haskell]\nelevated = true\n"),
	})
	require.NoError(t, err)

	assert.Equal(t, shownMain, origins.Of("custom-steps.haskell.after"))
	assert.Equal(t, shownDropIn, origins.Of("custom-steps.haskell.elevated"))
	assert.Empty(t, origins.Of("custom-steps.haskell.optional"))
}

func TestWriteTOMLWritesTheCustomStepsAfterTheBuiltInOnes(t *testing.T) {
	t.Parallel()

	cfg, origins, err := Inspect(t.Context(), fstest.MapFS{
		MainFile:          file(haskellStep),
		dropIn("10.toml"): file("[custom-steps.audit]\nafter = \"depclean\"\ncommand = [\"audit\"]\n"),
	})
	require.NoError(t, err)

	const defaults = "elevated = false  # default\noptional = false  # default\n"

	out := shown(t, cfg, origins)
	assert.Contains(t, out, "command = [\"emerge\", \"--depclean\"]  # default\n\n"+
		"[custom-steps.audit]\nenabled = true  # default\nafter = \"depclean\"  # "+shownDropIn+"\n"+
		"title = \"\"  # default\ncommand = [\"audit\"]  # "+shownDropIn+"\n"+defaults+"\n"+
		"[custom-steps.haskell]\nenabled = true  # default\nafter = \"preserved-rebuild\"  # "+shownMain+"\n"+
		"title = \"\"  # default\ncommand = [\"haskell-updater\", \"--quiet\"]  # "+shownMain+"\n"+defaults+
		"\n[colors]\n")
}

func TestWriteTOMLReadsBackTheCustomSteps(t *testing.T) {
	t.Parallel()

	cfg, origins, err := Inspect(t.Context(), fstest.MapFS{
		MainFile: file(haskellStep + "title = \"rebuild the Haskell packages\"\nelevated = true\noptional = true\n\n" +
			"[hooks.h]\nat = \"after-haskell\"\ncommand = [\"x\"]\n"),
		dropIn("10.toml"): file("[custom-steps.off]\nenabled = false\n"),
	})
	require.NoError(t, err)

	back, err := Load(t.Context(), fstest.MapFS{MainFile: file(shown(t, cfg, origins))})
	require.NoError(t, err)
	assert.Equal(t, cfg, back)
}
