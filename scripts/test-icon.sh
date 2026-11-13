#!/bin/sh
# M0-06 on Linux and macOS: installs the app icon, opens it as the system would,
# and checks that lazycode really started. On Linux, run it under a display.
#
# Usage: test-icon.sh /absolute/path/to/lazycode
set -eu

bin=$1
installed=$(sh "$(dirname "$0")/../packaging/install-icon.sh" "$bin")
echo "Installed: $installed"
case $(uname -s) in
Linux)
	desktop-file-validate "$installed"
	gio launch "$installed" &
	;;
Darwin)
	open "$installed"
	;;
esac

i=0
while pid=$(pgrep -x lazycode | head -n 1) && [ -z "$pid" ]; do
	i=$((i + 1))
	if [ "$i" -gt 30 ]; then
		echo "FAIL: lazycode never started" >&2
		exit 1
	fi
	sleep 1
done
echo "lazycode started, pid $pid. Its parents:"
parent=$pid
for _ in 1 2 3; do
	parent=$(ps -o ppid= -p "$parent" | tr -d ' ')
	[ "$parent" -gt 1 ] || break
	ps -o pid=,comm= -p "$parent"
done
kill "$pid"
echo "PASS"
