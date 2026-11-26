// SPDX-License-Identifier: GPL-3.0-or-later

package cli_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/emergup/internal/runner"
)

// hooksConfig sets a hook before the run, one after the update, and one
// turned off.
const hooksConfig = "[hooks.check]\nat = \"before-run\"\ncommand = [\"/usr/local/bin/check\", \"--quick\"]\n\n" +
	"[hooks.notify]\nat = \"after-update\"\ncommand = [\"/usr/local/bin/notify\"]\n\n" +
	"[hooks.off]\nenabled = false\nat = \"before-sync\"\ncommand = [\"/usr/local/bin/off\"]\n"

// hookCommands returns the commands of commands whose program is under
// /usr/local/bin, the hooks of hooksConfig.
func hookCommands(commands []runner.Command) []runner.Command {
	var hooks []runner.Command

	for _, command := range commands {
		if strings.HasPrefix(command.Args[0], "/usr/local/bin/") {
			hooks = append(hooks, command)
		}
	}

	return hooks
}

func TestRunRunsTheHooksOfTheConfiguration(t *testing.T) {
	t.Parallel()

	sess := newSession(t)
	sess.root = fstest.MapFS{configFile: {Data: []byte(hooksConfig)}}
	sess.keys.silent = true

	code, stdout, _ := sess.run(t, commandRun, optionYes)
	assert.Equal(t, 0, code)
	assert.True(t, strings.HasPrefix(stdout, "==> hook check (before the run): /usr/local/bin/check --quick\n"+
		firstHeading), stdout)
	assert.Contains(t, stdout, "==> hook notify (after update): /usr/local/bin/notify\n")

	hooks := hookCommands(sess.runner.Commands())
	require.Len(t, hooks, 2, "the hook turned off does not run")

	assert.Equal(t, runner.Command{
		Args: []string{"/usr/local/bin/check", "--quick"}, Mode: runner.Stream, Root: false,
		Env: []string{
			"EMERGUP_RUN_DIR=" + runDir, "EMERGUP_LOG=" + runDir + "/emergup.log",
			"EMERGUP_HOOK=check", "EMERGUP_WHEN=before", "EMERGUP_STEP=run",
		},
	}, hooks[0], "a hook runs as the invoking user, with the run's directory and log")
	assert.Equal(t, []string{"/usr/local/bin/notify"}, hooks[1].Args)
	assert.Contains(t, hooks[1].Env, "EMERGUP_STEP_STATUS=skipped")
}

func TestRunElevatesAHookConfiguredToBe(t *testing.T) {
	t.Parallel()

	sess := newSession(t)
	sess.root = fstest.MapFS{configFile: {Data: []byte(
		"[hooks.snapshot]\nat = \"before-run\"\ncommand = [\"/usr/local/bin/snapshot\"]\nelevated = true\n",
	)}}
	sess.keys.silent = true

	code, stdout, _ := sess.run(t, commandRun, optionYes)
	assert.Equal(t, 0, code)
	assert.True(t, strings.HasPrefix(stdout, "==> hook snapshot (before the run), as root: /usr/local/bin/snapshot\n"),
		stdout)

	commands := sess.runner.Commands()
	require.NotEmpty(t, commands)
	assert.Equal(t, []string{sudo, "/usr/local/bin/snapshot"}, commands[0].Args, "through the elevation method")
	assert.True(t, commands[0].Root)
}

func TestRunPretendShowsTheHooksAndRunsNone(t *testing.T) {
	t.Parallel()

	sess := newSession(t)
	sess.root = fstest.MapFS{configFile: {Data: []byte(hooksConfig)}}
	sess.keys.silent = true

	code, stdout, _ := sess.run(t, commandRun, optionPretend)
	assert.Equal(t, 0, code)
	assert.True(t, strings.HasPrefix(stdout, "    hook check (before the run): /usr/local/bin/check --quick\n"+
		firstHeading), stdout)
	assert.Contains(t, stdout, "    hook notify (after update): /usr/local/bin/notify\n")
	assert.NotContains(t, stdout, "hook off")
	assert.Empty(t, hookCommands(sess.runner.Commands()))
}
