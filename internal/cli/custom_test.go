// SPDX-License-Identifier: GPL-3.0-or-later

package cli_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"

	"github.com/OrbintSoft/emergup/internal/runner"
)

// customConfig adds a custom step after the depclean, elevated, an optional
// one whose program is not installed, and one turned off.
const customConfig = "[custom-steps.audit]\nafter = \"depclean\"\ncommand = [\"/usr/local/bin/audit\", \"--all\"]\n" +
	"elevated = true\n\n" +
	"[custom-steps.missing]\nafter = \"sync\"\ncommand = [\"not-installed\"]\noptional = true\n\n" +
	"[custom-steps.off]\nenabled = false\nafter = \"sync\"\ncommand = [\"/usr/local/bin/off\"]\n"

// customCommands returns the commands of commands that run the programs of
// customConfig, elevated or not.
func customCommands(commands []runner.Command) [][]string {
	var custom [][]string

	for _, command := range commands {
		for _, arg := range command.Args {
			if strings.HasPrefix(arg, "/usr/local/bin/") || arg == "not-installed" {
				custom = append(custom, command.Args)

				break
			}
		}
	}

	return custom
}

func TestRunRunsTheCustomStepsOfTheConfiguration(t *testing.T) {
	t.Parallel()

	sess := newSession(t)
	sess.root = fstest.MapFS{configFile: {Data: []byte(customConfig)}}
	sess.keys.silent = true

	code, stdout, _ := sess.run(t, commandRun, optionYes)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "==> [2/9] missing: run not-installed\n"+
		"==> missing skipped: not-installed is not installed\n==> [3/9] news: ")
	assert.Contains(t, stdout, "==> [9/9] audit: run /usr/local/bin/audit\n")
	assert.Contains(t, stdout, "    off                disabled\n")
	assert.Equal(t, [][]string{{sudo, "/usr/local/bin/audit", "--all"}}, customCommands(sess.runner.Commands()),
		"the elevated step runs through the elevation method, and only it")
}

func TestRunPretendShowsTheCustomStepsAndRunsNone(t *testing.T) {
	t.Parallel()

	sess := newSession(t)
	sess.root = fstest.MapFS{configFile: {Data: []byte(customConfig)}}
	sess.keys.silent = true

	code, stdout, _ := sess.run(t, commandRun, optionPretend)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "    not-installed is not installed, so the step would be skipped\n")
	assert.Contains(t, stdout, "==> [9/9] audit: run /usr/local/bin/audit\n"+
		"    would run as root: /usr/local/bin/audit --all\n")
	assert.Contains(t, stdout, "off is disabled in the configuration\n")
	assert.Empty(t, customCommands(sess.runner.Commands()))
}
