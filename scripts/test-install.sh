#!/bin/sh
# Tests install.sh against a fake release served from this machine. It builds the
# real binary, serves it with python3, and installs into a temporary HOME. It never
# touches your real shell files or ~/.local/bin.
#
# Usage: scripts/test-install.sh
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
server=
trap '[ -z "$server" ] || kill "$server" 2>/dev/null; rm -rf "$work"' EXIT

case $(uname -s) in Linux) os=linux ;; Darwin) os=darwin ;; esac
case $(uname -m) in x86_64 | amd64) arch=amd64 ;; *) arch=arm64 ;; esac
asset=lazycode_${os}_${arch}
tag=v0.0.0-installtest

sum() { if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1"; else shasum -a 256 "$1"; fi | awk '{ print $1 }'; }
pass() { echo "ok: $*"; }
fail() { echo "FAIL: $*" >&2; exit 1; }

# Three fake releases: good, one whose binary doesn't match its checksum, and one
# whose checksums.txt has no entry for the binary.
mkdir -p "$work/www/good"
(cd "$root" && go build -trimpath -ldflags "-X main.version=$tag" -o "$work/www/good/$asset" ./cmd/lazycode)
cp "$root/packaging/install-icon.sh" "$work/www/good/"
for f in "$asset" install-icon.sh; do printf '%s  %s\n' "$(sum "$work/www/good/$f")" "$f"; done >"$work/www/good/checksums.txt"

cp -R "$work/www/good" "$work/www/badsum"
printf 'tampered' >>"$work/www/badsum/$asset"
cp -R "$work/www/good" "$work/www/noentry"
grep -v " $asset\$" "$work/www/good/checksums.txt" >"$work/www/noentry/checksums.txt"

port=$(python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])')
python3 -m http.server "$port" --bind 127.0.0.1 --directory "$work/www" >/dev/null 2>&1 &
server=$!
i=0
until curl -fsS -o /dev/null "http://127.0.0.1:$port/good/checksums.txt" 2>/dev/null; do
	i=$((i + 1))
	[ "$i" -lt 50 ] || fail "the test server didn't start"
	sleep 0.1
done

# install RELEASE HOME [VAR=value ...] runs install.sh the way the README does, through a pipe.
install() {
	rel=$1 home=$2
	shift 2
	mkdir -p "$home"
	cat "$root/install.sh" | env -i PATH=/usr/bin:/bin HOME="$home" SHELL=/bin/bash XDG_DATA_HOME="$home/.local/share" \
		LAZYCODE_BASE_URL="http://127.0.0.1:$port/$rel" "$@" sh >"$work/out" 2>&1
}

# 1. A good install works, and the binary runs.
h=$work/h1
install good "$h" || { cat "$work/out"; fail "good install failed"; }
bin=$h/.local/bin/lazycode
[ -x "$bin" ] || fail "binary not installed"
[ "$(sum "$bin")" = "$(sum "$work/www/good/$asset")" ] || fail "installed binary differs from the release"
[ "$(stat -c %a "$bin" 2>/dev/null || stat -f %Lp "$bin")" = 755 ] || fail "mode isn't 755"
grep -q "127.0.0.1:$port/good" "$work/out" || fail "doesn't say where it downloads from"
if grep -q 'main.version' "$root/cmd/lazycode/main.go"; then
	[ "$("$bin" --version | grep -c "$tag")" -ge 1 ] || fail "--version doesn't report $tag: $("$bin" --version)"
	pass "lazycode --version reports $tag"
else
	echo "NOTE: cmd/lazycode/main.go has no version variable yet, so --version wasn't checked"
fi
pass "installed, executable, mode 755, same bytes as the release"
[ "$(grep -c 'added by the lazycode installer' "$h/.bashrc")" = 1 ] || fail "PATH line not added once"
pass "PATH line added to .bashrc"
if [ "$os" = linux ]; then
	grep -q '^Exec=' "$h/.local/share/applications/lazycode.desktop" || fail "no desktop entry"
	pass "app icon created"
else
	[ -d "$h/Applications/LazyCode.app" ] || fail "no app icon"
	pass "app icon created"
fi

# 2. Running it again doesn't add the line twice, and replaces the binary.
install good "$h" || { cat "$work/out"; fail "second install failed"; }
[ "$(grep -c 'added by the lazycode installer' "$h/.bashrc")" = 1 ] || fail "PATH line added twice"
pass "second run leaves one PATH line"

# 3. A wrong checksum is refused and nothing is installed.
h=$work/h2
if install badsum "$h"; then fail "bad checksum was accepted"; fi
grep -q 'checksum mismatch' "$work/out" || fail "no mismatch message: $(cat "$work/out")"
[ ! -e "$h/.local/bin/lazycode" ] || fail "binary installed despite bad checksum"
[ ! -e "$h/.bashrc" ] || fail "shell file changed despite bad checksum"
pass "bad checksum refused, nothing installed"

# 4. A missing checksum entry is refused.
h=$work/h3
if install noentry "$h"; then fail "missing entry was accepted"; fi
grep -q 'no entry' "$work/out" || fail "no missing-entry message: $(cat "$work/out")"
[ ! -e "$h/.local/bin/lazycode" ] || fail "binary installed despite missing entry"
pass "missing checksum entry refused, nothing installed"

# 5. LAZYCODE_NO_MODIFY_PATH leaves shell files alone.
h=$work/h4
install good "$h" LAZYCODE_NO_MODIFY_PATH=1 || { cat "$work/out"; fail "install with opt-out failed"; }
[ -x "$h/.local/bin/lazycode" ] || fail "binary not installed"
[ ! -e "$h/.bashrc" ] || fail "LAZYCODE_NO_MODIFY_PATH ignored"
pass "LAZYCODE_NO_MODIFY_PATH=1 leaves shell files alone"

# 6. No hash tool: refuse. The PATH here has curl and uname, and no sha256sum or shasum.
h=$work/h5
mkdir "$work/nohash"
for t in curl uname; do ln -s "$(command -v $t)" "$work/nohash/$t"; done
if cat "$root/install.sh" | env -i PATH="$work/nohash" HOME="$h" LAZYCODE_BASE_URL="http://127.0.0.1:$port/good" /bin/sh >"$work/out" 2>&1; then
	fail "installed without a hash tool"
fi
grep -q "can't be verified" "$work/out" || fail "no hash-tool message: $(cat "$work/out")"
[ ! -e "$h/.local/bin/lazycode" ] || fail "installed without a hash tool"
pass "refuses when there is no hash tool"

echo "PASS"
