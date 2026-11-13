#!/bin/sh
# M0-07 in macOS Terminal, using simulated key presses. macOS may refuse these
# without Accessibility permission, in which case keytest has to be run by hand.
#
# Usage: keytest-macos.sh /absolute/path/to/keytest
set -eu

bin=$1
dir=$(mktemp -d)
report=$dir/report.txt
printf "#!/bin/sh\nexec '%s' --report '%s'\n" "$bin" "$report" >"$dir/keytest.command"
chmod +x "$dir/keytest.command"
open -a Terminal "$dir/keytest.command"
sleep 5

press() { osascript -e "tell application \"System Events\" to $1"; sleep 0.2; }

osascript -e 'tell application "Terminal" to activate'
sleep 1
# The same keys as checks() in cmd/keytest/main.go, for macOS.
press 'key code 42 using control down' # Ctrl+\
press 'key code 49 using control down' # Ctrl+Space
for letter in a b c d f k n o p q r s v w x y z; do
	press "keystroke \"$letter\" using control down"
done
press 'key code 122' # F1
press 'key code 120' # F2
press 'key code 99'  # F3
press 'key code 109' # F10
press 'key code 111' # F12
press 'key code 53'  # Esc
sleep 0.5
press 'keystroke "q"'

i=0
until [ -s "$report" ]; do
	i=$((i + 1))
	if [ "$i" -gt 15 ]; then
		echo "FAIL: no report" >&2
		exit 1
	fi
	sleep 1
done
cat "$report"
if grep -q MISSING "$report"; then
	exit 1
fi
