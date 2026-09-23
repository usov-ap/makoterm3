#!/usr/bin/env sh
# Wrapper for sandboxed/offline environments: keeps the Go build and module
# caches inside the repository so that no writes outside the workspace are
# needed. Normal development does not need this file.
#
# Usage:   ./scripts/go.sh test ./...
#          ./scripts/go.sh build -o makoterm
#
# For an interactive shell, export the variables instead:
#          . ./scripts/goenv.sh

set -eu

_self=$0
_repo_root=$(CDPATH= cd -- "$(dirname -- "$_self")/.." && pwd)

if [ ! -f "$_repo_root/go.mod" ]; then
	echo "go.sh: cannot locate repository root (looked in $_repo_root)" >&2
	exit 1
fi

GOCACHE="$_repo_root/.tmp/gocache"
GOMODCACHE="$_repo_root/.tmp/gomodcache"
GOPATH="$_repo_root/.tmp/gopath"
GOFLAGS="${GOFLAGS:--mod=mod}"
export GOCACHE GOMODCACHE GOPATH GOFLAGS

mkdir -p "$GOCACHE" "$GOMODCACHE" "$GOPATH"

exec go "$@"
