# Troubleshooting

*Русская версия: [troubleshooting.ru.md](troubleshooting.ru.md)*

**Start here, on the laptop/PC:**

```sh
filedrop-agent check            # role, folders, key, counterpart's IP, TCP reachability
systemctl status filedrop-agent
journalctl -u filedrop-agent -n 50 --no-pager
```

On the panel: `filedrop-agent check` and `journalctl -u ssh -u sshd -n 50` (the unit is `ssh` on Debian-family systems, `sshd` on RPM-family ones).

| Symptom | Likely cause and fix |
|---|---|
| `hostname "x" does not match HOSTNAME_PATTERN` | The machine's name does not follow the scheme. Rename it, or change `HOSTNAME_PATTERN`/`PANEL_ROLES`/`CLIENT_ROLES`, or force `ROLE=` and `PEER_HOSTNAMES=`. Test without renaming: `FILEDROP_HOSTNAME=pc-room12 filedrop-agent info` |
| `check`: counterpart does not resolve | `dns` mode: the name is not in DNS — try `getent hosts panel-room12`; use `directory` or `static` mode. `directory` mode: the *other* machine is not registered yet — look at `/status` of the directory |
| `check`: `cannot connect to … port 22` | Look at the label: `same subnet` ⇒ sshd not running on the panel (`systemctl status ssh`) or a host firewall; `other subnet, routed` ⇒ the router/firewall between the subnets blocks TCP 22 (laptop subnet → panel subnet), or there is no route. See [networks.md](networks.md). Compare with the addresses on `/status` of the directory |
| `directory URL … is not reachable, using … instead` (journal) | Informational: the first `DIRECTORY_URL` is not the one reachable from this machine; reorder them |
| `Permission denied (publickey)` | The panel does not have the matching public key: `sudo filedrop-setup --key fleet_key` there (or `--authorize`). Also check `ls -ld /opt/filedrop /opt/filedrop/.ssh` and that `authorized_keys` is `root:filedrop 0640`; `journalctl -u ssh` names the exact reason |
| `UNPROTECTED PRIVATE KEY FILE` | The key must be owned by `filedrop` with mode 0600: run `sudo filedrop-setup` |
| `subsystem request failed` / `scp: Connection closed` / protocol errors | The client's scp speaks the legacy SCP protocol but the panel allows SFTP only. Use OpenSSH ≥ 9.0, or ≥ 8.7 with `SCP_SFTP_FLAG=yes` |
| sshd: `bad ownership or modes for chroot directory` | `/opt/filedrop` must be `root:root 0755`: run `sudo filedrop-setup` |
| sshd ignores the drop-in | `/etc/ssh/sshd_config` has no `Include /etc/ssh/sshd_config.d/*.conf` — `filedrop-setup` adds it; verify with `sudo sshd -T \| grep -i chroot` (needs `-C user=filedrop` on some versions) |
| File stays in the shared folder, journal shows repeated send errors | Read the error: resolution, network, or ssh. A transfer is retried every few seconds until it works |
| Folder arrives but the panel user cannot delete it | Old agent version (before directories were made world-writable on the sender) — upgrade |
| Folder stays in the outbox after being delivered / is sent again and again | The agent lacks `CAP_DAC_OVERRIDE`/`CAP_FOWNER` (hand-edited unit file?). Compare with `packaging/common/filedrop-agent.service` |
| `GLIBC_2.xx not found` | You are running a self-built, dynamically linked binary. Release packages are static; build with `CGO_ENABLED=0` |
| Shortcut missing on the desktop | It is restored every ~2 min by `filedrop-desktop.timer`; check `systemctl list-timers \| grep filedrop` and `/var/log/filedrop-desktop-link.log`. The role must be resolvable (`filedrop-agent info`) |
| No cleanup pop-up | needs `zenity` and a graphical X11 session; deletion itself works without it |

## Testing the SFTP path by hand (from a client)

```sh
sudo -u filedrop sftp -i /etc/filedrop/id_ed25519 -o UserKnownHostsFile=/var/lib/filedrop/known_hosts \
     -o StrictHostKeyChecking=accept-new filedrop@PANEL_IP
sftp> ls /inbox
```

## Still stuck?

Open an issue with the output of `filedrop-agent check`, `filedrop-agent version`, `ssh -V` on both machines and the last journal lines (remove tokens first).
