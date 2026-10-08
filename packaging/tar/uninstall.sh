#!/bin/sh
# Removes what install.sh installed.
#   sudo ./uninstall.sh [--purge]
# --purge additionally deletes /etc/filedrop and /opt/filedrop (INCLUDING any
# files waiting there) and the user "filedrop".

set -u
[ "$(id -u)" = "0" ] || { echo "run as root: sudo ./uninstall.sh" >&2; exit 1; }
HERE="$(cd "$(dirname "$0")" && pwd)"
ROOTFS="$HERE/rootfs"

[ -x /usr/sbin/filedrop-setup ] && /usr/sbin/filedrop-setup --teardown

( cd "$ROOTFS" && find . -type f | sed 's|^\./||' ) | while IFS= read -r rel; do
    case "$rel" in
        etc/*) continue ;;   # configuration stays unless --purge
    esac
    rm -f "/$rel"
done
rmdir /usr/lib/filedrop /usr/share/doc/filedrop-agent 2>/dev/null
command -v systemctl >/dev/null 2>&1 && systemctl daemon-reload >/dev/null 2>&1
echo "Removed. Configuration (/etc/filedrop, /etc/ssh/sshd_config.d/filedrop.conf), /opt/filedrop and the user 'filedrop' were left in place."

if [ "${1:-}" = "--purge" ]; then
    rm -rf /etc/filedrop /opt/filedrop /etc/ssh/sshd_config.d/filedrop.conf
    userdel filedrop >/dev/null 2>&1
    echo "Purged."
fi
