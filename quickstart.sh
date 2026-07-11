#!/usr/bin/env bash
# ==========================================================================
# Greenagonia quickstart — gets a MacBook from zero to a built CLI.
#
#   git clone https://github.com/justynroberts/greenagonia.git \
#     && cd greenagonia && ./quickstart.sh
#
# What it does:
#   1. Verifies prerequisites (listed below), installing terraform/go via
#      Homebrew when missing.
#   2. Builds the greenagonia CLI to ./greenagonia (repo root).
#   3. Prints the next steps to run.
#
# Prerequisites:
#   - macOS (Apple Silicon or Intel); Linux works too if brew is present
#   - Homebrew                  https://brew.sh
#   - Terraform >= 1.5          installed automatically via brew if missing
#   - Go >= 1.25                installed automatically via brew if missing
#   - A PagerDuty account with an admin REST API key
#       create at: Integrations -> API Access Keys  (Read/Write access)
# ==========================================================================
set -euo pipefail

GREEN='\033[38;5;35m'; BOLD='\033[1m'; DIM='\033[2m'; RED='\033[31m'; NC='\033[0m'
say()  { printf "  ${GREEN}●${NC} %b\n" "$1"; }
warn() { printf "  ${RED}✗${NC} %b\n" "$1"; }
hr()   { printf "  ${DIM}%s${NC}\n" "────────────────────────────────────────────────────"; }

printf "\n  ${GREEN}●${NC}  ${BOLD}greenagonia${NC}  ${DIM}· quickstart${NC}\n"
hr
echo

# --- prerequisites ---------------------------------------------------------
printf "  ${BOLD}prerequisites${NC}\n"
printf "  ${DIM}macOS + Homebrew, Terraform >=1.5, Go >=1.25,${NC}\n"
printf "  ${DIM}a PagerDuty admin API key.${NC}\n"
echo

if ! command -v brew >/dev/null 2>&1; then
  warn "Homebrew not found. Install it first:"
  printf '\n    /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"\n\n'
  printf "  then re-run ./quickstart.sh\n\n"
  exit 1
fi
say "Homebrew $(brew --version | head -1 | awk '{print $2}')"

if ! command -v terraform >/dev/null 2>&1; then
  say "Terraform not found — installing via brew…"
  brew install terraform >/dev/null
fi
say "Terraform $(terraform version -json 2>/dev/null | sed -n 's/.*"terraform_version": *"\([^"]*\)".*/\1/p' | head -1)"

if ! command -v go >/dev/null 2>&1; then
  say "Go not found — installing via brew…"
  brew install go >/dev/null
fi
say "Go $(go version | awk '{print $3}' | sed 's/^go//')"
echo

# --- build -----------------------------------------------------------------
printf "  ${BOLD}building${NC}\n"
( cd cli && go build -trimpath -ldflags "-s -w" -o ../greenagonia . )
say "built ./greenagonia"
echo

# --- next steps ------------------------------------------------------------
printf "  ${BOLD}next steps${NC}\n\n"
printf "    ${DIM}# 1. Run the setup wizard (tokens, time zone, first admin)${NC}\n"
printf "    ./greenagonia setup\n\n"
printf "    ${DIM}# 2. Deploy the shared environment to PagerDuty${NC}\n"
printf "    ./greenagonia deploy\n\n"
printf "    ${DIM}# 3. Get per-admin storefront links${NC}\n"
printf "    ./greenagonia urls\n\n"
printf "    ${DIM}# 4. Serve the storefront locally${NC}\n"
printf "    cd site && python3 -m http.server 8080\n\n"
printf "    ${DIM}full walkthrough: ADMIN-GUIDE.md${NC}\n"
echo
hr
printf "  ${DIM}docs: README.md · ADMIN-GUIDE.md${NC}\n\n"
