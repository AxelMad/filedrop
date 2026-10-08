# Configuration reference

*Русская версия: [configuration.ru.md](configuration.ru.md)*

The agent reads `/etc/filedrop/agent.conf` (override with `-config FILE`): plain `KEY=VALUE` lines, `#` starts a comment, values may be wrapped in quotes. Unknown keys are an error (so typos do not go unnoticed). After a change: `sudo systemctl restart filedrop-agent`, then `filedrop-agent check`.

## 1. The naming scheme (who is the panel, who is the laptop)

filedrop pairs machines **only by hostname**:

- `HOSTNAME_PATTERN` is a regular expression ([Go/RE2 syntax](https://github.com/google/re2/wiki/Syntax)) that must match the *whole* hostname and contain a named group `(?P<role>…)`.
- `PANEL_ROLES` lists the values of that group meaning "panel" (receiver); `CLIENT_ROLES` the values meaning "client" (sender). Separate with commas or spaces. A value must not be in both lists.
- The **counterpart** of a machine is its own hostname with *only the role part replaced* by a role of the opposite kind. Everything else in the name (site, room, number …) must be identical for a pair. A panel with several client roles tries each in turn (useful when a room has either a laptop or an all-in-one).

| Pattern | Roles | `pc-room12` talks to |
|---|---|---|
| `^(?P<role>panel\|pc)-(?P<site>.+)$` (default) | panel=`panel`, client=`pc` | `panel-room12` |
| `^(?P<site>[a-z]+)-(?P<role>board\|desk)-(?P<n>\d+)$` | panel=`board`, client=`desk` | `north-desk-7` ⇄ `north-board-7` |
| `^(?P<role>[pnm])[0-9]{4}-(?P<s>\d+)-(?P<r>\d+)-(?P<n>\d+)$` | panel=`p`, client=`n m` | panel `p1234-0-421-0` ⇄ `n1234-0-421-0` or `m1234-0-421-0` |

Names that follow no scheme at all: set `ROLE=client` (or `panel`) and name the counterpart in `PEER_HOSTNAMES` (hostnames or IP addresses). `PEER_HOSTNAMES` overrides the derived counterpart in any case.

| Key | Default | Meaning |
|---|---|---|
| `ROLE` | `auto` | `auto` = from the hostname; `panel` / `client` force the side |
| `HOSTNAME_PATTERN` | `^(?P<role>panel\|pc)-(?P<site>.+)$` | see above |
| `PANEL_ROLES` | `panel` | |
| `CLIENT_ROLES` | `pc` | |
| `PEER_HOSTNAMES` | *(empty)* | explicit counterpart(s) |

## 2. Finding the counterpart's IP address (`RESOLVE_MODE`)

| Mode | Use it when | Needs |
|---|---|---|
| `dns` (default) | machine names resolve: DNS, mDNS/avahi, `/etc/hosts` | nothing |
| `directory` | DHCP network, names **not** in DNS | the `filedrop-directory` service ([install](#the-directory-service)) |
| `static` | fixed addresses (DHCP reservations) | a table file |

**directory** — every agent registers itself every `REGISTER_INTERVAL_SEC` seconds with **all its local addresses**; the server also records the IP the request came from. A counterpart gets all of them back and picks the one on its own subnet — see [networks.md](networks.md).

- `DIRECTORY_URL` — one URL, or **all** addresses of the server separated by commas/spaces (e.g. one per subnet). The same line works on every machine: the agent tries them in turn and sticks to the one that works from its own subnet (a one-time journal note appears when the first one was not it).
- `DIRECTORY_TOKEN` — shared secret (header `X-Filedrop-Token`), printed by the directory installer.

**static** — `STATIC_HOSTS_FILE` (default `/etc/filedrop/hosts.tsv`): lines `hostname<whitespace>ip [ip …]` (several addresses allowed); re-read on every lookup, so edits apply without a restart.

### The directory service

Only for `RESOLVE_MODE=directory`. Download `filedrop-directory-<version>-linux-<arch>.tar.gz` from the release, unpack on an always-on Linux machine, run `sudo ./install.sh` (it prints the token), and open TCP 8781 in that machine's firewall. Status page: `curl -H "X-Filedrop-Token: …" http://HOST:8781/status`. It never touches the files — it is only a phone book.

## 3. Folders and shortcut

| Key | Default | Meaning |
|---|---|---|
| `INBOX_DIR` | `/opt/filedrop/inbox` | panel: where received items land |
| `OUTBOX_DIR` | `/opt/filedrop/outbox` | client: the shared folder users drop items into |
| `SENT_DIR` | `/opt/filedrop/outbox/.sent` | client: delivered items are moved here |
| `REMOTE_INBOX_PATH` | `/inbox` | the inbox path **as seen inside the sshd chroot** (`ChrootDirectory /opt/filedrop`). Not the same value as `INBOX_DIR`, although it is the same folder |
| `SHARED_FOLDER_NAME` | `Shared folder` | name of the desktop shortcut (any language, e.g. `Общая папка`) |

If you change `INBOX_DIR`/`OUTBOX_DIR` you must also change `ChrootDirectory` in `/etc/ssh/sshd_config.d/filedrop.conf` and `REMOTE_INBOX_PATH` consistently, and the paths in the setup script's permissions — the defaults are recommended.

## 4. Transport

| Key | Default | Meaning |
|---|---|---|
| `SSH_USER` | `filedrop` | account on the receiving side |
| `SSH_KEY` | `/etc/filedrop/id_ed25519` | private key |
| `SSH_PORT` | `22` | |
| `KNOWN_HOSTS_FILE` | `/var/lib/filedrop/known_hosts` | host keys are accepted on first contact (`accept-new`) and pinned afterwards |
| `IGNORE_INTERFACES` | `lo docker* br-* veth* virbr* lxcbr* lxdbr* cni* flannel* cali* podman* vmnet* vboxnet*` | glob patterns of interfaces whose addresses are neither reported nor used to judge "same subnet" |
| `SCP_SFTP_FLAG` | `no` | add `scp -s` (force SFTP protocol). Needed with OpenSSH 8.7–8.9 clients |

## 5. Timing

| Key | Default | Meaning |
|---|---|---|
| `POLL_INTERVAL_SEC` | `2` | how often the outbox is scanned |
| `SETTLE_CHECKS` | `2` | polls in a row an item's size must stay unchanged before it is sent (a folder counts as its total size) |
| `REGISTER_INTERVAL_SEC` | `45` | directory mode: how often the agent registers |
| `LOG_FILE` | *(empty)* | empty = journal (`journalctl -u filedrop-agent`) |

Transfer timeout: 60 s + size ÷ 2 MB/s, at most 20 minutes.

## 6. Auto-cleanup (`/etc/filedrop/cleanup.conf`)

| Key | Default | Meaning |
|---|---|---|
| `RETENTION_DAYS` | `14` | delete items older than this; warnings 4 days before, 1 day before and on the last day |
| `NOTICE_LANG` | `auto` | `auto`, `en` or `ru` |

The age of a folder is the age of the newest file inside it. The cleanup runs at boot and daily (`systemctl list-timers | grep filedrop`).

## Helper commands

```sh
filedrop-agent check                     # full diagnosis of this machine
FILEDROP_HOSTNAME=pc-room12 filedrop-agent info    # test a scheme without renaming the machine
```
