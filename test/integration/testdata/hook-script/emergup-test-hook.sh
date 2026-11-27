#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# The hook of the integration tests. It appends to /tmp/emergup-test-hooks
# a line with the context emergup gives it, then does each operation its
# arguments give, in order:
#
#   touch:PATH  creates PATH
#   need:PATH   exits with status 1 unless PATH exists
#   mounted:PATH
#               exits with status 2 unless PATH exists, as a check whose
#               backup disk is missing
#   exit:N      exits with status N
#
# A variable every hook gets that is missing makes it fail.

set -euo pipefail

printf '%s %s %s status=%s result=%s user=%s dir=%s log=%s\n' \
	"$EMERGUP_HOOK" "$EMERGUP_WHEN" "$EMERGUP_STEP" \
	"${EMERGUP_STEP_STATUS-}" "${EMERGUP_RESULT-}" "$(id -un)" \
	"$EMERGUP_RUN_DIR" "$EMERGUP_LOG" >>/tmp/emergup-test-hooks

for operation in "$@"; do
	case "$operation" in
	touch:*) touch "${operation#touch:}" ;;
	need:*) test -e "${operation#need:}" || exit 1 ;;
	mounted:*) test -e "${operation#mounted:}" || exit 2 ;;
	exit:*) exit "${operation#exit:}" ;;
	*)
		echo "unknown operation: $operation" >&2
		exit 125
		;;
	esac
done
