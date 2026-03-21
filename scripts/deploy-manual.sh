#!/usr/bin/env bash
# scripts/deploy-manual.sh
# Manually deploys the binary to vm01. Used before GitHub Actions is wired up,
# and as a fallback if CI/CD is broken.
# Run this from your Mac after running scripts/build.sh.
#
# Usage: bash scripts/deploy-manual.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$SCRIPT_DIR/.."
BINARY="$ROOT/french75-linux"
VM01="vm01.paftech.se"

if [[ ! -f "$BINARY" ]]; then
echo "ERROR: $BINARY not found. Run scripts/build.sh first."
exit 1
fi

echo "Copying binary to $VM01 ..."
scp "$BINARY" "paf@$VM01:/tmp/french75_new"

echo "Installing and restarting on $VM01 ..."
ssh -t "$VM01" "sudo mv /tmp/french75_new /opt/french75/french75 && sudo chmod +x /opt/french75/french75 && sudo systemctl restart french75 && sleep 2 && systemctl is-active french75"

echo ""
echo "Checking /health endpoint ..."
sleep 1
curl -sf https://french75.paftech.se/health && echo " — OK" || echo " — FAILED (service may still be starting)"
