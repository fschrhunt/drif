#!/bin/sh
# Installs drif into ~/.local/bin (or $DRIF_INSTALL_DIR) and links git-drif
# beside it so `git drif` resolves too. Run it again to update.
#
#   curl -fsSL https://raw.githubusercontent.com/fschrhunt/drif/main/install.sh | sh
#   DRIF_VERSION=v0.1.0 ./install.sh
set -eu

dir=${DRIF_INSTALL_DIR:-$HOME/.local/bin}
version=${DRIF_VERSION:-latest}

command -v go >/dev/null 2>&1 || {
    echo "drif install: Go is required to build from source" >&2
    exit 1
}

mkdir -p "$dir"
GOBIN="$dir" go install "github.com/fschrhunt/drif/cmd/drif@${version}"
ln -sf drif "$dir/git-drif"
echo "installed $dir/drif and $dir/git-drif"
