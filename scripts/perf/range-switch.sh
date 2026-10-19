#!/bin/sh
# Measures range and Plays/Time switches on a page with agent-browser.
#   scripts/perf/range-switch.sh <base-url> [path...]
# Start the server first, e.g. `make mock` (full mock dataset).
set -e
here=$(dirname "$0")
base=${1:?base URL}
shift
for p in "${@:-/}"; do
	echo "== $p (label | url ms | settled ms | long tasks ms | worst frame gap ms)"
	agent-browser open "$base$p" >/dev/null
	agent-browser wait 3000 >/dev/null
	agent-browser eval "$(cat "$here/range-switch.js")" >/dev/null
	agent-browser eval "__suite(false).then(r => r.map(x => [x.label, x.url, x.settle, x.longTasks.join('+') || '-', x.maxFrameGap].join(' | ')).join('\n'))" | sed -e 's/^"//' -e 's/"$//' -e 's/\\n/\n/g'
done
