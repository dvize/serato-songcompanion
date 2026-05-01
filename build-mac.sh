#!/usr/bin/env bash
# Run this on your Mac to build Serato Companion.dmg
# Requirements: Node.js (https://nodejs.org)
set -e
cd "$(dirname "$0")/electron"
echo "→ Installing npm dependencies..."
npm install
echo "→ Building macOS DMG..."
npx electron-builder --mac
echo "✓ Done. Check ../dist/ for the DMG."
