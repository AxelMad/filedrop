#!/bin/sh
# filedrop-cleanup.sh
#
# Cleans the shared folder (outbox on a laptop/PC, inbox on a panel) of files
# and folders older than RETENTION_DAYS, warning the user beforehand with a
# pop-up: 4 days before, 1 day before and on the day of deletion. Runs as root
# from filedrop-cleanup.timer (at boot and once a day), because deleting must
# work regardless of who is logged in; the pop-up is shown in the session of
# whoever is at the computer (a root service cannot show a GUI otherwise).
#
# The age of a folder is the age of the NEWEST file inside it.
# Settings: /etc/filedrop/cleanup.conf (RETENTION_DAYS, NOTICE_LANG).
#
# State ("who was shown what") lives in /opt/filedrop/.cleanup-state, one
# "name<TAB>stage" line per entry, so the same warning does not pop up on every
# run but only when an entry moves to a new stage.

set -u

CONF=/etc/filedrop/cleanup.conf
RETENTION_DAYS=14
NOTICE_LANG=auto
# shellcheck disable=SC1090
[ -r "$CONF" ] && . "$CONF"

TARGET="$(/usr/bin/filedrop-agent info 2>/dev/null | sed -n 's/^target=//p')"
[ -n "$TARGET" ] && [ -d "$TARGET" ] || exit 0

# ---- language of the pop-up ----
if [ "$NOTICE_LANG" = "auto" ]; then
    l="${LC_ALL:-${LANG:-}}"
    if [ -z "$l" ]; then
        for f in /etc/locale.conf /etc/default/locale; do
            [ -r "$f" ] && l="$(sed -n 's/^LANG=//p' "$f" | head -n1 | tr -d '"')" && [ -n "$l" ] && break
        done
    fi
    case "$l" in ru*) NOTICE_LANG=ru ;; *) NOTICE_LANG=en ;; esac
fi

if [ "$NOTICE_LANG" = "ru" ]; then
    T_DELETED="filedrop-cleanup: удалено (старше $RETENTION_DAYS дн.):"
    T_HEAD1="Файлы в «Общей папке» хранятся $RETENTION_DAYS дн., дальше удаляются автоматически."
    T_HEAD2="Если что-то из списка ниже ещё нужно — перенесите это в другое место."
    T_TODAY="Будет удалено СЕГОДНЯ (последний день):"
    T_FINAL="Будет удалено ЗАВТРА:"
    T_EARLY="Будет удалено через 4 дня:"
else
    T_DELETED="filedrop-cleanup: deleted (older than $RETENTION_DAYS days):"
    T_HEAD1="Items in the shared folder are kept for $RETENTION_DAYS days and then deleted automatically."
    T_HEAD2="If you still need something from the list below, move it somewhere else."
    T_TODAY="Will be deleted TODAY (last day):"
    T_FINAL="Will be deleted TOMORROW:"
    T_EARLY="Will be deleted in 4 days:"
fi

STATE_FILE=/opt/filedrop/.cleanup-state
touch "$STATE_FILE" 2>/dev/null

NOW=$(date +%s)
DAY=86400

# modification time of an entry: a file's own mtime; for a folder the newest of
# the folder itself and everything inside it
entry_mtime() {
    f="$1"
    m="$(stat -c %Y "$f" 2>/dev/null)" || return 1
    if [ -d "$f" ]; then
        newest="$(find "$f" -type f -printf '%T@\n' 2>/dev/null | sort -n | tail -n1)"
        newest="${newest%.*}"
        [ -n "$newest" ] && [ "$newest" -gt "$m" ] 2>/dev/null && m="$newest"
    fi
    echo "$m"
}

# remaining = RETENTION_DAYS - age_in_days
#   < 0  -> delete now      0 -> last day      1 -> 1 day left      4 -> 4 days left
stage_for_remaining() {
    remaining=$1
    if [ "$remaining" -lt 0 ]; then
        echo "delete"
    elif [ "$remaining" -eq 0 ]; then
        echo "today"
    elif [ "$remaining" -eq 1 ]; then
        echo "final"
    elif [ "$remaining" -eq 4 ]; then
        echo "early"
    else
        echo ""
    fi
}

prev_stage_for() {
    awk -F'\t' -v f="$1" '$1==f{print $2}' "$STATE_FILE" 2>/dev/null | tail -n1
}

TMP_STATE="$(mktemp)"
NEW_EARLY=""
NEW_FINAL=""
NEW_TODAY=""
DELETED_LIST=""
NL='
'

for f in "$TARGET"/*; do
    [ -e "$f" ] || continue
    [ -L "$f" ] && continue            # never follow or delete links
    base="$(basename "$f")"
    mtime="$(entry_mtime "$f")" || continue
    age_days=$(( (NOW - mtime) / DAY ))
    remaining=$(( RETENTION_DAYS - age_days ))
    st="$(stage_for_remaining "$remaining")"

    if [ "$st" = "delete" ]; then
        if rm -rf -- "$f" 2>/dev/null; then
            DELETED_LIST="$DELETED_LIST $base;"
        fi
        continue
    fi

    if [ -n "$st" ]; then
        prev="$(prev_stage_for "$base")"
        if [ "$st" != "$prev" ]; then
            case "$st" in
                early) NEW_EARLY="$NEW_EARLY- $base$NL" ;;
                final) NEW_FINAL="$NEW_FINAL- $base$NL" ;;
                today) NEW_TODAY="$NEW_TODAY- $base$NL" ;;
            esac
        fi
        printf '%s\t%s\n' "$base" "$st" >> "$TMP_STATE"
    fi
done
cat "$TMP_STATE" > "$STATE_FILE"
rm -f "$TMP_STATE"

if [ -n "$DELETED_LIST" ]; then
    echo "$T_DELETED$DELETED_LIST"
fi

[ -n "$NEW_EARLY$NEW_FINAL$NEW_TODAY" ] || exit 0

NOTICE=/tmp/filedrop-cleanup-notice.txt
{
    echo "$T_HEAD1"
    echo "$T_HEAD2"
    echo
    if [ -n "$NEW_TODAY" ]; then
        echo "$T_TODAY"
        printf '%s' "$NEW_TODAY"
        echo
    fi
    if [ -n "$NEW_FINAL" ]; then
        echo "$T_FINAL"
        printf '%s' "$NEW_FINAL"
        echo
    fi
    if [ -n "$NEW_EARLY" ]; then
        echo "$T_EARLY"
        printf '%s' "$NEW_EARLY"
    fi
} > "$NOTICE"
chmod 0644 "$NOTICE" 2>/dev/null

command -v zenity >/dev/null 2>&1 || exit 0

for d in /tmp/.X11-unix/X*; do
    [ -e "$d" ] || continue
    n="${d##*X}"
    disp=":$n"
    owner="$(stat -c '%U' "$d" 2>/dev/null)"
    [ -n "$owner" ] || continue
    [ "$owner" = "root" ] && continue
    home="$(getent passwd "$owner" | cut -d: -f6)"
    [ -n "$home" ] || continue
    su -s /bin/sh - "$owner" -c \
        "DISPLAY='$disp' XAUTHORITY='$home/.Xauthority' zenity --text-info --title='filedrop' --filename='$NOTICE' --width=520 --height=340 --ok-label=OK" \
        >/dev/null 2>&1
    break
done
exit 0
