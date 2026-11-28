// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/emergup/internal/report"
	"github.com/OrbintSoft/emergup/internal/runner"
	"github.com/OrbintSoft/emergup/internal/runner/runnertest"
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
