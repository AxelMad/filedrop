#!/bin/sh
# Installs filedrop-directory (hostname -> IP lookup service), needed only for
# RESOLVE_MODE=directory. Run it as root on the machine that will host the
# service (any always-on Linux box that all agents can reach over TCP 8781).
#
#   sudo ./install.sh [TOKEN]
#
# Without TOKEN a random one is generated (or, on a re-install, the existing
# one is kept). Put the same token into DIRECTORY_TOKEN of every agent.

set -e

if [ "$(id -u)" != "0" ]; then
    echo "run as root: sudo ./install.sh" >&2
    exit 1
fi

HERE="$(cd "$(dirname "$0")" && pwd)"
ENV_FILE=/etc/filedrop-directory/env
TOKEN="${1:-}"

if [ -z "$TOKEN" ] && [ -r "$ENV_FILE" ]; then
    TOKEN="$(sed -n 's/^FILEDROP_TOKEN=//p' "$ENV_FILE" | head -n1)"
    [ -n "$TOKEN" ] && echo "Keeping the existing token from $ENV_FILE."
fi
if [ -z "$TOKEN" ]; then
    TOKEN="$(head -c 24 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c 32)"
    echo "No token given - a random one was generated."
fi

getent group filedrop-directory >/dev/null || groupadd --system filedrop-directory
nologin=/usr/sbin/nologin; [ -x "$nologin" ] || nologin=/sbin/nologin
getent passwd filedrop-directory >/dev/null || useradd --system --gid filedrop-directory \
    --home-dir /var/lib/filedrop-directory --no-create-home --shell "$nologin" \
    --comment "filedrop-directory service account" filedrop-directory

install -D -m 0755 "$HERE/filedrop-directory" /usr/local/bin/filedrop-directory
install -D -m 0644 "$HERE/filedrop-directory.service" /etc/systemd/system/filedrop-directory.service

mkdir -p /var/lib/filedrop-directory
chown filedrop-directory:filedrop-directory /var/lib/filedrop-directory
chmod 0750 /var/lib/filedrop-directory

mkdir -p /etc/filedrop-directory
umask 077
printf 'FILEDROP_TOKEN=%s\n' "$TOKEN" > "$ENV_FILE"
chown root:filedrop-directory "$ENV_FILE"
chmod 0640 "$ENV_FILE"

systemctl daemon-reload
systemctl enable --now filedrop-directory
systemctl restart filedrop-directory

cat <<MSG

==================================================================
filedrop-directory is installed and running on TCP port 8781.

TOKEN (put it into DIRECTORY_TOKEN of every agent):
  $TOKEN

Check:  curl -H "X-Filedrop-Token: $TOKEN" http://127.0.0.1:8781/status

Make sure TCP 8781 is allowed by this machine's firewall from every subnet
that has panels or computers, e.g.
  firewalld:  firewall-cmd --permanent --add-port=8781/tcp && firewall-cmd --reload
  ufw:        ufw allow 8781/tcp
==================================================================
MSG
