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
#   2. Builds the greenagonia CLI to ./bin/greenagonia.
#   3. If PAGERDUTY_TOKEN and GREENAGONIA_EMAIL are exported, deploys the
#      PagerDuty environment immediately. Otherwise prints the exact
#      commands to run next.
#
# Prerequisites:
#   - macOS (Apple Silicon or Intel); Linux works too if brew is present
#   - Homebrew                  https://brew.sh
#   - Terraform >= 1.5          installed automatically via brew if missing
#   - Go >= 1.22                installed automatically via brew if missing
#   - A PagerDuty account with an admin REST API key
#       create at: Integrations -> API Access Keys  (Read/Write access)
#   - An email address NOT already registered in that PagerDuty account
#       (the deploy creates a new user with it)
#   - EU PagerDuty accounts (*.eu.pagerduty.com): add --region eu on deploy
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
printf "  ${DIM}macOS + Homebrew, Terraform >=1.5, Go >=1.22,${NC}\n"
printf "  ${DIM}a PagerDuty admin API key, and a fresh on-call email.${NC}\n"
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
mkdir -p bin
( cd cli && go build -trimpath -ldflags "-s -w" -o ../bin/greenagonia . )
say "built ./bin/greenagonia ($(du -h bin/greenagonia | cut -f1 | tr -d ' '))"
echo

# --- deploy or print next steps ---------------------------------------------
if [[ -n "${PAGERDUTY_TOKEN:-}" && -n "${GREENAGONIA_EMAIL:-}" ]]; then
  printf "  ${BOLD}deploying${NC} ${DIM}(PAGERDUTY_TOKEN + GREENAGONIA_EMAIL detected)${NC}\n\n"
  ./bin/greenagonia deploy \
    --env "${GREENAGONIA_ENV:-prod}" \
    --email "$GREENAGONIA_EMAIL" \
    ${GREENAGONIA_REGION:+--region "$GREENAGONIA_REGION"}
  echo
  say "now try:  ./bin/greenagonia web --env ${GREENAGONIA_ENV:-prod}"
else
  printf "  ${BOLD}next steps${NC}\n\n"
  printf "    ${DIM}# 1. PagerDuty admin API key (Integrations -> API Access Keys)${NC}\n"
  printf "    export PAGERDUTY_TOKEN=<your-key>\n\n"
  printf "    ${DIM}# 2. deploy (creates users/services/orchestration in PagerDuty)${NC}\n"
  printf "    ./bin/greenagonia deploy --env prod --email you@example.com\n"
  printf "    ${DIM}#    EU account? add: --region eu${NC}\n\n"
  printf "    ${DIM}# 3. open the trigger site${NC}\n"
  printf "    ./bin/greenagonia web --env prod\n\n"
  printf "    ${DIM}full walkthrough: ./bin/greenagonia setup${NC}\n"
fi
echo
hr
printf "  ${DIM}docs: README.md · guided setup: ./bin/greenagonia setup${NC}\n\n"
