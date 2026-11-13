#!/bin/sh
# REL-3: runs the checks behind doc 12's targets and prints the results as text to
# paste into doc 12. Run it on the i3 reference machine (or a VM limited to match it).
# It does nothing over the network (the Go test cache and modules must already be
# there: run `go mod download` first on a connected machine if they aren't).
#
# Numbers from any other machine are not i3 results, and the header says whose they are.
#
# Usage: scripts/reference-check.sh [-count N]   (default N=3 for the benchmarks)
set -eu

cd "$(dirname "$0")/.."
count=3
[ "${1:-}" != -count ] || count=${2:?usage: reference-check.sh [-count N]}

echo "## Reference check, $(date -u +%Y-%m-%d)"
echo
echo "Machine:"
echo "- OS: $(uname -sr), $(uname -m)"
case $(uname -s) in
Linux)
	echo "- CPU: $(awk -F': *' '/model name/ { print $2; exit }' /proc/cpuinfo), $(grep -c '^processor' /proc/cpuinfo) logical cores"
	echo "- RAM: $(awk '/MemTotal/ { printf "%.1f GB", $2 / 1048576 }' /proc/meminfo)"
	;;
Darwin)
	echo "- CPU: $(sysctl -n machdep.cpu.brand_string), $(sysctl -n hw.logicalcpu) logical cores"
	echo "- RAM: $(($(sysctl -n hw.memsize) / 1073741824)) GB"
	;;
esac
echo "- Go: $(go version)"
echo "- Commit: $(git rev-parse --short HEAD 2>/dev/null || echo unknown)$(git diff --quiet 2>/dev/null || echo ' (with uncommitted changes)')"
echo

# run TITLE PACKAGE PATTERN KIND   (KIND is bench or test)
run() {
	echo "### $1"
	echo '```text'
	if [ "$4" = bench ]; then
		go test -run=NONE -bench "$3" -benchmem -count "$count" "$2" 2>&1 | grep -E '^(Benchmark|ok|FAIL|---|panic)' || true
	else
		go test -run "$3" -count=1 -v "$2" 2>&1 | grep -E '^(\s+.*(MB|ms)|---|ok|FAIL|panic)' || true
	fi
	echo '```'
	echo
}

# 100 MB file open: under 100 MB extra, usable within 1 second.
run "100 MB file: open, scattered edit, draw (internal/buffer)" ./internal/buffer 'Open100MB|ScatteredEdit100MB|Draw60Lines100MB|OpenFile100MB' bench
run "100 MB file: frame, count matches (internal/editor)" ./internal/editor 'Frame100MB|CountMatches100MB' bench
run "100 MB file: draw a frame (internal/app)" ./internal/app 'DrawFrame100MB' bench
# Key press to screen (under 16 ms): multi-cursor typing and a long line, then a highlighted 1 MiB frame.
run "Typing and frames (internal/editor)" ./internal/editor 'Type10kCursors|FrameLongLine' bench
run "Highlighted 1 MiB frame (internal/app)" ./internal/app 'FrameOfAHighlightedMiB' bench
run "Highlight update after one key (internal/syntax)" ./internal/syntax 'UpdateOneMiB' bench
# Directory search at 50,000 files.
run "Search 50,000 files (internal/search)" ./internal/search 'Search50000Files' bench
# Editor RAM under 300 MB with ten terminal tabs. CI fails above the limit; the log has the numbers. Linux only.
run "Memory with terminal tabs (cmd/lazycode)" ./cmd/lazycode 'TestMemoryStaysUnderLimit' test

# Startup to usable: 100 ms. Directory or project switch: 50 ms.
run "Startup, real binary (cmd/lazycode)" ./cmd/lazycode 'TestStartupTime' test
run "Project switch (internal/app)" ./internal/app 'SwitchProject' bench

echo "### Not covered here"
echo "- Download size and installed size: check the built binary with ls -l."
echo "- Key press to screen over 16 ms on the real terminal: only a person can judge that."
