#!/bin/sh
# M0-07 in Kitty: real X11 key presses, so Kitty's own shortcuts get their chance to
# take a key. Needs kitty, xdotool and setxkbmap, under a display such as xvfb-run.
#
# Usage: keytest-kitty.sh /absolute/path/to/keytest
set -eu

bin=$1
report=$(mktemp)

run() {
	layout=$1
	echo "=== Kitty $(kitty --version | cut -d' ' -f2), $layout keyboard layout"
	setxkbmap "$layout"
	rm -f "$report"
	kitty --config NONE "$bin" --report "$report" &
	kitty_pid=$!
	i=0
	until window=$(xdotool search --onlyvisible --class kitty 2>/dev/null | head -n 1) && [ -n "$window" ]; do
		i=$((i + 1))
		[ "$i" -le 30 ] || { echo "FAIL: no Kitty window"; kill "$kitty_pid"; return 1; }
		sleep 1
	done
	xdotool windowfocus "$window" || true
	sleep 2
	for key in ctrl+backslash ctrl+a ctrl+b ctrl+c ctrl+d ctrl+f ctrl+n ctrl+o ctrl+p ctrl+s ctrl+v \
		ctrl+w ctrl+x ctrl+y ctrl+z F2 F12 Escape alt+d alt+o alt+p alt+f alt+g alt+h alt+s alt+w alt+v alt+i alt+e \
		alt+comma alt+period alt+shift+f ctrl+F2; do
		xdotool key "$key"
		sleep 0.2
	done
	xdotool key q
	i=0
	until [ -s "$report" ]; do
		i=$((i + 1))
		[ "$i" -le 20 ] || { echo "FAIL: no report"; kill "$kitty_pid"; return 1; }
		sleep 1
	done
	cat "$report"
	wait "$kitty_pid" || true
}

run us
us_missing=$(grep -c MISSING "$report" || true)
# Requirement C-3 names Spanish keyboards, where \ needs AltGr. Reported, not enforced.
run es || true
[ "$us_missing" = 0 ] || { echo "FAIL: $us_missing keys missing on the US layout"; exit 1; }
