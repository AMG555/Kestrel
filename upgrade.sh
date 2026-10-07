#!/usr/bin/env bash
set -euo pipefail

# Kestrel in-place update and backup script
#
# Preserves:
# - config.yaml
# - data/ (SQLite database)
# - tools/ (custom recipes)
# - roles/
# - skills/

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

BINARY_NAME="kestrel-server"
CONFIG_FILE="$ROOT_DIR/config.yaml"
DATA_DIR="$ROOT_DIR/data"
BACKUP_BASE_DIR="$ROOT_DIR/.upgrade-backup"

info() { echo -e "\033[0;34mℹ️  $*\033[0m"; }
success() { echo -e "\033[0;32m✅ $*\033[0m"; }
warn() { echo -e "\033[1;33m⚠️  $*\033[0m"; }
err() { echo -e "\033[0;31m❌ $*\033[0m"; }

backup_environment() {
  local ts
  ts="$(date +"%Y%m%d_%H%M%S")"
  local target_dir="${BACKUP_BASE_DIR}/${ts}"
  mkdir -p "$target_dir"

  info "Backing up configuration and database to $target_dir..."
  if [ -f "$CONFIG_FILE" ]; then
    cp -a "$CONFIG_FILE" "$target_dir/config.yaml"
  fi
  if [ -d "$DATA_DIR" ]; then
    tar -czf "$target_dir/data.tgz" -C "$ROOT_DIR" data || true
  fi
  if [ -d "$ROOT_DIR/tools" ]; then
    tar -czf "$target_dir/tools.tgz" -C "$ROOT_DIR" tools || true
  fi
  success "Backup created at $target_dir"
}

rebuild_and_start() {
  info "Rebuilding Kestrel server binary..."
  go build -o "$BINARY_NAME" ./cmd/server
  if [ -f "./run.sh" ]; then
    chmod +x ./run.sh
    exec ./run.sh "$@"
  else
    exec "./$BINARY_NAME" -config "$CONFIG_FILE"
  fi
}

main() {
  echo ""
  echo "=========================================="
  echo "      Kestrel Maintenance & Upgrade"
  echo "=========================================="
  echo ""

  backup_environment
  rebuild_and_start "$@"
}

main "$@"
