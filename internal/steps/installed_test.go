// SPDX-License-Identifier: GPL-3.0-or-later

package steps_test

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/emergup/internal/runner"
	"github.com/OrbintSoft/emergup/internal/runner/runnertest"
	"github.com/OrbintSoft/emergup/internal/steps"
)

func TestInstalledPackagesListsThePackageDatabase(t *testing.T) {
	t.Parallel()

	var fake runnertest.Fake

	fake.Script(runnertest.Answer{Result: runner.Result{Output: []byte("/var/db/pkg\n")}})

	root := fstest.MapFS{
		"var/db/pkg/sys-apps/bar-3/SLOT":                   {Data: []byte("0\n")},
		"var/db/pkg/app-misc/foo-1.2/SLOT":                 {Data: []byte("0\n")},
		"var/db/pkg/app-misc/-MERGING-foo-1.3/SLOT":        {Data: []byte("0\n")},
		"var/db/pkg/app-misc/a-file":                       {Data: []byte("not a package\n")},
		"var/db/pkg/.cache/files":                          {Data: []byte("not a category\n")},
		"var/db/pkg/dev-libs/stable-1.0/CONTENTS":          {Data: []byte("\n")},
		"var/db/pkg/x11-drivers/nvidia-drivers-580.1/SLOT": {Data: []byte("0/580\n")},
	}

	installed, err := steps.InstalledPackages(t.Context(), &fake, root)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"app-misc/foo-1.2", "dev-libs/stable-1.0", "sys-apps/bar-3", "x11-drivers/nvidia-drivers-580.1",
	}, installed, "each package once, in order, none being merged")

	require.Len(t, fake.Commands(), 1)
	assert.Equal(t, queryVDB, fake.Commands()[0], "the database Portage names, asked unprivileged")
}

func TestInstalledPackagesSaysWhenItCannotFindThem(t *testing.T) {
	t.Parallel()

	var fake runnertest.Fake

	fake.Script(runnertest.Answer{Result: runner.Result{ExitCode: 1}})

	_, err := steps.InstalledPackages(t.Context(), &fake, fstest.MapFS{})
	require.ErrorContains(t, err, "could not find the installed packages")
}
