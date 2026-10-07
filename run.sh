#!/bin/bash

set -euo pipefail

# Kestrel deployment and run script
ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT_DIR"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info() { echo -e "${BLUE}ℹ️  $1${NC}"; }
success() { echo -e "${GREEN}✅ $1${NC}"; }
warning() { echo -e "${YELLOW}⚠️  $1${NC}"; }
error() { echo -e "${RED}❌ $1${NC}"; }

CONFIG_FILE="$ROOT_DIR/config.yaml"
EXAMPLE_CONFIG_FILE="$ROOT_DIR/config.example.yaml"
VENV_DIR="$ROOT_DIR/venv"
REQUIREMENTS_FILE="$ROOT_DIR/requirements.txt"
BINARY_NAME="kestrel-server"

check_go() {
    if ! command -v go >/dev/null 2>&1; then
        error "Go 1.21+ required but not found in PATH."
        exit 1
    fi
}

check_python() {
    if command -v python3 >/dev/null 2>&1; then
        if [ ! -d "$VENV_DIR" ]; then
            info "Creating Python virtual environment at $VENV_DIR..."
            python3 -m venv "$VENV_DIR" || true
        fi
        if [ -f "$REQUIREMENTS_FILE" ] && [ -d "$VENV_DIR" ]; then
            info "Installing Python security helper dependencies..."
            "$VENV_DIR/bin/pip" install -r "$REQUIREMENTS_FILE" >/dev/null 2>&1 || true
        fi
    fi
}

build_server() {
    info "Building Kestrel server binary..."
    go build -o "$BINARY_NAME" ./cmd/server
    success "Binary ready: $BINARY_NAME"
}

main() {
    echo ""
    echo "=========================================="
    echo "       Kestrel Platform Launcher"
    echo "=========================================="
    echo ""

    check_go
    check_python

    if [ ! -f "$CONFIG_FILE" ] && [ -f "$EXAMPLE_CONFIG_FILE" ]; then
        info "Initializing config.yaml from template..."
        cp "$EXAMPLE_CONFIG_FILE" "$CONFIG_FILE"
    fi

    build_server

    info "Starting Kestrel server..."
    exec "./$BINARY_NAME" "$@"
}

main "$@"
