# How it works, and why it is safe

*Русская версия: [architecture.ru.md](architecture.ru.md)*

## The data path

1. A user drops a file or folder into the shared folder = `/opt/filedrop/outbox` (the desktop shortcut points there; the directory is mode `1777`, like `/tmp`).
2. The **client agent** scans the outbox every 2 s. An entry is "settled" when its size (for a folder: the total size of all files) has not changed for `SETTLE_CHECKS` polls, so half-copied files are never sent.
3. It derives the counterpart's hostname from its own (naming scheme), resolves **all addresses** of the counterpart (`dns` / `directory` / `static`), puts those on its own subnets first, picks the first whose SSH port answers ([networks.md](networks.md)) and runs the system `scp` with the dedicated key (`-r` for folders). Timeout: 60 s + size ÷ 2 MB/s, at most 20 min.
4. On the **panel**, sshd maps the account `filedrop` to `ChrootDirectory /opt/filedrop` with `ForceCommand internal-sftp`; the item lands in `/opt/filedrop/inbox` (`/inbox` inside the chroot), which the desktop shortcut on the panel points to.
5. After success the client moves the item to `outbox/.sent/<timestamp>_<name>`. If that move fails (the transfer *did* succeed) the agent remembers the item as delivered and does not resend it until it changes.
6. Auto-cleanup deletes items older than `RETENTION_DAYS`, with warnings.

The directory service (optional) is only a phone book: agents `POST /register` with all their local addresses (the server also records the source IP) and `GET /resolve?name=…`.

## Security model

- **The key can do one thing.** The panel's `filedrop` account has no shell, no TTY, no password, no port/agent/X11 forwarding and is confined to an SFTP chroot; the `authorized_keys` entry adds `no-port-forwarding,no-agent-forwarding,no-X11-forwarding,no-pty`. Whoever steals the fleet key can drop files into a panel's inbox — nothing else. They cannot read files from other machines or run commands.
- **No key in any package or in the repository.** You generate it (`filedrop-keygen`) and install it with `filedrop-setup --key`. `.gitignore` excludes the usual key file names.
- **chroot hygiene.** `/opt/filedrop` is `root:root 0755` (sshd refuses a chroot that anyone else can write to). The inbox is `0777` without sticky bit so that whoever sits at the panel can delete received items, but not the folder itself.
- **The agent is unprivileged.** It runs as user `filedrop` under systemd with `NoNewPrivileges`, `ProtectSystem=strict` (writes only to `/opt/filedrop` and `/var/lib/filedrop`), `ProtectHome` and exactly two capabilities, `CAP_DAC_OVERRIDE` and `CAP_FOWNER` — needed to chmod and move items that users, not the agent, own (sticky outbox, folders created by teachers).
- **Network trust.** filedrop assumes a classroom LAN. Host keys are pinned on first contact (`accept-new`). The directory service authenticates agents with a shared token (constant-time comparison) but speaks plain HTTP: do not expose it beyond the school network.

## Why these technical choices

- **System `scp`, not an SSH library:** OpenSSH is already on every machine and behaves identically when you reproduce a failure by hand.
- **Static Go binary (`CGO_ENABLED=0`):** a dynamically linked build broke on distributions with an older glibc (`GLIBC_2.34 not found`); a static one runs everywhere.
- **Directories get mode 0777 on the sender before `scp -r`:** the receiving sftp-server creates directories with exactly the sender's modes (no umask), and the receiving user must be able to move and delete them.
- **Shortcut self-healing instead of `chattr +i`:** the kernel does not support attributes on symlinks; a root timer recreates the shortcut every ~2 minutes instead.

## Layout of the repository

```
cmd/filedrop-agent/       the agent (naming, config, resolvers, sender, receiver)
cmd/filedrop-directory/   optional hostname→IP directory service
packaging/common/         files shared by all packages (units, sshd drop-in, setup/keygen scripts)
packaging/{rpm,deb,tar,directory}/   package specifics
scripts/build.sh          builds everything into dist/;  scripts/rollout.sh  bulk installer
examples/                 sample configurations
```
