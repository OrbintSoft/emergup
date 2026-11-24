// SPDX-License-Identifier: GPL-3.0-or-later

package steps_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/emergup/internal/config"
	"github.com/OrbintSoft/emergup/internal/pipeline"
	"github.com/OrbintSoft/emergup/internal/runner/runnertest"
	"github.com/OrbintSoft/emergup/internal/steps"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// notAvailable is what a step whose work is not written yet says.
const notAvailable = "not available yet in this version of emergup"

// stepKeys returns the keys of the [steps] section, in order.
func stepKeys() []string {
	section := reflect.TypeFor[config.Steps]()
	keys := make([]string, 0, section.NumField())

	for field := range section.Fields() {
		key, _, _ := strings.Cut(field.Tag.Get("toml"), ",")
		keys = append(keys, key)
	}

	return keys
}

// names returns the name of each of all.
func names(all []pipeline.Step) []string {
	named := make([]string, 0, len(all))
	for _, step := range all {
		named = append(named, step.Name())
	}

	return named
}

func TestBuiltInFollowsTheStepsSection(t *testing.T) {
	t.Parallel()

	all := steps.BuiltIn(config.Default(), steps.Options{})
	assert.Equal(t, stepKeys(), names(all), "a step for each key of [steps], in the same order")

	for _, step := range all {
		assert.True(t, step.Enabled(), "%s is enabled by default", step.Name())
	}
}

func TestBuiltInTakesWhetherEachStepRunsFromTheConfiguration(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Steps.LiveRebuild.Enabled = false
	cfg.Steps.Depclean.Enabled = false

	enabled := map[string]bool{}
	for _, step := range steps.BuiltIn(cfg, steps.Options{}) {
		enabled[step.Name()] = step.Enabled()
	}

	assert.Equal(t, map[string]bool{
		"sync": true, updateName: true, "live-rebuild": false,
		"preserved-rebuild": true, "config-files": true, "depclean": false,
	}, enabled)
}

func TestBuiltInTitles(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Steps.Update.Target = "@system"

	titles := map[string]string{}
	for _, step := range steps.BuiltIn(cfg, steps.Options{}) {
		titles[step.Name()] = step.Title()
	}

	assert.Equal(t, map[string]string{
		"sync":              "sync the repositories",
		updateName:          "update @system",
		"live-rebuild":      "rebuild the live packages",
		"preserved-rebuild": "rebuild the packages that use preserved libraries",
		"config-files":      "update the configuration files",
		"depclean":          "remove the packages no longer needed",
	}, titles)
}

func TestStepsNotWrittenYetRunNothing(t *testing.T) {
	t.Parallel()

	var fake runnertest.Fake

	env := pipeline.Env{
		Runner:  &fake,
		Printer: ui.Printer{Out: nil, Colors: config.Default().Colors, Color: false},
		Menu:    ui.Menu{},
		Now:     time.Now,
	}

	all := steps.BuiltIn(config.Default(), steps.Options{})
	require.NotEmpty(t, all)

	for _, step := range all {
		if slices.Contains([]string{syncName, updateName, liveName}, step.Name()) {
			continue // written
		}

		lines, err := step.Pretend(t.Context(), env)
		require.NoError(t, err)
		assert.Equal(t, []string{notAvailable}, lines, step.Name())
		assert.Equal(t, pipeline.Skip(notAvailable), step.Run(t.Context(), env), step.Name())
	}

	assert.Empty(t, fake.Commands())
}
