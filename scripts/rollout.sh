#!/bin/sh
# Bulk rollout: installs filedrop on many machines over SSH.
#
#   scripts/rollout.sh rollout.conf [host ...]
#
# rollout.conf (see examples/rollout.conf.example) names the machines, the
# packages, the fleet key and the agent.conf to push. For each machine the
# script copies the right package for its OS/CPU, the key and the agent.conf,
# installs, applies the key, restarts the agent and runs `filedrop-agent check`.
#
# Requires: ssh/scp access with a user that may use sudo (key-based or via
# sshpass - not handled here), and `xargs -P` for parallelism.

set -eu

CONF="${1:?usage: rollout.sh rollout.conf [host ...]}"
shift || true

# defaults, overridable in the config file
HOSTS_FILE=""
SSH_USER="root"
SSH_OPTS="-o StrictHostKeyChecking=accept-new -o ConnectTimeout=10 -o BatchMode=yes"
USE_SUDO="yes"
PACKAGE_DIR="dist"
FLEET_KEY=""
AGENT_CONF=""
PARALLEL=5
# shellcheck disable=SC1090
. "$CONF"

[ -n "$FLEET_KEY" ] && [ -r "$FLEET_KEY" ] || { echo "FLEET_KEY is not set or unreadable (create one with filedrop-keygen)" >&2; exit 1; }
[ -n "$AGENT_CONF" ] && [ -r "$AGENT_CONF" ] || { echo "AGENT_CONF is not set or unreadable" >&2; exit 1; }

if [ "$#" -gt 0 ]; then
    HOSTLIST="$*"
else
    [ -r "$HOSTS_FILE" ] || { echo "HOSTS_FILE is not set or unreadable" >&2; exit 1; }
    HOSTLIST="$(grep -v '^[[:space:]]*#' "$HOSTS_FILE" | awk 'NF{print $1}')"
fi

SUDO=""
[ "$USE_SUDO" = "yes" ] && [ "$SSH_USER" != "root" ] && SUDO="sudo"

deploy_one() {
    host="$1"
    tag="[$host]"
    # shellcheck disable=SC2086
    arch="$(ssh $SSH_OPTS "$SSH_USER@$host" 'uname -m' 2>/dev/null)" || { echo "$tag FAILED: cannot connect"; return 1; }
    case "$arch" in
        x86_64) deb=amd64; rpm=x86_64 ;;
        aarch64|arm64) deb=arm64; rpm=aarch64 ;;
        *) echo "$tag FAILED: unsupported CPU $arch"; return 1 ;;
    esac
    # shellcheck disable=SC2086
    if ssh $SSH_OPTS "$SSH_USER@$host" 'command -v dpkg >/dev/null 2>&1'; then
        pkg="$(ls "$PACKAGE_DIR"/filedrop-agent_*_"$deb".deb 2>/dev/null | tail -n1)"; kind=deb
    elif ssh $SSH_OPTS "$SSH_USER@$host" 'command -v rpm >/dev/null 2>&1'; then
        pkg="$(ls "$PACKAGE_DIR"/filedrop-agent-*."$rpm".rpm 2>/dev/null | tail -n1)"; kind=rpm
    else
        echo "$tag FAILED: neither dpkg nor rpm found (use the tar.gz installer by hand)"; return 1
    fi
    [ -n "$pkg" ] || { echo "$tag FAILED: no $kind package for $arch in $PACKAGE_DIR"; return 1; }

    rtmp="/tmp/filedrop-rollout"
    # shellcheck disable=SC2086
    ssh $SSH_OPTS "$SSH_USER@$host" "rm -rf $rtmp && mkdir -p $rtmp && chmod 700 $rtmp" || return 1
    # shellcheck disable=SC2086
    scp -q $SSH_OPTS "$pkg" "$FLEET_KEY" "$AGENT_CONF" "$SSH_USER@$host:$rtmp/" || { echo "$tag FAILED: copy"; return 1; }

    pkgname="$(basename "$pkg")"
    if [ "$kind" = deb ]; then
        install_cmd="$SUDO dpkg -i $rtmp/$pkgname || $SUDO apt-get install -f -y"
    else
        install_cmd="$SUDO rpm -Uvh --replacepkgs $rtmp/$pkgname"
    fi
    remote="set -e
$install_cmd
$SUDO install -m 0640 -o root -g filedrop $rtmp/$(basename "$AGENT_CONF") /etc/filedrop/agent.conf
$SUDO /usr/sbin/filedrop-setup --key $rtmp/$(basename "$FLEET_KEY")
$SUDO systemctl restart filedrop-agent
rm -rf $rtmp
sleep 3
$SUDO /usr/bin/filedrop-agent check || true"
    # shellcheck disable=SC2086
    if out="$(ssh $SSH_OPTS "$SSH_USER@$host" "$remote" 2>&1)"; then
        echo "$tag OK"
        printf '%s\n' "$out" | sed "s/^/$tag   /"
    else
        echo "$tag FAILED:"
        printf '%s\n' "$out" | tail -n 15 | sed "s/^/$tag   /"
        return 1
    fi
}

count=$(printf '%s\n' $HOSTLIST | wc -l)
failed=0
if [ -n "${ROLLOUT_CHILD:-}" ] || [ "$PARALLEL" -le 1 ] || [ "$count" -le 1 ]; then
    for h in $HOSTLIST; do deploy_one "$h" || failed=$((failed+1)); done
else
    # run this script again, one host per process, $PARALLEL at a time
    printf '%s\n' $HOSTLIST | ROLLOUT_CHILD=1 xargs -P "$PARALLEL" -I{} "$0" "$CONF" {} || failed=1
fi
if [ "$failed" -eq 0 ]; then [ -n "${ROLLOUT_CHILD:-}" ] || echo "All done."; else echo "Some machines FAILED - see above." >&2; exit 1; fi
