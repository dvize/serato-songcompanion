#!/usr/bin/env bash
# Serato Companion — Mac launcher
# Requirements: Node.js (https://nodejs.org)
set -e
cd "$(dirname "$0")"

# Make Go binaries executable
chmod 755 companion-mac-arm64 companion-mac-amd64 2>/dev/null || true

# Install Electron deps if needed
if [ ! -d "electron/node_modules" ]; then
    echo "→ Installing dependencies (one-time)..."
    (cd electron && npm install)
fi

echo "→ Starting Serato Companion..."
cd electron && npm start
