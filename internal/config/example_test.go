// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// examples is the example configuration of the repository, laid out as on
// the root of a system.
func examples() fs.FS {
	return os.DirFS(filepath.Join("..", "..", "examples"))
}

func TestExampleConfigurationIsValid(t *testing.T) {
	t.Parallel()

	cfg, err := Load(t.Context(), examples())
	require.NoError(t, err)
	assert.Equal(t,
		Entries{"x11-drivers/nvidia-drivers", "sys-kernel/gentoo-sources", "sys-kernel/linux-headers"},
		cfg.Steps.Update.Exclude)
	assert.Equal(t, Entries{"orbintsoft"}, cfg.Steps.Sync.AlwaysSync)
	assert.Equal(t, Entries{"EDITOR", "VISUAL"}, cfg.Elevation.KeepEnv)
}

// commentedSetting is a setting the example main file shows commented out:
// "#key = value", where a comment in prose starts with "# ".
var commentedSetting = regexp.MustCompile(`(?m)^#([a-z][a-z0-9-]* = )`)

// TestExampleMainFileDocumentsEveryDefault keeps the example main file in
// step with the code: with its settings uncommented it must give exactly the
// generic defaults, and it must show every value of the schema.
func TestExampleMainFileDocumentsEveryDefault(t *testing.T) {
	t.Parallel()

	data, err := fs.ReadFile(examples(), MainFile)
	require.NoError(t, err)

	uncommented := commentedSetting.ReplaceAll(data, []byte("$1"))

	cfg, err := Load(t.Context(), fstest.MapFS{MainFile: &fstest.MapFile{Data: uncommented}})
	require.NoError(t, err)
	assert.Equal(t, Default(), cfg)

	shown := fileKeys(uncommented)

	// The custom steps and the hooks have no defaults: the file shows an
	// example of each instead.
	for key, typ := range schemaKeys(reflect.TypeFor[Config]()) {
		isExample := slices.ContainsFunc([]string{customStepsTable, hooksTable}, func(table string) bool {
			return key == table || strings.HasPrefix(key, table+".")
		})
		if !isExample && (typ.Kind() != reflect.Struct || isTextValue(typ)) {
			assert.Contains(t, shown, key, "the example main file does not show %s", key)
		}
	}
}

// exampleHook matches a hook the example main file shows: the lines of a
// commented "# [hooks.<name>]" table, each starting with "# ".
var exampleHook = regexp.MustCompile(`(?m)^# (\[hooks\.[a-z0-9-]+\]\n)((?:# .+\n)+)`)

// TestExampleMainFileShowsValidHooks keeps the example hooks of the main
// file valid: uncommented, they are hooks that run, a snapshot before the
// update and a backup check before the run, whose remedy is for one exit
// status only and runs only when someone answers.
func TestExampleMainFileShowsValidHooks(t *testing.T) {
	t.Parallel()

	data, err := fs.ReadFile(examples(), MainFile)
	require.NoError(t, err)

	var hooks strings.Builder

	for _, match := range exampleHook.FindAllSubmatch(data, -1) {
		hooks.WriteString(string(match[1]) + strings.ReplaceAll(string(match[2]), "# ", "") + "\n")
	}

	cfg, err := Load(t.Context(), fstest.MapFS{MainFile: &fstest.MapFile{Data: []byte(hooks.String())}})
	require.NoError(t, err)
	require.Len(t, cfg.Hooks, 2)

	snapshot := cfg.Hooks["snapshot"]
	assert.True(t, snapshot.Enabled)
	assert.Equal(t, HookPlace("before-update"), snapshot.At)
	assert.NotEmpty(t, snapshot.Command)

	backup := cfg.Hooks["backup"]
	assert.True(t, backup.Enabled)
	assert.Equal(t, HookPlace("before-run"), backup.At)
	assert.Equal(t, HookAsk, backup.OnFailure)
	assert.NotEmpty(t, backup.Remedy)
	assert.True(t, backup.RemedyElevated)
	assert.True(t, backup.RemedyInteractiveOnly)
	assert.Equal(t, []int{1}, backup.RemedyStatus)
}

// exampleCustomStep matches the custom step the example main file shows:
// the lines of a commented "# [custom-steps.<name>]" table, each starting
// with "# ".
var exampleCustomStep = regexp.MustCompile(`(?m)^# (\[custom-steps\.[a-z0-9-]+\]\n)((?:# .+\n)+)`)

// TestExampleMainFileShowsAValidCustomStep keeps the example custom step of
// the main file valid: uncommented, it is a custom step that runs, elevated
// and optional, after the preserved-library rebuild.
func TestExampleMainFileShowsAValidCustomStep(t *testing.T) {
	t.Parallel()

	data, err := fs.ReadFile(examples(), MainFile)
	require.NoError(t, err)

	match := exampleCustomStep.FindSubmatch(data)
	require.NotNil(t, match, "the example main file shows no custom step")

	step := string(match[1]) + strings.ReplaceAll(string(match[2]), "# ", "")

	cfg, err := Load(t.Context(), fstest.MapFS{MainFile: &fstest.MapFile{Data: []byte(step)}})
	require.NoError(t, err)
	require.Len(t, cfg.CustomSteps, 1)

	for _, each := range cfg.CustomSteps {
		assert.True(t, each.Enabled)
		assert.Equal(t, "preserved-rebuild", each.After)
		assert.Equal(t, []string{"haskell-updater", "--quiet", "--", "--verbose"}, each.Command)
		assert.True(t, each.Elevated)
		assert.True(t, each.Optional)
	}
}
