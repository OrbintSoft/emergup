#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# The hook of the integration tests. It appends to /tmp/emergup-test-hooks
# a line with the context emergup gives it, then exits with the status its
# first argument gives, 0 by default. A variable every hook gets that is
# missing makes it fail.

set -euo pipefail

printf '%s %s %s status=%s result=%s user=%s dir=%s log=%s\n' \
	"$EMERGUP_HOOK" "$EMERGUP_WHEN" "$EMERGUP_STEP" \
	"${EMERGUP_STEP_STATUS-}" "${EMERGUP_RESULT-}" "$(id -un)" \
	"$EMERGUP_RUN_DIR" "$EMERGUP_LOG" >>/tmp/emergup-test-hooks

exit "${1:-0}"
