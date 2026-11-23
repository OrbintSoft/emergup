// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefault(t *testing.T) {
	t.Parallel()

	want := Config{
		Elevation: Elevation{Method: ElevationAuto},
		Steps: Steps{
			Sync: Sync{
				Step:        Step{Enabled: true, Command: []string{emerge, "--sync"}},
				MinInterval: 24 * time.Hour,
				AlwaysSync:  Entries{},
			},
			Update: Update{
				Step:    Step{Enabled: true, Command: []string{emerge}},
				Options: []string{"--verbose", "--update", "--deep", "--newuse", "--verbose-conflicts"},
				Target:  "@world",
				Exclude: Entries{},
			},
			LiveRebuild: LiveRebuild{
				Step:         Step{Enabled: true, Command: []string{emerge, "--oneshot"}},
				Mode:         LiveRebuildAuto,
				Interval:     24 * time.Hour,
				SmartCommand: []string{"smart-live-rebuild"},
			},
			PreservedRebuild: Step{Enabled: true, Command: []string{emerge, "@preserved-rebuild"}},
			ConfigFiles:      Step{Enabled: true, Command: []string{"dispatch-conf"}},
			Depclean:         Step{Enabled: true, Command: []string{emerge, "--depclean"}},
		},
	}

	assert.Equal(t, want, Default())
}

func TestDefaultReturnsIndependentValues(t *testing.T) {
	t.Parallel()

	first := Default()
	require.NotEmpty(t, first.Steps.Update.Options)
	require.NotEmpty(t, first.Steps.Sync.Command)

	first.Steps.Update.Options[0] = "--changed"
	first.Steps.Update.Exclude = append(first.Steps.Update.Exclude, "app-misc/example")
	first.Steps.Sync.Command[0] = "changed"

	second := Default()
	assert.Equal(t, "--verbose", second.Steps.Update.Options[0])
	assert.Empty(t, second.Steps.Update.Exclude)
	assert.Equal(t, emerge, second.Steps.Sync.Command[0])
}
