#!/usr/bin/env bash
# scripts/recon-vm04.sh
# Run this on vm04 to report what is already installed.
# Usage: bash scripts/recon-vm04.sh

set -euo pipefail

echo "===== OS ====="
lsb_release -a 2>/dev/null || cat /etc/os-release
echo "Arch: $(uname -m)"

echo ""
echo "===== DISK ====="
df -h /

echo ""
echo "===== POSTGRESQL ====="
if command -v psql &>/dev/null; then
    psql --version
    pg_lsclusters 2>/dev/null || echo "(pg_lsclusters not available)"
    echo "--- Existing databases ---"
    sudo -u postgres psql -c "\l" 2>/dev/null || echo "(could not list databases — check sudo access)"
    echo "--- Existing roles ---"
    sudo -u postgres psql -c "\du" 2>/dev/null || echo "(could not list roles)"
    echo "--- postgresql.conf listen_addresses ---"
    sudo grep -h "^listen_addresses" /etc/postgresql/*/main/postgresql.conf 2>/dev/null || echo "(not set / not found)"
    echo "--- pg_hba.conf (non-comment lines) ---"
    sudo grep -hv "^#\|^$" /etc/postgresql/*/main/pg_hba.conf 2>/dev/null || echo "(not found)"
else
    echo "PostgreSQL NOT installed"
fi

echo ""
echo "===== FIREWALL ====="
sudo ufw status 2>/dev/null || echo "(ufw not available)"

echo ""
echo "===== NETWORK ====="
hostname -I
ip route get 1 2>/dev/null | head -1

echo ""
echo "===== DONE ====="
echo "Paste this entire output back to Claude."
