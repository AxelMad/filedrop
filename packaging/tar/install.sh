#!/bin/sh
# Installer for systems without RPM/DEB (or when you simply prefer a tarball).
#
#   sudo ./install.sh [--key /path/to/fleet_key]
#
# Copies the files under / (existing configuration files are NOT overwritten),
# then runs filedrop-setup. Uninstall with ./uninstall.sh.

set -eu
[ "$(id -u)" = "0" ] || { echo "run as root: sudo ./install.sh" >&2; exit 1; }

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOTFS="$HERE/rootfs"
KEYFILE=""
if [ "${1:-}" = "--key" ]; then
    KEYFILE="${2:?--key needs a file}"
fi

[ -d "$ROOTFS" ] || { echo "rootfs/ not found next to install.sh" >&2; exit 1; }

( cd "$ROOTFS" && find . -type f | sed 's|^\./||' ) | while IFS= read -r rel; do
    dest="/$rel"
    case "$rel" in
        etc/*)
            if [ -e "$dest" ]; then
                echo "keeping existing $dest"
                continue
            fi ;;
    esac
    mode="$(stat -c %a "$ROOTFS/$rel")"
    install -D -m "$mode" "$ROOTFS/$rel" "$dest"
done

/usr/sbin/filedrop-setup --postinst
if [ -n "$KEYFILE" ]; then
    /usr/sbin/filedrop-setup --key "$KEYFILE"
fi
