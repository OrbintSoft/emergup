// SPDX-License-Identifier: GPL-3.0-or-later

package cli_test

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The configuration of the tests of the variables kept for the commands run
// as root, with a hook run as root, which is one such command.
const (
	keepEditor   = "[elevation]\nkeep-env = [\"EDITOR\", \"VISUAL\"]\n"
	elevatedHook = "[hooks.snapshot]\nat = \"before-run\"\ncommand = [\"/usr/local/bin/snapshot\"]\nelevated = true\n"
	doasMethod   = "[elevation]\nmethod = \"doas\"\n"
	doasKeep     = "keep-env = [\"EDITOR\", \"VISUAL\"]\n"
	doas         = "doas"
)

// doasNotice is the notice of a run through doas whose commands run as root
// are to keep EDITOR and VISUAL, and the variables of a hook if hook is set.
func doasNotice(hook bool) string {
	names := "EDITOR VISUAL"
	if hook {
		names += " EMERGUP_HOOK EMERGUP_WHEN EMERGUP_STEP EMERGUP_RUN_DIR EMERGUP_LOG EMERGUP_STEP_STATUS EMERGUP_RESULT"
	}

	return "==> doas keeps only the variables doas.conf keeps: give its rule setenv { " + names +
		" } for the commands run as root to get them\n"
}

func TestRunKeepsTheVariablesOfKeepEnvThroughSudo(t *testing.T) {
	t.Parallel()

	sess := newSession(t)
	sess.root = fstest.MapFS{configFile: {Data: []byte(keepEditor + "\n" + elevatedHook)}}
	sess.keys.silent = true

	code, _, _ := sess.run(t, commandRun, optionYes)
	assert.Equal(t, 0, code)

	commands := sess.runner.Commands()
	require.NotEmpty(t, commands)
	assert.Equal(t, []string{
		sudo, "--preserve-env=EDITOR,VISUAL,EMERGUP_RUN_DIR,EMERGUP_LOG,EMERGUP_HOOK,EMERGUP_WHEN,EMERGUP_STEP",
		"/usr/local/bin/snapshot",
	}, commands[0].Args)
}

func TestRunGivesRun0TheValuesOfTheVariablesToKeep(t *testing.T) {
	t.Parallel()

	sess := newSession(t)
	sess.system.programs = []string{"run0"}
	sess.system.env = map[string]string{"VISUAL": "emacs"}
	sess.root = fstest.MapFS{configFile: {Data: []byte(keepEditor + "\n" + elevatedHook)}}
	sess.keys.silent = true

	code, _, _ := sess.run(t, commandRun, optionYes)
	assert.Equal(t, 0, code)

	commands := sess.runner.Commands()
	require.NotEmpty(t, commands)
	assert.Equal(t, []string{"run0", "--setenv=VISUAL=emacs", "--setenv=EMERGUP_RUN_DIR=" + runDir},
		commands[0].Args[:3], "EDITOR is not set")
}

func TestRunThroughDoasSaysWhatDoasConfMustKeep(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config string
		notice string
	}{
		{"the variables to keep", doasMethod + doasKeep, doasNotice(false)},
		{"and those of a hook run as root", doasMethod + doasKeep + "\n" + elevatedHook, doasNotice(true)},
		{"nothing to keep", doasMethod, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			for _, option := range []string{optionYes, optionPretend} {
				sess := newSession(t)
				sess.system.programs = []string{doas}
				sess.root = fstest.MapFS{configFile: {Data: []byte(tt.config)}}
				sess.keys.silent = true

				code, stdout, _ := sess.run(t, commandRun, option)
				assert.Equal(t, 0, code, option)

				if tt.notice == "" {
					assert.NotContains(t, stdout, "doas.conf", option)

					continue
				}

				assert.Contains(t, stdout, tt.notice, option)
			}
		})
	}
}

func TestRunThroughSudoSaysNothingOfDoasConf(t *testing.T) {
	t.Parallel()

	sess := newSession(t)
	sess.root = fstest.MapFS{configFile: {Data: []byte(keepEditor)}}
	sess.keys.silent = true

	_, stdout, _ := sess.run(t, commandRun, optionYes)
	assert.NotContains(t, stdout, "doas.conf")
}
