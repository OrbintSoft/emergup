// SPDX-License-Identifier: GPL-3.0-or-later

package cli_test

import (
	"bytes"
	"io/fs"
	"path"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"

	"github.com/OrbintSoft/emergup/internal/cli"
	"github.com/OrbintSoft/emergup/internal/config"
	"github.com/OrbintSoft/emergup/internal/version"
)

const usageLine = "usage: emergup <command>"

// cmdConfig is the config command.
const cmdConfig = "config"

// run calls cli.Main as the program invokedAs with args, on a system without
// configuration files, and returns the exit status and what was written to
// stdout and stderr.
func run(t *testing.T, invokedAs string, args ...string) (int, string, string) {
	t.Helper()

	return runOn(t, fstest.MapFS{}, invokedAs, args...)
}

// runOn is run on a system whose root file system is root.
func runOn(t *testing.T, root fs.FS, invokedAs string, args ...string) (int, string, string) {
	t.Helper()

	var stdout, stderr bytes.Buffer

	code := cli.Main(t.Context(), cli.Env{Stdout: &stdout, Stderr: &stderr, Root: root}, invokedAs, args)

	return code, stdout.String(), stderr.String()
}

func TestVersion(t *testing.T) {
	t.Parallel()

	for _, arg := range []string{"version", "--version"} {
		t.Run(arg, func(t *testing.T) {
			t.Parallel()
			code, stdout, stderr := run(t, "/usr/bin/emergup", arg)
			assert.Equal(t, 0, code)
			assert.Equal(t, "emergup "+version.String()+"\n", stdout)
			assert.Empty(t, stderr)
		})
	}
}

func TestHelp(t *testing.T) {
	t.Parallel()

	for _, arg := range []string{"help", "-h", "--help"} {
		t.Run(arg, func(t *testing.T) {
			t.Parallel()
			code, stdout, stderr := run(t, "/usr/bin/emergup", arg)
			assert.Equal(t, 0, code)
			assert.Contains(t, stdout, usageLine)
			assert.Contains(t, stdout, "version")
			assert.Contains(t, stdout, "run ")
			assert.Contains(t, stdout, "--yes")
			assert.Contains(t, stdout, "--pretend")
			assert.Contains(t, stdout, "config check")
			assert.Contains(t, stdout, "config show")
			assert.Empty(t, stderr)
		})
	}
}

func TestUsageErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		message string
	}{
		{"no command", nil, ""},
		{"unknown command", []string{"frobnicate"}, `emergup: unknown command "frobnicate"`},
		{"unknown option", []string{"--frobnicate"}, `emergup: unknown option "--frobnicate"`},
		{"argument after version", []string{"version", "extra"}, `emergup version: unexpected argument "extra"`},
		{"argument after help", []string{"help", "extra"}, `emergup help: unexpected argument "extra"`},
		{"config without subcommand", []string{cmdConfig}, "emergup config: missing subcommand"},
		{"unknown config subcommand", []string{cmdConfig, "frob"}, `emergup config: unknown subcommand "frob"`},
		{"argument after config check", []string{cmdConfig, "check", "x"}, `emergup config check: unexpected argument "x"`},
		{"argument after config show", []string{cmdConfig, "show", "x"}, `emergup config show: unexpected argument "x"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			code, stdout, stderr := run(t, "/usr/bin/emergup", tt.args...)
			assert.Equal(t, 2, code)
			assert.Empty(t, stdout)
			assert.Contains(t, stderr, tt.message)
			assert.Contains(t, stderr, usageLine)
		})
	}
}

func TestUsageNamesTheInvokedProgram(t *testing.T) {
	t.Parallel()
	code, stdout, _ := run(t, "/opt/test/emergup-dev", "help")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "usage: emergup-dev <command>")
}

// file is a configuration file with the given content.
func file(content string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte(content)}
}

// dropIn is the path of a drop-in file of config.DropInDir.
func dropIn(name string) string {
	return path.Join(config.DropInDir, name)
}

func TestConfigCheckWithoutConfigurationFiles(t *testing.T) {
	t.Parallel()

	code, stdout, stderr := run(t, "emergup", cmdConfig, "check")
	assert.Equal(t, 0, code)
	assert.Equal(t, "No configuration file: the generic defaults apply.\nThe configuration is valid.\n", stdout)
	assert.Empty(t, stderr)
}

func TestConfigCheckListsTheFilesRead(t *testing.T) {
	t.Parallel()

	code, stdout, stderr := runOn(t, fstest.MapFS{
		config.MainFile:   file("[steps.update]\ntarget = \"@system\"\n"),
		dropIn("10.toml"): file("[steps.depclean]\nenabled = false\n"),
	}, "emergup", cmdConfig, "check")
	assert.Equal(t, 0, code)
	assert.Equal(t, "Read /etc/emergup/config.toml\nRead /etc/emergup/config.d/10.toml\n"+
		"The configuration is valid.\n", stdout)
	assert.Empty(t, stderr)
}

func TestConfigCheckReportsEachProblem(t *testing.T) {
	t.Parallel()

	code, stdout, stderr := runOn(t, fstest.MapFS{
		config.MainFile:   file("[steps.sync]\nbogus = 1\n"),
		dropIn("10.toml"): file("[steps.update]\ntarget = \"\"\n"),
	}, "emergup", cmdConfig, "check")
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Equal(t, "/etc/emergup/config.toml:2:1: steps.sync.bogus: unknown key\n"+
		"/etc/emergup/config.d/10.toml:2:1: steps.update.target: empty target: give a package set such as \"@world\"\n"+
		"2 problems in the configuration.\n", stderr)
}

func TestConfigShow(t *testing.T) {
	t.Parallel()

	code, stdout, stderr := runOn(t, fstest.MapFS{
		config.MainFile: file("[steps.update]\ntarget = \"@system\"\n"),
	}, "emergup", cmdConfig, "show")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "[steps.update]\n")
	assert.Contains(t, stdout, "target = \"@system\"  # /etc/emergup/config.toml\n")
	assert.Contains(t, stdout, "min-interval = \"24h\"  # default\n")
	assert.Empty(t, stderr)
}

func TestConfigShowWithAProblem(t *testing.T) {
	t.Parallel()

	code, stdout, stderr := runOn(t, fstest.MapFS{
		config.MainFile: file("[steps.sync]\nbogus = 1\n"),
	}, "emergup", cmdConfig, "show")
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Equal(t, "/etc/emergup/config.toml:2:1: steps.sync.bogus: unknown key\n"+
		"1 problem in the configuration.\n", stderr)
}
