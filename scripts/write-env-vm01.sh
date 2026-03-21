#!/usr/bin/env bash
# scripts/write-env-vm01.sh
# Reads secrets from your local .env and writes the production .env to vm01.
# Run this from your Mac, not from the VM.
#
# Usage: bash scripts/write-env-vm01.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOCAL_ENV="$SCRIPT_DIR/../.env"

if [[ ! -f "$LOCAL_ENV" ]]; then
echo "ERROR: $LOCAL_ENV not found. Cannot read secrets."
exit 1
fi

# Read secrets from local .env
source "$LOCAL_ENV"

VM01="vm01.paftech.se"
REMOTE_ENV="/opt/french75/.env"

echo "Writing production .env to $VM01:$REMOTE_ENV ..."

ssh "$VM01" "sudo tee $REMOTE_ENV > /dev/null" <<EOF
APP_ENV=production
PORT=${PORT:-8090}
DATABASE_URL=${DATABASE_URL}
SESSION_SECRET=${SESSION_SECRET}
CSRF_KEY=${CSRF_KEY}
INVITE_ONLY=${INVITE_ONLY:-true}
GOOGLE_CLIENT_ID=${GOOGLE_CLIENT_ID:-}
GOOGLE_CLIENT_SECRET=${GOOGLE_CLIENT_SECRET:-}
WEBAUTHN_RPID=french75.paftech.se
WEBAUTHN_RPORIGIN=https://french75.paftech.se
WEBAUTHN_RPDISPLAYNAME=French 75 Tracker
STORAGE_PATH=/opt/french75/photos
STORAGE_URL_PREFIX=https://french75.paftech.se/photos
APP_BASE_URL=https://french75.paftech.se
SMTP_HOST=${SMTP_HOST:-}
SMTP_PORT=${SMTP_PORT:-587}
SMTP_USER=${SMTP_USER:-}
SMTP_PASS=${SMTP_PASS:-}
SMTP_FROM=${SMTP_FROM:-noreply@paftech.se}
EOF

ssh "$VM01" "sudo chmod 600 $REMOTE_ENV && sudo chown french75:french75 $REMOTE_ENV"

echo "Verifying (first 3 lines):"
ssh "$VM01" "sudo head -3 $REMOTE_ENV"

echo "Done."
