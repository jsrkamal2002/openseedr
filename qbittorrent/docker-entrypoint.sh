#!/bin/sh
# Provisions WebUI credentials in qBittorrent.conf before first start.
# Reads QBT_WEBUI_USER and QBT_WEBUI_PASS environment variables.
# Safe to run on every restart — only writes credentials when the password is missing.
set -e

# Ensure /data is world-writable (sticky bit) so the api container (different UID)
# can create per-user subdirectories. Pre-existing volumes may not have this mode.
chmod 1777 /data 2>/dev/null || true

# Fix permissions on existing download content so the API user (different UID)
# can delete files and directories that qBittorrent created.
# BusyBox chmod -R exits early when it hits dirs it doesn't own, so we use
# find to target only qbt-owned entries and chmod them individually.
find /data -user qbt \( -type d -o -type f \) -exec chmod a+rwX {} + 2>/dev/null || true

CONFIG_FILE="/config/qBittorrent/config/qBittorrent.conf"

mkdir -p "$(dirname "$CONFIG_FILE")"

# Write credentials block only once (when Password_PBKDF2 is absent).
if ! grep -qs "Password_PBKDF2" "$CONFIG_FILE" 2>/dev/null; then
    QBT_USER="${QBT_WEBUI_USER:-admin}"
    PASS="${QBT_WEBUI_PASS:-changeme}"

    # Generate PBKDF2-SHA512 hash in qBittorrent's native format:
    # base64(16-byte-salt) + ":" + base64(64-byte-key)
    # Parameters: SHA-512, 100 000 iterations, 64-byte output (see password.cpp)
    PBKDF2=$(python3 -c "
import hashlib, base64, os, sys
password = sys.argv[1].encode()
salt = os.urandom(16)
key = hashlib.pbkdf2_hmac('sha512', password, salt, 100000, 64)
print(base64.b64encode(salt).decode() + ':' + base64.b64encode(key).decode(), end='')
" "$PASS")

    cat >> "$CONFIG_FILE" << CONF

[Preferences]
WebUI\Username=$QBT_USER
WebUI\Password_PBKDF2="@ByteArray($PBKDF2)"
WebUI\HostHeaderValidation=false
WebUI\CSRFProtection=false
CONF

    echo "[entrypoint] qBittorrent WebUI credentials provisioned for user: $QBT_USER"
fi

# Ensure host-header and CSRF protection are always disabled, even if the
# [Preferences] block was written by a previous version without these keys.
if ! grep -qs "HostHeaderValidation=false" "$CONFIG_FILE" 2>/dev/null; then
    printf '\nWebUI\\HostHeaderValidation=false\n' >> "$CONFIG_FILE"
    echo "[entrypoint] HostHeaderValidation disabled"
fi
if ! grep -qs "CSRFProtection=false" "$CONFIG_FILE" 2>/dev/null; then
    printf '\nWebUI\\CSRFProtection=false\n' >> "$CONFIG_FILE"
    echo "[entrypoint] CSRFProtection disabled"
fi

# Set permissive umask so qBittorrent creates files as 0666 and directories as
# 0777. This allows the API container (running as a different UID) to delete
# downloaded content without requiring root or CAP_FOWNER.
umask 0000

exec "$@"
