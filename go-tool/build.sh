#!/usr/bin/env bash
# Cross-compiles prepare-for-print for macOS (Intel + Apple Silicon) and
# Windows (64-bit), from any machine with the Go toolchain installed.
# No cgo, no platform-specific dependencies, so this works from Mac, Windows,
# or Linux alike.
#
# Usage: ./build.sh
# Output binaries land in ./dist/

set -euo pipefail
cd "$(dirname "$0")"

mkdir -p dist

echo "Building macOS (Apple Silicon)..."
GOOS=darwin GOARCH=arm64 go build -o dist/prepare-for-print-mac-arm64 .

echo "Building macOS (Intel)..."
GOOS=darwin GOARCH=amd64 go build -o dist/prepare-for-print-mac-intel .

echo "Building Windows (64-bit)..."
GOOS=windows GOARCH=amd64 go build -o dist/prepare-for-print-windows.exe .

echo
echo "Done. Binaries are in ./dist/"
ls -la dist/
