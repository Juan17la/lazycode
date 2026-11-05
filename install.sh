#!/bin/sh
# Installs LazyCode on Linux and macOS.
#
#   curl -fsSL https://raw.githubusercontent.com/Juan17la/lazycode/main/install.sh | sh
#
# It downloads one binary from GitHub Releases, checks its SHA-256 against
# checksums.txt, and puts it in $LAZYCODE_INSTALL_DIR (default ~/.local/bin).
# It never uses sudo. If that folder isn't on your PATH, it adds one marked line
# to your shell's startup file, unless LAZYCODE_NO_MODIFY_PATH=1.
#
# Everything is inside main, which is called on the last line, so a download that
# stops half way runs nothing.
set -eu

main() {
	# LAZYCODE_BASE_URL is only for tests that serve a fake release locally.
	base=${LAZYCODE_BASE_URL:-https://github.com/Juan17la/lazycode/releases/latest/download}
	dir=${LAZYCODE_INSTALL_DIR:-$HOME/.local/bin}
	marker='# added by the lazycode installer'

	case $(uname -s) in
	Linux) os=linux ;;
	Darwin) os=darwin ;;
	*) fail "this script is for Linux and macOS. On Windows, use install.ps1." ;;
	esac
	case $(uname -m) in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	*) fail "unsupported CPU: $(uname -m). LazyCode is built for amd64 and arm64." ;;
	esac
	asset=lazycode_${os}_${arch}

	command -v curl >/dev/null 2>&1 || fail "curl is needed to download LazyCode."
	if command -v sha256sum >/dev/null 2>&1; then
		hasher="sha256sum"
	elif command -v shasum >/dev/null 2>&1; then
		hasher="shasum -a 256"
	else
		fail "no sha256sum or shasum found, so the download can't be verified. Nothing was installed."
	fi

	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT
	trap 'exit 1' HUP INT TERM

	echo "Downloading $asset from $base"
	curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt" || fail "couldn't download checksums.txt."
	fetch "$asset" # The binary.
	fetch install-icon.sh

	mkdir -p "$dir"
	cp "$tmp/$asset" "$dir/.lazycode.new"
	chmod 755 "$dir/.lazycode.new"
	mv -f "$dir/.lazycode.new" "$dir/lazycode"
	echo "Installed $dir/lazycode"

	# A failed icon is only a warning.
	if icon=$(sh "$tmp/install-icon.sh" "$dir/lazycode" 2>&1); then
		echo "Created the app icon: $icon"
	else
		echo "Warning: couldn't create the app icon: $icon" >&2
	fi

	add_to_path
	echo "Done. Run: lazycode"
}

fail() {
	echo "install.sh: $*" >&2
	exit 1
}

# fetch NAME downloads NAME into $tmp and refuses to go on unless its SHA-256 is
# listed in checksums.txt and matches.
fetch() {
	curl -fsSL -o "$tmp/$1" "$base/$1" || fail "couldn't download $1."
	want=$(awk -v f="$1" '{ n = $2; sub(/^\*/, "", n) } n == f { print $1; exit }' "$tmp/checksums.txt")
	[ -n "$want" ] || fail "checksums.txt has no entry for $1. Nothing was installed."
	got=$($hasher "$tmp/$1" | awk '{ print $1 }')
	[ "$got" = "$want" ] || fail "checksum mismatch for $1 (wanted $want, got $got). Nothing was installed."
}

add_to_path() {
	case ":$PATH:" in
	*":$dir:"*) return ;;
	esac
	if [ "${LAZYCODE_NO_MODIFY_PATH:-}" = 1 ]; then
		echo "$dir isn't on your PATH. Add it yourself to run lazycode by name."
		return
	fi
	case $dir in
	*"'"*) echo "$dir isn't on your PATH, and its name has a quote, so it was left alone."; return ;;
	esac
	case $(basename "${SHELL:-}") in
	zsh) rc=${ZDOTDIR:-$HOME}/.zshrc line="export PATH='$dir':\"\$PATH\" $marker" ;;
	bash)
		# macOS terminals start bash as a login shell, which skips .bashrc.
		if [ "$(uname -s)" = Darwin ]; then rc=$HOME/.bash_profile; else rc=$HOME/.bashrc; fi
		line="export PATH='$dir':\"\$PATH\" $marker"
		;;
	fish)
		rc=${XDG_CONFIG_HOME:-$HOME/.config}/fish/config.fish
		line="fish_add_path '$dir' $marker"
		;;
	*)
		echo "$dir isn't on your PATH, and your shell isn't one this script knows. Add it yourself."
		return
		;;
	esac
	if [ -f "$rc" ] && grep -qF "$marker" "$rc"; then
		echo "$rc already puts $dir on your PATH."
		return
	fi
	mkdir -p "$(dirname "$rc")"
	printf '%s\n' "$line" >>"$rc"
	echo "Added this line to $rc so your shell finds lazycode:"
	echo "  $line"
	echo "Open a new terminal, or run: . $rc"
}

main "$@"
