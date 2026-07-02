#!/bin/sh
# Build and install gdbg into $GOBIN (or $HOME/go/bin).
set -e

if ! command -v go >/dev/null 2>&1; then
    echo "error: go is not installed (https://go.dev/dl/)" >&2
    exit 1
fi

cd "$(dirname "$0")"
go install ./cmd/gdbg

BIN="$(go env GOBIN)"
[ -z "$BIN" ] && BIN="$(go env GOPATH)/bin"
echo "installed $BIN/gdbg"

command -v dlv >/dev/null 2>&1 || \
    echo "note: dlv not found — go install github.com/go-delve/delve/cmd/dlv@latest"
command -v gopls >/dev/null 2>&1 || \
    echo "note: gopls not found (needed for def/hover/refs) — go install golang.org/x/tools/gopls@latest"
