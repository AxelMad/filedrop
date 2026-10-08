# Диагностика и частые проблемы

*English version: [troubleshooting.md](troubleshooting.md)*

**Начните отсюда, на ноутбуке/ПК:**

```sh
filedrop-agent check            # роль, папки, ключ, IP пары, доступность по TCP
systemctl status filedrop-agent
journalctl -u filedrop-agent -n 50 --no-pager
```

На панели: `filedrop-agent check` и `journalctl -u ssh -u sshd -n 50` (служба называется `ssh` в семействе Debian и `sshd` в семействе RPM).

| Симптом | Вероятная причина и решение |
|---|---|
| `hostname "x" does not match HOSTNAME_PATTERN` | Имя машины не подходит под схему. Переименуйте, либо поправьте `HOSTNAME_PATTERN`/`PANEL_ROLES`/`CLIENT_ROLES`, либо задайте `ROLE=` и `PEER_HOSTNAMES=`. Проверить, не переименовывая: `FILEDROP_HOSTNAME=pc-room12 filedrop-agent info` |
| `check`: пара не резолвится | режим `dns`: имени нет в DNS — проверьте `getent hosts panel-room12`, перейдите на `directory` или `static`. Режим `directory`: *вторая* машина ещё не зарегистрировалась — смотрите `/status` справочника |
| `check`: `cannot connect to … port 22` | Смотрите пометку: `same subnet` ⇒ на панели не запущен sshd (`systemctl status ssh`) или локальный файрвол; `other subnet, routed` ⇒ маршрутизатор/файрвол между подсетями блокирует TCP 22 (подсеть ноутбука → подсеть панели) или нет маршрута. См. [networks.ru.md](networks.ru.md). Сравните с адресами на `/status` справочника |
| `directory URL … is not reachable, using … instead` (журнал) | Информационное: первый `DIRECTORY_URL` недоступен с этой машины; переставьте адреса |
| `Permission denied (publickey)` | На панели нет подходящего публичного ключа: выполните там `sudo filedrop-setup --key fleet_key` (или `--authorize`). Проверьте `ls -ld /opt/filedrop /opt/filedrop/.ssh`, `authorized_keys` должен быть `root:filedrop 0640`; точную причину назовёт `journalctl -u ssh` |
| `UNPROTECTED PRIVATE KEY FILE` | Ключ должен принадлежать `filedrop` с правами 0600: выполните `sudo filedrop-setup` |
| `subsystem request failed` / `scp: Connection closed` / ошибки протокола | Клиентский scp говорит на старом протоколе SCP, а панель разрешает только SFTP. Нужен OpenSSH ≥ 9.0 либо ≥ 8.7 с `SCP_SFTP_FLAG=yes` |
| sshd: `bad ownership or modes for chroot directory` | `/opt/filedrop` должен быть `root:root 0755`: выполните `sudo filedrop-setup` |
| sshd не читает drop-in | в `/etc/ssh/sshd_config` нет `Include /etc/ssh/sshd_config.d/*.conf` — `filedrop-setup` добавляет его; проверка: `sudo sshd -T \| grep -i chroot` (в некоторых версиях нужен `-C user=filedrop`) |
| Файл остаётся в общей папке, в журнале повторяются ошибки отправки | Прочитайте ошибку: определение адреса, сеть или ssh. Передача повторяется каждые несколько секунд, пока не получится |
| Папка пришла, но пользователь панели не может её удалить | Старая версия агента (до того, как каталоги стали делаться доступными всем на стороне отправителя) — обновите |
| Папка остаётся в outbox после доставки / отправляется снова и снова | У агента нет `CAP_DAC_OVERRIDE`/`CAP_FOWNER` (юнит правили руками?). Сравните с `packaging/common/filedrop-agent.service` |
| `GLIBC_2.xx not found` | Запущен самостоятельно собранный динамический бинарник. Релизные пакеты статические; собирайте с `CGO_ENABLED=0` |
| Нет ярлыка на рабочем столе | `filedrop-desktop.timer` восстанавливает его раз в ~2 мин; проверьте `systemctl list-timers \| grep filedrop` и `/var/log/filedrop-desktop-link.log`. Роль должна определяться (`filedrop-agent info`) |
| Нет окна автоочистки | нужен `zenity` и графическая сессия X11; само удаление работает и без него |

## Проверка SFTP-пути вручную (с клиента)

```sh
sudo -u filedrop sftp -i /etc/filedrop/id_ed25519 -o UserKnownHostsFile=/var/lib/filedrop/known_hosts \
     -o StrictHostKeyChecking=accept-new filedrop@IP_ПАНЕЛИ
sftp> ls /inbox
```

## Не помогло?

Создайте issue с выводом `filedrop-agent check`, `filedrop-agent version`, `ssh -V` на обеих машинах и последними строками журнала (предварительно уберите токены).
