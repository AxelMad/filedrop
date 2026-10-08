# Security policy

## Reporting a vulnerability

Please **do not open a public issue** for a security problem. Use GitHub's private reporting instead: *Security → Report a vulnerability* on the repository page (enable it under *Settings → Code security → Private vulnerability reporting*). Include the version (`filedrop-agent version`), what you observed and how to reproduce it.

## Supported versions

Only the latest release receives fixes.

## Things that are by design

- filedrop assumes a trusted classroom LAN. The directory service speaks plain HTTP with a shared token; do not expose it to untrusted networks.
- One fleet key is shared by all machines. It only allows dropping files into a panel's inbox (chrooted SFTP account, no shell, no forwarding). If it leaks, rotate it: `filedrop-keygen` a new one and `filedrop-setup --key` it everywhere (see `scripts/rollout.sh`).
