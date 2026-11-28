#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# The AI of the integration tests, a command backend. It saves the request
# it reads on its standard input to /tmp/emergup-test-ai-request, after a
# first line naming the user it runs as, and answers with two fixed lines.

set -euo pipefail

{
	printf 'user=%s\n' "$(id -un)"
	cat
} >/tmp/emergup-test-ai-request

echo "The compile phase of emergup-test/upgrade-1.2 dies: this version does not build."
echo "This answer comes from the AI of the integration tests."
