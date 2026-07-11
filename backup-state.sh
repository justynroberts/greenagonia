#!/usr/bin/env bash
# backup-state.sh — create a dated tar.gz of all terraform state files
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")" && pwd)"
BACKUP_DIR="${REPO_ROOT}/state/backups"
TIMESTAMP=$(date +"%Y%m%d-%H%M%S")
ARCHIVE="${BACKUP_DIR}/state-${TIMESTAMP}.tar.gz"

mkdir -p "$BACKUP_DIR"

# Collect state files from both terraform dirs and the root state/ dir
tar -czf "$ARCHIVE" \
    -C "$REPO_ROOT" \
    --exclude="state/backups" \
    $(find state terraform terraform-shared -name "*.tfstate" -o -name "*.tfstate.backup" 2>/dev/null | sort) \
    2>/dev/null || true

SIZE=$(du -sh "$ARCHIVE" | cut -f1)
echo "  backup → ${ARCHIVE} (${SIZE})"

# Keep only the 10 most recent backups
ls -t "${BACKUP_DIR}"/state-*.tar.gz 2>/dev/null | tail -n +11 | xargs -r rm --
KEPT=$(ls "${BACKUP_DIR}"/state-*.tar.gz 2>/dev/null | wc -l)
echo "  ${KEPT} backup(s) retained in ${BACKUP_DIR}"
