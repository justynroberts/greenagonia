#!/usr/bin/env bash
# install.sh — install greenagonia binary + terraform on a Linux x86_64 server
set -euo pipefail

REPO="justynroberts/greenagonia"
TF_VERSION="1.9.8"
INSTALL_DIR="/usr/local/bin"

# ---------------------------------------------------------------------------
# helpers
# ---------------------------------------------------------------------------

info()  { echo "  --> $*"; }
die()   { echo "error: $*" >&2; exit 1; }

need() {
    command -v "$1" >/dev/null 2>&1 || die "$1 is required but not installed"
}

need curl
need unzip

# ---------------------------------------------------------------------------
# greenagonia binary
# ---------------------------------------------------------------------------

info "fetching latest greenagonia release..."

LATEST=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\(.*\)".*/\1/')

if [[ -z "$LATEST" ]]; then
    # No release tags yet — download the pre-built linux binary from the repo root
    info "no release found; downloading greenagonia-linux-amd64 from main branch..."
    curl -fsSL "https://raw.githubusercontent.com/${REPO}/main/greenagonia-linux-amd64" \
        -o /tmp/greenagonia
else
    info "downloading greenagonia ${LATEST}..."
    curl -fsSL "https://github.com/${REPO}/releases/download/${LATEST}/greenagonia-linux-amd64" \
        -o /tmp/greenagonia
fi

chmod +x /tmp/greenagonia
sudo mv /tmp/greenagonia "${INSTALL_DIR}/greenagonia"
info "greenagonia installed → ${INSTALL_DIR}/greenagonia"
greenagonia --help | head -3

# ---------------------------------------------------------------------------
# terraform
# ---------------------------------------------------------------------------

if command -v terraform >/dev/null 2>&1; then
    INSTALLED=$(terraform version -json 2>/dev/null | grep '"terraform_version"' | sed 's/.*"\(.*\)".*/\1/' || terraform version | head -1)
    info "terraform already installed: ${INSTALLED}"
else
    info "installing terraform ${TF_VERSION}..."
    TF_ZIP="terraform_${TF_VERSION}_linux_amd64.zip"
    curl -fsSL "https://releases.hashicorp.com/terraform/${TF_VERSION}/${TF_ZIP}" -o /tmp/${TF_ZIP}
    unzip -q /tmp/${TF_ZIP} -d /tmp/tf_extract
    sudo mv /tmp/tf_extract/terraform "${INSTALL_DIR}/terraform"
    rm -rf /tmp/${TF_ZIP} /tmp/tf_extract
    info "terraform installed → ${INSTALL_DIR}/terraform"
    terraform version
fi

echo ""
echo "done. run 'greenagonia setup' to configure."
