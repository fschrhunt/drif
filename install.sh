#!/bin/sh
# Installs drift into ~/.local/bin (or $DRIFT_INSTALL_DIR) and links git-drift
# beside it so `git drift` resolves too. Run it again to update.
#
#   curl -fsSL https://raw.githubusercontent.com/fschrhunt/drift/main/install.sh | sh
#   DRIFT_VERSION=v0.1.0 ./install.sh
set -eu

dir=${DRIFT_INSTALL_DIR:-$HOME/.local/bin}
version=${DRIFT_VERSION:-latest}

command -v go >/dev/null 2>&1 || {
    echo "drift install: Go is required to build from source" >&2
    exit 1
}

mkdir -p "$dir"
GOBIN="$dir" go install "github.com/fschrhunt/drift/cmd/drift@${version}"
ln -sf drift "$dir/git-drift"
echo "installed $dir/drift and $dir/git-drift"
