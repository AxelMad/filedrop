#!/bin/sh
# Refuses to pass if the tree contains something that looks like a secret.
# Run it before every push (CI runs it too):   scripts/check-secrets.sh
# Only tracked-or-to-be-added files are checked when inside a git repository,
# otherwise every file under the current directory.

set -u
cd "$(dirname "$0")/.." || exit 1

if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    files="$(git ls-files --cached --others --exclude-standard)"
else
    files="$(find . -type f -not -path './dist/*' -not -path './.git/*' | sed 's|^\./||')"
fi

bad=0
report() { echo "SECRET? $1"; bad=1; }

# 1. private keys
for f in $files; do
    [ -f "$f" ] || continue
    case "$f" in scripts/check-secrets.sh) continue ;; esac
    if grep -Iq -e '-----BEGIN [A-Z ]*PRIVATE KEY-----' -e 'OPENSSH PRIVATE KEY' "$f" 2>/dev/null; then
        report "$f contains a private key"
    fi
done

# 2. files with key-like names
for f in $files; do
    case "$(basename "$f")" in
        id_ed25519|id_rsa|id_ecdsa|fleet_key|*.pem|*.key|rollout.conf|.env)
            report "$f has a name used for secrets" ;;
    esac
done

# 3. tokens assigned a real-looking value
for f in $files; do
    [ -f "$f" ] || continue
    case "$f" in scripts/check-secrets.sh) continue ;; esac
    if grep -IEq '(FILEDROP_TOKEN|DIRECTORY_TOKEN)=[A-Za-z0-9]{20,}' "$f" 2>/dev/null; then
        report "$f assigns a long token value"
    fi
    if grep -IEq 'gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{30,}' "$f" 2>/dev/null; then
        report "$f contains a GitHub token"
    fi
done

if [ "$bad" -eq 0 ]; then
    echo "check-secrets: nothing suspicious found."
else
    echo "check-secrets: FIX THE ABOVE BEFORE PUSHING (and rotate anything that already leaked)." >&2
    exit 1
fi
