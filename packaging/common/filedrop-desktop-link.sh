#!/bin/sh
# filedrop-desktop-link.sh - keeps a "shared folder" shortcut on every user's
# desktop. On the panel and on the laptop/PC it has the same name although it
# points to different real folders (inbox on the panel, outbox on the client).
# The role, the target folder and the shortcut name come from the agent
# (`filedrop-agent info`), so there is no naming logic duplicated here.
#
# About "undeletable": a symlink cannot be protected with chattr +i (the kernel
# neither reads nor writes attributes of symlinks). Instead this script runs as
# root every ~2 minutes from systemd (filedrop-desktop.timer) and recreates the
# shortcut if it is gone. A user can delete it but cannot keep it away without
# administrator rights. The desktop directory itself must not be made
# immutable - users would not be able to keep their own files on the desktop.
#
# The real desktop directory is found properly (not just "~/Desktop"): on a
# localized system it is usually e.g. "Рабочий стол". Order: xdg-user-dir in the
# user's own session, then the system default /etc/xdg/user-dirs.defaults, then
# what already exists on disk, and "Desktop" only as the last resort.
#
# Usage:
#   filedrop-desktop-link.sh --all           /etc/skel and every /home/*
#   filedrop-desktop-link.sh --remove-all    remove the shortcut everywhere
#   filedrop-desktop-link.sh <home-dir>      a single home directory (debugging)

set -u

# Candidate names of the desktop directory. Kept as separate variables (not
# one space-separated string) because "Рабочий стол" itself contains a space.
CAND1="Рабочий стол"
CAND2="Desktop"
LOG_FILE=/var/log/filedrop-desktop-link.log

INFO="$(/usr/bin/filedrop-agent info 2>/dev/null)" || INFO=""
LINK_TARGET="$(printf '%s\n' "$INFO" | sed -n 's/^target=//p')"
LINK_NAME="$(printf '%s\n' "$INFO" | sed -n 's/^link_name=//p')"
[ -n "$LINK_TARGET" ] && [ -n "$LINK_NAME" ] || exit 0   # unknown hostname scheme: nothing to do

# log <text> - short entry for REAL problems only (not normal operation), so
# that the next failure can be diagnosed from /var/log/filedrop-desktop-link.log
log() {
    printf '%s %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$1" >> "$LOG_FILE" 2>/dev/null
    # keep the log from growing forever if a problem repeats every 2 minutes
    if [ -f "$LOG_FILE" ]; then
        lines="$(wc -l < "$LOG_FILE" 2>/dev/null || echo 0)"
        if [ "$lines" -gt 500 ] 2>/dev/null; then
            tail -n 200 "$LOG_FILE" > "$LOG_FILE.tmp" 2>/dev/null && mv "$LOG_FILE.tmp" "$LOG_FILE" 2>/dev/null
        fi
    fi
    return 0
}

# resolve_desktop_dir <home> - prints the path of this user's desktop directory
# (creating it is up to the caller).
#
# The order matters: "what already exists on disk" is deliberately NOT the
# first priority - if an older version once created the wrong directory, that
# mistake would otherwise confirm itself forever.
resolve_desktop_dir() {
    base="$1"
    user="$(basename "$base")"

    # su WITHOUT "-" (not a login shell): a login shell runs /etc/profile,
    # ~/.profile, ~/.bashrc ... and ANY stray output there (banner, MOTD,
    # nvm/rbenv) would end up in the variable and break the path. HOME is set
    # to the target user anyway. Only the last line that looks like an
    # absolute path is taken, as protection against any other noise.
    if [ "$base" != "/etc/skel" ] && command -v xdg-user-dir >/dev/null 2>&1 \
        && getent passwd "$user" >/dev/null 2>&1; then
        d="$(su -s /bin/sh "$user" -c 'xdg-user-dir DESKTOP' 2>/dev/null | grep '^/' | tail -n1)"
        if [ -n "$d" ] && [ "$d" != "$base" ]; then
            printf '%s\n' "$d"
            return 0
        fi
    fi

    if [ -r /etc/xdg/user-dirs.defaults ]; then
        name="$(sed -n 's/^DESKTOP=//p' /etc/xdg/user-dirs.defaults | head -n1)"
        if [ -n "$name" ]; then
            printf '%s\n' "$base/$name"
            return 0
        fi
    fi

    for cand in "$CAND1" "$CAND2"; do
        if [ -d "$base/$cand" ]; then
            printf '%s\n' "$base/$cand"
            return 0
        fi
    done

    printf '%s\n' "$base/Desktop"
    return 0
}

# purge_strays <base> <current-link-path> - removes orphaned shortcuts in ANY
# known candidate desktop directory except the current one. Done BEFORE the
# current link is created and independently of whether that succeeds, so a
# misplaced old shortcut cannot linger where nobody sees it.
purge_strays() {
    base="$1"
    keep="$2"
    for cand in "$CAND1" "$CAND2"; do
        stray="$base/$cand/$LINK_NAME"
        [ "$stray" = "$keep" ] && continue
        if [ -e "$stray" ] || [ -L "$stray" ]; then
            if rm -f "$stray" 2>/dev/null; then
                log "removed stale shortcut: $stray"
            else
                log "FAILED to remove stale shortcut: $stray"
            fi
        fi
    done
    return 0
}

link_one() {
    base="$1"
    [ -d "$base" ] || return 0

    desktop_dir="$(resolve_desktop_dir "$base")"
    link_path="$desktop_dir/$LINK_NAME"

    purge_strays "$base" "$link_path"

    if ! mkdir -p "$desktop_dir" 2>/dev/null; then
        log "cannot create $desktop_dir"
        return 0
    fi

    if ! ln -sfn "$LINK_TARGET" "$link_path" 2>/dev/null; then
        log "cannot create shortcut $link_path"
        return 0
    fi

    if [ "$base" != "/etc/skel" ]; then
        owner="$(stat -c '%U:%G' "$base" 2>/dev/null)"
        [ -n "$owner" ] && chown -h "$owner" "$desktop_dir" "$link_path" 2>/dev/null
    fi

    return 0
}

remove_one() {
    base="$1"
    [ -d "$base" ] || return 0
    for cand in "$CAND1" "$CAND2"; do
        link_path="$base/$cand/$LINK_NAME"
        if [ -L "$link_path" ]; then
            rm -f "$link_path" 2>/dev/null
        fi
    done
    return 0
}

case "${1:-}" in
    --all)
        link_one /etc/skel
        for base in /home/*; do
            link_one "$base"
        done
        ;;
    --remove-all)
        remove_one /etc/skel
        for base in /home/*; do
            remove_one "$base"
        done
        ;;
    "")
        echo "usage: filedrop-desktop-link.sh <home-dir> | --all | --remove-all" >&2
        exit 1
        ;;
    *)
        link_one "$1"
        ;;
esac

exit 0
