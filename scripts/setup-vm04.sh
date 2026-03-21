#!/usr/bin/env bash
# scripts/setup-vm04.sh
# Sets up PostgreSQL on vm04 for the french75 app.
# Uses PostgreSQL 15 (already running on port 5432).
#
# Usage: bash scripts/setup-vm04.sh <vm01-ip> <db-password>
# Example: bash scripts/setup-vm04.sh 1.2.3.4 mysecretpassword
#
# Run this ON vm04 after cloning the repo:
#   git clone https://github.com/patrikhson/french75.git
#   bash french75/scripts/setup-vm04.sh <vm01-ip> <db-password>

set -euo pipefail

VM01_IP="${1:-}"
DB_PASS="${2:-}"

if [[ -z "$VM01_IP" || -z "$DB_PASS" ]]; then
    echo "Usage: bash $0 <vm01-ip> <db-password>"
    echo "Example: bash $0 1.2.3.4 mysecretpassword"
    exit 1
fi

PG_VERSION=15
PG_CONF="/etc/postgresql/${PG_VERSION}/main/postgresql.conf"
PG_HBA="/etc/postgresql/${PG_VERSION}/main/pg_hba.conf"

echo "===== Setting up PostgreSQL ${PG_VERSION} for french75 ====="
echo "vm01 IP: ${VM01_IP}"
echo ""

# ---------------------------------------------------------------
# 1. Create database user and database
# ---------------------------------------------------------------
echo "----- Creating database user and database -----"
sudo -u postgres psql <<EOF
DO \$\$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'french75') THEN
        CREATE USER french75 WITH PASSWORD '${DB_PASS}';
        RAISE NOTICE 'User french75 created.';
    ELSE
        ALTER USER french75 WITH PASSWORD '${DB_PASS}';
        RAISE NOTICE 'User french75 already exists — password updated.';
    END IF;
END
\$\$;
EOF

sudo -u postgres psql <<EOF
SELECT 'CREATE DATABASE french75 OWNER french75'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'french75')\gexec
EOF

echo "Done."

# ---------------------------------------------------------------
# 2. Configure listen_addresses so PG listens on the VM's IP
# ---------------------------------------------------------------
echo ""
echo "----- Configuring listen_addresses -----"
VM04_IP=$(hostname -I | awk '{print $1}')
echo "vm04 IP detected as: ${VM04_IP}"

# Remove any existing listen_addresses line and add ours
sudo sed -i "s/^#*listen_addresses\s*=.*/listen_addresses = 'localhost,${VM04_IP}'/" "$PG_CONF"

# Verify
echo "listen_addresses is now:"
sudo grep "^listen_addresses" "$PG_CONF"

# ---------------------------------------------------------------
# 3. Allow connection from vm01 in pg_hba.conf
# ---------------------------------------------------------------
echo ""
echo "----- Configuring pg_hba.conf -----"
HBA_RULE="host    french75        french75        ${VM01_IP}/32            scram-sha-256"

# Add rule only if not already present
if sudo grep -qF "${VM01_IP}/32" "$PG_HBA"; then
    echo "Rule for ${VM01_IP} already exists in pg_hba.conf — skipping."
else
    echo "$HBA_RULE" | sudo tee -a "$PG_HBA"
    echo "Rule added."
fi

echo ""
echo "pg_hba.conf (non-comment lines):"
sudo grep -v "^#\|^$" "$PG_HBA"

# ---------------------------------------------------------------
# 4. Restart PostgreSQL 15
# ---------------------------------------------------------------
echo ""
echo "----- Restarting PostgreSQL ${PG_VERSION} -----"
sudo systemctl restart "postgresql@${PG_VERSION}-main"
sleep 2
sudo systemctl is-active "postgresql@${PG_VERSION}-main"

# ---------------------------------------------------------------
# 5. Verify connection works locally
# ---------------------------------------------------------------
echo ""
echo "----- Verifying local connection -----"
PGPASSWORD="${DB_PASS}" psql -h localhost -U french75 -d french75 -c "SELECT version();" \
    && echo "Local connection: OK" \
    || echo "Local connection: FAILED — check password and pg_hba.conf"

# ---------------------------------------------------------------
# 6. Note about firewall
# ---------------------------------------------------------------
echo ""
echo "===== FIREWALL NOTE ====="
echo "ufw is not installed on this VM."
echo "If your hosting provider has a network-level firewall (control panel),"
echo "make sure TCP port 5432 is allowed from ${VM01_IP} only."
echo "If using iptables directly, run:"
echo "  sudo iptables -A INPUT -p tcp --dport 5432 -s ${VM01_IP} -j ACCEPT"
echo "  sudo iptables -A INPUT -p tcp --dport 5432 -j DROP"
echo ""
echo "===== vm04 setup complete ====="
echo "DATABASE_URL for .env on vm01:"
echo "  postgres://french75:${DB_PASS}@${VM04_IP}:5432/french75?sslmode=require"
