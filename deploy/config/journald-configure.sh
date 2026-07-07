#!/usr/bin/env bash
# Configure systemd journald size limits to prevent unbounded disk growth.
# Creates /etc/systemd/journald.conf.d/togather.conf.
#
# Usage: sudo ./deploy/config/journald-configure.sh
set -euo pipefail

JOURNALD_CONF="/etc/systemd/journald.conf.d"
JOURNALD_FILE="${JOURNALD_CONF}/togather.conf"

if [[ -f "$JOURNALD_FILE" ]]; then
    echo "Journald config already exists at $JOURNALD_FILE — skipping."
    exit 0
fi

mkdir -p "$JOURNALD_CONF"

cat > "$JOURNALD_FILE" << 'EOF'
[Journal]
SystemMaxUse=500M
MaxFileSec=14day
EOF

systemctl restart systemd-journald

echo "Journald limits configured: SystemMaxUse=500M, MaxFileSec=14day"
