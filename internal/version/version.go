// SPDX-License-Identifier: GPL-3.0-or-later

// Package version reports the version of this emergup build.
package version

import "runtime/debug"

// unknown is reported when the build carries no version at all.
const unknown = "unknown"

// stamped is set at link time by whoever builds a release, for example
//
//	go build -ldflags "-X github.com/OrbintSoft/emergup/internal/version.stamped=1.2.3"
//
// It is empty in an ordinary go build or go install.
var stamped string

// String returns the version of this build: the value stamped at link time,
// else the module version the Go toolchain recorded (a release tag for
// go install, a pseudo-version for a build from a git checkout), else
// "unknown".
func String() string {
	return resolve(stamped, debug.ReadBuildInfo)
}

func resolve(stamped string, readBuildInfo func() (*debug.BuildInfo, bool)) string {
	if stamped != "" {
		return stamped
	}
	if info, ok := readBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return unknown
}
