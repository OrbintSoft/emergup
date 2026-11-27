#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# A stand-in for smart-live-rebuild in the integration tests. With
# --pretend, as emergup runs it, it says on its standard error that it
# checks the upstreams, and lists on its standard output the live package
# of the test repository when /var/tmp/emergup-test-upstream exists, as if
# its upstream had changed. It must run as root, as smart-live-rebuild
# updates the clones of the repositories; without --pretend it fails, since
# emergup has emerge rebuild the packages itself. Each time it lists, it
# creates /var/tmp/emergup-test-smart-ran.

set -euo pipefail

if [[ "${1-}" != --pretend ]]; then
	echo "smart-live-rebuild: run without --pretend" >&2
	exit 99
fi

if [[ "$(id -u)" != 0 ]]; then
	echo "smart-live-rebuild: not run as root" >&2
	exit 98
fi

echo ">>> Updating the repositories..." >&2
touch /var/tmp/emergup-test-smart-ran

if [[ -e /var/tmp/emergup-test-upstream ]]; then
	echo "emergup-test/live:0"
fi
