# Changelog

Format: [Keep a Changelog](https://keepachangelog.com/). Versions follow [SemVer](https://semver.org/).

## [2.0.0] — 2026-10-05

First public release. Based on the internal 1.0.0-x line that ran in production on ~80 classroom machines.

### Added
- **Subnet-aware pairing.** A laptop and its panel may be in the same subnet or in different ones, with one or several addresses each, mixed freely on one site. Agents report all their local addresses (with masks) to the directory; the counterpart's addresses come back as a list; the agent prefers addresses on its own subnets, never uses its own addresses, ignores container/VM bridges (`IGNORE_INTERFACES`) and sends to the first address whose SSH port answers. `dns` (all A records) and `static` (several addresses per line) get the same treatment. See `docs/networks.md`.
- `filedrop-agent check` shows this machine's addresses and, per counterpart address, "same subnet" / "other subnet, routed" and port reachability.
- **Configurable naming scheme**: `HOSTNAME_PATTERN` (regex with a `role` group), `PANEL_ROLES`, `CLIENT_ROLES`, `PEER_HOSTNAMES`, `ROLE` override.
- **`dns` resolve mode** (system resolver) in addition to `directory` and `static`.
- Subcommands `filedrop-agent check`, `info`, `version`.
- `filedrop-keygen` and `filedrop-setup --key | --generate-key | --authorize`: the SSH key is created by the administrator, **no key ships in any package**.
- Packages: `.deb`, `.rpm`, generic `.tar.gz` installer, for amd64 and arm64; `filedrop-directory` tarball; `scripts/rollout.sh` bulk installer.
- GitHub Actions: CI (format, vet, tests, shellcheck, secrets scan, package build) and tag-triggered releases.
- English/Russian documentation; English/Russian cleanup pop-ups (`NOTICE_LANG`); configurable shortcut name (`SHARED_FOLDER_NAME`).
- `SCP_SFTP_FLAG` for OpenSSH 8.7–8.9 clients.

### Changed
- **Breaking:** the school-specific hostname scheme is no longer built in. A 1.x fleet must add `HOSTNAME_PATTERN`/`PANEL_ROLES`/`CLIENT_ROLES` (see `examples/structured-names/agent.conf`).
- **Breaking:** the shared fleet key is no longer inside the package. Install your existing key with `filedrop-setup --key`.
- Default `RESOLVE_MODE` is `dns` (was `directory`); default `SHARED_FOLDER_NAME` is `Shared folder` (set `Общая папка` to keep the old name).
- Pinned host keys now live in `/var/lib/filedrop/known_hosts` (was `/etc/filedrop/known_hosts`, which the hardened unit could not write).
- The directory service compares tokens in constant time.

### Carried over from 1.0.0-x
- Whole **folders** are transferred (with the receiving side able to move/delete them); big transfers get a size-based timeout.
- The agent runs with `CAP_DAC_OVERRIDE` and `CAP_FOWNER` only, so it can chmod/move items users own; delivered-but-unmovable items are not resent in a loop.
- Static, glibc-independent binaries.
- Several directory URLs with sticky "last good" selection (a server with an address in several subnets); the same `DIRECTORY_URL` list is now meant for all machines.
- Auto-cleanup of files and folders with 4-day / 1-day / last-day warnings; self-healing desktop shortcut.

[2.0.0]: https://github.com/AxelMad/filedrop/releases/tag/v2.0.0
