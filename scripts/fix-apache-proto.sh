#!/usr/bin/env bash
# One-time fix: correct X-Forwarded-Proto in the SSL vhost created by certbot.
set -euo pipefail
FILE=/etc/apache2/sites-available/french75-le-ssl.conf
sudo sed -i 's/X-Forwarded-Proto "http"/X-Forwarded-Proto "https"/' "$FILE"
echo "Fixed:"
sudo grep X-Forwarded "$FILE"
sudo systemctl reload apache2
echo "Apache reloaded."
