#!/usr/bin/env bash
# scripts/recon-vm01.sh
# Run this on vm01 to report what is already installed.
# Usage: bash scripts/recon-vm01.sh

set -euo pipefail

echo "===== OS ====="
lsb_release -a 2>/dev/null || cat /etc/os-release
echo "Arch: $(uname -m)"

echo ""
echo "===== DISK ====="
df -h /

echo ""
echo "===== APACHE ====="
if command -v apache2 &>/dev/null; then
    apache2 -v
    echo "--- Enabled modules ---"
    sudo apache2ctl -M 2>/dev/null | sort
    echo "--- Enabled sites ---"
    ls /etc/apache2/sites-enabled/
    echo "--- Listening ports ---"
    ss -tlnp | grep apache2 || echo "(none found)"
else
    echo "Apache NOT installed"
fi

echo ""
echo "===== GO ====="
go version 2>/dev/null || echo "Go NOT installed"

echo ""
echo "===== NODE / NPM ====="
node --version 2>/dev/null || echo "Node NOT installed"
npm --version 2>/dev/null || echo "npm NOT installed"

echo ""
echo "===== TLS / CERTBOT ====="
if command -v certbot &>/dev/null; then
    certbot --version
    sudo certbot certificates 2>/dev/null || echo "(no certificates found)"
else
    echo "Certbot NOT installed"
fi
ls /etc/letsencrypt/live/ 2>/dev/null || echo "(no /etc/letsencrypt/live)"

echo ""
echo "===== EXISTING SERVICES ====="
systemctl list-units --type=service --state=running --no-legend 2>/dev/null | grep -v "^$"

echo ""
echo "===== PORTS IN USE ====="
ss -tlnp | grep -E ":80|:443|:8080" || echo "(none of :80, :443, :8080 in use)"

echo ""
echo "===== /opt DIRECTORY ====="
ls -la /opt/ 2>/dev/null || echo "(/opt does not exist)"

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
