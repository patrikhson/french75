#!/usr/bin/env bash
# scripts/setup-deploy-key.sh
# Generates an SSH key pair for GitHub Actions deployments.
# The public key goes to vm01's deploy user.
# The private key goes to GitHub as a repository secret.
# Run this from your Mac once.
#
# Usage: bash scripts/setup-deploy-key.sh

set -euo pipefail

KEY_PATH="$HOME/.ssh/french75_deploy"
VM01="vm01.paftech.se"

if [[ -f "$KEY_PATH" ]]; then
echo "Key already exists at $KEY_PATH — delete it first if you want to regenerate."
exit 1
fi

echo "Generating SSH key pair ..."
ssh-keygen -t ed25519 -C "github-actions-french75-deploy" -f "$KEY_PATH" -N ""

echo ""
echo "Installing public key on $VM01 for the deploy user ..."
PUBLIC_KEY=$(cat "${KEY_PATH}.pub")
ssh -t "$VM01" "sudo mkdir -p /home/deploy/.ssh && echo '$PUBLIC_KEY' | sudo tee -a /home/deploy/.ssh/authorized_keys > /dev/null && sudo chmod 700 /home/deploy/.ssh && sudo chmod 600 /home/deploy/.ssh/authorized_keys && sudo chown -R deploy:deploy /home/deploy/.ssh"

echo ""
echo "Testing deploy user SSH access ..."
ssh -i "$KEY_PATH" -o StrictHostKeyChecking=no "deploy@$VM01" "echo 'SSH access as deploy: OK'"

echo ""
echo "===== GITHUB SECRET ====="
echo "Add the following as a GitHub Actions secret named DEPLOY_SSH_KEY:"
echo "Go to: https://github.com/patrikhson/french75/settings/secrets/actions/new"
echo ""
cat "$KEY_PATH"
echo ""
echo "===== DONE ====="
echo "Private key is at: $KEY_PATH"
echo "Keep it safe — if lost, re-run this script."
