#!/bin/sh
# Builds this checkout and installs it as `lazycode`, then says what you got. Works from any folder.
# Usage: scripts/install-dev.sh
set -eu

cd "$(dirname "$0")/.."
target="$HOME/.local/bin/lazycode"
mkdir -p "$(dirname "$target")"
go build -o "$target" ./cmd/lazycode
echo "Installed $("$target" --version) to $target"

found=$(command -v lazycode || true)
if [ "$found" != "$target" ]; then
	echo "Warning: typing lazycode runs ${found:-nothing}, not this one. Put $(dirname "$target") first on your PATH." >&2
fi
echo "Restart any LazyCode that's running. If this shell still finds an old copy, run: hash -r"
