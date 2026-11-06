#!/bin/sh
# Lists every place LazyCode's own Go source (not tests) can make a network
# connection or start another program, so the owner can check doc 11's rule: the
# only automatic connection is the update check. It reads source and runs
# `go list`; it makes no network connection itself.
#
# Usage: scripts/privacy-check.sh
set -eu

cd "$(dirname "$0")/.."
mod=$(go list -m)
src=$(find cmd internal -name '*.go' ! -name '*_test.go' | sort)

echo "## 1. Packages in the build that import net/http or net"
echo "Our own packages, and the dependencies that bring them in."
for p in net/http net; do
	echo
	echo "Imports of $p:"
	for goos in linux darwin windows; do
		GOOS=$goos GOARCH=amd64 CGO_ENABLED=1 go list -deps -f "{{range .Imports}}{{if eq . \"$p\"}}{{\$.ImportPath}}{{\"\\n\"}}{{end}}{{end}}" ./cmd/lazycode
	done | sort -u | sed 's/^/  /'
done
echo
echo "Of those, ours (the rest are standard library or third-party):"
GOOS=linux GOARCH=amd64 go list -deps -f '{{if not .Standard}}{{.ImportPath}} {{join .Imports " "}}{{end}}' ./cmd/lazycode |
	awk -v m="$mod" 'index($1, m) == 1 { for (i = 2; i <= NF; i++) if ($i == "net/http" || $i == "net") print "  " $1 " imports " $i }' | sort -u

echo
echo "## 2. Source lines that connect to the network"
# shellcheck disable=SC2086
grep -nE '"net/http"|"net"|net\.(Dial|Listen)|http\.(Get|Post|Head|NewRequest|Client|DefaultClient)|websocket|\.Dial\(|"net/url"|api\.github\.com|github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+/(releases|archive)|https?://' $src | grep -vE '^\S+:[0-9]+:\s*//' || echo "  none"

echo
echo "## 3. Source lines that start another program"
# shellcheck disable=SC2086
grep -nE '"os/exec"|exec\.(Command|CommandContext|LookPath)|os\.StartProcess|syscall\.(Exec|ForkExec)|pty\.(Start|Command)|\.Start\(\)' $src | grep -vE '^\S+:[0-9]+:\s*//' || echo "  none"

echo
echo "## 4. Which programs those are (string literals passed near exec, git, gh, and language servers)"
# shellcheck disable=SC2086
grep -nE '"(git|gh|gopls|pyright|pyright-langserver|typescript-language-server|npm|pip|pip3|go|curl|wget|xdg-open|open|start)"' $src | grep -vE '^\S+:[0-9]+:\s*//' || echo "  none"

echo
echo "## For the owner to check by hand"
echo "- Every item in 2 should be the update check, or a user's own action (a store, an install the user confirmed)."
echo "- Every item in 3 and 4 starts a program the user's action chose: their shell, an AI tool, git, gh, a language server, or their play command."
echo "- git and gh talk to the network only when the user pushes, pulls, fetches, or runs :repo create."
