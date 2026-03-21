#!/usr/bin/env bash
# scripts/setup-vm01.sh
# Sets up vm01 as the application server for french75.
# Apache is already running. This script adds the vhost and app infrastructure.
#
# Usage: bash scripts/setup-vm01.sh <domain> <app-port>
# Example: bash scripts/setup-vm01.sh french75.paftech.se 8090
#
# Run this ON vm01 after cloning the repo:
#   sudo git clone https://github.com/patrikhson/french75.git /usr/local/src/french75
#   bash /usr/local/src/french75/scripts/setup-vm01.sh french75.paftech.se 8090

set -euo pipefail

DOMAIN="${1:-}"
APP_PORT="${2:-8090}"

if [[ -z "$DOMAIN" ]]; then
    echo "Usage: bash $0 <domain> [app-port]"
    echo "Example: bash $0 french75.paftech.se 8090"
    exit 1
fi

echo "===== Setting up vm01 for french75 ====="
echo "Domain:   ${DOMAIN}"
echo "App port: ${APP_PORT}"
echo ""

# ---------------------------------------------------------------
# 1. Enable required Apache modules
# ---------------------------------------------------------------
echo "----- Enabling Apache modules -----"
for mod in proxy proxy_http ssl headers rewrite remoteip; do
    if sudo a2enmod "$mod" 2>&1 | grep -q "already enabled"; then
        echo "  $mod: already enabled"
    else
        echo "  $mod: enabled"
    fi
done

# ---------------------------------------------------------------
# 2. Create app directory and system user
# ---------------------------------------------------------------
echo ""
echo "----- Creating app directory and system user -----"
sudo mkdir -p /opt/french75/photos

if id french75 &>/dev/null; then
    echo "User french75 already exists."
else
    sudo useradd -r -s /bin/false french75
    echo "User french75 created."
fi

sudo chown -R french75:french75 /opt/french75
echo "/opt/french75 owned by french75."

# ---------------------------------------------------------------
# 3. Create deploy user for GitHub Actions CI/CD
# ---------------------------------------------------------------
echo ""
echo "----- Setting up deploy user -----"
if id deploy &>/dev/null; then
    echo "User deploy already exists."
else
    sudo useradd -m -s /bin/bash deploy
    echo "User deploy created."
fi

sudo mkdir -p /home/deploy/.ssh
sudo chmod 700 /home/deploy/.ssh
sudo chown -R deploy:deploy /home/deploy/.ssh

# Sudoers entry for deploy user (narrow permissions)
SUDOERS_LINE="deploy ALL=(ALL) NOPASSWD: /bin/mv /tmp/french75_new /opt/french75/french75, /bin/chmod +x /opt/french75/french75, /bin/systemctl restart french75"
SUDOERS_FILE="/etc/sudoers.d/french75-deploy"
if [[ -f "$SUDOERS_FILE" ]]; then
    echo "Sudoers entry already exists."
else
    echo "$SUDOERS_LINE" | sudo tee "$SUDOERS_FILE" > /dev/null
    sudo chmod 440 "$SUDOERS_FILE"
    echo "Sudoers entry created: $SUDOERS_FILE"
fi

echo ""
echo "ACTION REQUIRED: Add the GitHub Actions SSH public key to deploy's authorized_keys."
echo "After generating the key pair (see README), run:"
echo "  echo 'ssh-ed25519 AAAA...' | sudo tee -a /home/deploy/.ssh/authorized_keys"
echo "  sudo chmod 600 /home/deploy/.ssh/authorized_keys"
echo "  sudo chown deploy:deploy /home/deploy/.ssh/authorized_keys"

# ---------------------------------------------------------------
# 4. Create systemd service file
# ---------------------------------------------------------------
echo ""
echo "----- Creating systemd service -----"
sudo tee /etc/systemd/system/french75.service > /dev/null <<EOF
[Unit]
Description=French 75 Tracker
After=network.target

[Service]
Type=simple
User=french75
Group=french75
WorkingDirectory=/opt/french75
EnvironmentFile=/opt/french75/.env
ExecStart=/opt/french75/french75
Restart=on-failure
RestartSec=5s
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ReadWritePaths=/opt/french75/photos
StandardOutput=journal
StandardError=journal
SyslogIdentifier=french75

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable french75
echo "Systemd service created and enabled."

# ---------------------------------------------------------------
# 5. Create Apache virtual host
# ---------------------------------------------------------------
echo ""
echo "----- Creating Apache virtual host -----"
VHOST_FILE="/etc/apache2/sites-available/french75.conf"

sudo tee "$VHOST_FILE" > /dev/null <<EOF
<VirtualHost *:80>
    ServerName ${DOMAIN}
    # Will be upgraded to HTTPS redirect by certbot
    ProxyPreserveHost On
    ProxyPass        / http://127.0.0.1:${APP_PORT}/
    ProxyPassReverse / http://127.0.0.1:${APP_PORT}/
    RequestHeader set X-Forwarded-Proto "http"
    ErrorLog  /var/log/apache2/french75_error.log
    CustomLog /var/log/apache2/french75_access.log combined
</VirtualHost>
EOF

sudo a2ensite french75
sudo systemctl reload apache2
echo "Virtual host created and enabled: ${VHOST_FILE}"

# ---------------------------------------------------------------
# 6. Create .env template on vm01
# ---------------------------------------------------------------
echo ""
echo "----- Creating .env template -----"
ENV_FILE="/opt/french75/.env"
if [[ -f "$ENV_FILE" ]]; then
    echo ".env already exists — not overwriting."
else
    sudo tee "$ENV_FILE" > /dev/null <<EOF
APP_ENV=production
PORT=${APP_PORT}

DATABASE_URL=postgres://french75:CHANGE_THIS_PASSWORD@vm04.paftech.se:5432/french75?sslmode=require

SESSION_SECRET=GENERATE_WITH_openssl_rand_hex_32
CSRF_KEY=GENERATE_WITH_openssl_rand_hex_32

INVITE_ONLY=true

GOOGLE_CLIENT_ID=
GOOGLE_CLIENT_SECRET=

WEBAUTHN_RPID=${DOMAIN}
WEBAUTHN_RPORIGIN=https://${DOMAIN}
WEBAUTHN_RPDISPLAYNAME=French 75 Tracker

STORAGE_PATH=/opt/french75/photos
STORAGE_URL_PREFIX=https://${DOMAIN}/photos
APP_BASE_URL=https://${DOMAIN}

SMTP_HOST=
SMTP_PORT=587
SMTP_USER=
SMTP_PASS=
SMTP_FROM=noreply@paftech.se
EOF
    sudo chmod 600 "$ENV_FILE"
    sudo chown french75:french75 "$ENV_FILE"
    echo ".env template created at ${ENV_FILE}"
    echo "IMPORTANT: Edit it and fill in DATABASE_URL, SESSION_SECRET, CSRF_KEY, Google OAuth credentials."
fi

# ---------------------------------------------------------------
# 7. Summary
# ---------------------------------------------------------------
echo ""
echo "===== vm01 setup complete ====="
echo ""
echo "Next steps:"
echo ""
echo "1. Make sure DNS for ${DOMAIN} points to this server's IP:"
echo "   $(hostname -I | awk '{print $1}')"
echo ""
echo "2. Edit /opt/french75/.env and fill in all CHANGE_THIS / GENERATE_WITH values:"
echo "   sudo nano /opt/french75/.env"
echo "   Generate secrets with: openssl rand -hex 32"
echo ""
echo "3. Once DNS is live, get a TLS certificate:"
echo "   sudo certbot --apache -d ${DOMAIN}"
echo ""
echo "4. Add the GitHub Actions deploy SSH key to /home/deploy/.ssh/authorized_keys"
echo "   (see README for how to generate the key pair)"
echo ""
echo "5. Deploy the first binary manually (before CI/CD is wired up):"
echo "   scp french75-linux deploy@${DOMAIN}:/tmp/french75_new"
echo "   ssh deploy@${DOMAIN} 'sudo mv /tmp/french75_new /opt/french75/french75 && sudo chmod +x /opt/french75/french75 && sudo systemctl start french75'"
