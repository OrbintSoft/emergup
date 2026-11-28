#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# The Claude Code of the integration tests. Each call appends to
# /tmp/emergup-test-claude a line with the user it runs as, its working
# directory and its arguments. With -p it saves the request it reads on
# its standard input to /tmp/emergup-test-claude-request-<session> and
# answers with the events of Claude Code's stream-json output; otherwise it
# is a chat that writes one line and ends.

set -euo pipefail

printf 'user=%s dir=%s args=%s\n' "$(id -un)" "$PWD" "$*" >>/tmp/emergup-test-claude

session=""
print=false
previous=""

for argument in "$@"; do
	case "$previous" in
	--session-id | --resume) session="$argument" ;;
	esac

	if [[ "$argument" == "-p" ]]; then
		print=true
	fi

	previous="$argument"
done

if ! "$print"; then
	echo "The chat of the integration tests, in session $session."
	exit 0
fi

cat >"/tmp/emergup-test-claude-request-$session"

event() {
	printf '%s\n' "$1"
}

event '{"type":"system","subtype":"init","session_id":"'"$session"'"}'
event '{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"The compile phase of "}},"session_id":"'"$session"'"}'
event '{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"emergup-test/upgrade-1.2 dies.\n"}},"session_id":"'"$session"'"}'
event '{"type":"assistant","message":{"content":[{"type":"text","text":"not shown twice"}]},"session_id":"'"$session"'"}'
event '{"type":"result","subtype":"success","is_error":false,"result":"done","session_id":"'"$session"'"}'
