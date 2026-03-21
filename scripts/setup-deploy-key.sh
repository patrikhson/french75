#!/usr/bin/env bash
# scripts/setup-deploy-key.sh
# Generates an SSH key pair for GitHub Actions deployments.
# Run this from your Mac once.

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
echo "Copying public key to $VM01:/tmp ..."
scp "${KEY_PATH}.pub" "$VM01:/tmp/french75_deploy.pub"

echo "Installing public key for deploy user (will prompt for sudo password) ..."
ssh -t "$VM01" "sudo mkdir -p /home/deploy/.ssh && sudo cp /tmp/french75_deploy.pub /home/deploy/.ssh/authorized_keys && sudo chmod 700 /home/deploy/.ssh && sudo chmod 600 /home/deploy/.ssh/authorized_keys && sudo chown -R deploy:deploy /home/deploy/.ssh && rm /tmp/french75_deploy.pub && echo 'Key installed.'"

echo ""
echo "Testing deploy user SSH access ..."
ssh -i "$KEY_PATH" -o BatchMode=yes -o StrictHostKeyChecking=no "deploy@$VM01" "echo 'SSH as deploy: OK'"

echo ""
echo "===== ADD THIS TO GITHUB SECRETS ====="
echo "Name: DEPLOY_SSH_KEY"
echo "URL:  https://github.com/patrikhson/french75/settings/secrets/actions/new"
echo ""
cat "$KEY_PATH"
echo "===== DONE ====="
