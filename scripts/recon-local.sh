#!/usr/bin/env bash
# scripts/recon-local.sh
# Run this on your Mac to check your local dev tools.
# Usage: bash scripts/recon-local.sh

echo "===== Go ====="
go version 2>/dev/null || echo "Go NOT installed — install from https://go.dev/dl/"

echo ""
echo "===== Git ====="
git --version 2>/dev/null || echo "Git NOT installed"

echo ""
echo "===== GitHub CLI (gh) ====="
gh --version 2>/dev/null || echo "gh NOT installed — install with: brew install gh"

echo ""
echo "===== Node / npm (for Tailwind CSS build) ====="
node --version 2>/dev/null || echo "Node NOT installed — install from https://nodejs.org or: brew install node"
npm --version 2>/dev/null || echo "npm NOT installed"

echo ""
echo "===== PostgreSQL client (psql, for local dev) ====="
psql --version 2>/dev/null || echo "psql NOT installed — install with: brew install postgresql"

echo ""
echo "===== DONE ====="
echo "Paste this entire output back to Claude."
