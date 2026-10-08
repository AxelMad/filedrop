# Публикация filedrop на GitHub — пошаговая инструкция

*English version: [GITHUB.en.md](GITHUB.en.md)*

Инструкция рассчитана на человека, который раньше не публиковал проекты на GitHub. Время — около 20–30 минут. Команды даны для Linux/macOS; на Windows используйте **Git Bash** (ставится вместе с Git for Windows), команды те же.

---

## Шаг 0. Что понадобится

1. **Аккаунт на GitHub.** Если нет — [github.com/signup](https://github.com/signup): почта, пароль, логин. Логин станет частью адреса проекта (`github.com/ЛОГИН/filedrop`), выбирайте так, чтобы не стыдно было показать.
2. **Двухфакторная аутентификация** (*Settings → Password and authentication → Two-factor authentication*). С 2023 года GitHub требует её для всех, кто публикует код.
3. **Git** на компьютере: `git --version`. Нет — `sudo apt install git` (Debian/Ubuntu), `sudo dnf install git` (RPM), [git-scm.com](https://git-scm.com/download/win) (Windows).
4. Представьтесь git один раз. Чтобы не светить настоящую почту в истории, возьмите адрес-«прокладку» GitHub: *Settings → Emails → Keep my email addresses private*, там же показан адрес вида `12345+ЛОГИН@users.noreply.github.com`.

```sh
git config --global user.name  "Ваше Имя"
git config --global user.email "12345+ЛОГИН@users.noreply.github.com"
git config --global init.defaultBranch main
```

## Шаг 1. Подготовьте папку проекта

Распакуйте архив с репозиторием (`filedrop-repo.tar.gz`) и перейдите в папку:

```sh
tar -xzf filedrop-repo.tar.gz
cd filedrop
ls          # должны быть: cmd packaging scripts docs examples README.md LICENSE ...
```

**1.1. Подставьте свои данные.**

- `LICENSE` — замените `filedrop contributors` на своё имя/организацию (по желанию: «Copyright (c) 2026 Ваше Имя»).
- `CHANGELOG.md` — в последней строке замените `OWNER` на свой логин GitHub.
- `scripts/build.sh` — значение `REPO_URL` по умолчанию содержит `OWNER`; это нужно только для локальной сборки, в GitHub Actions адрес подставляется автоматически. Можно оставить.
- В примерах (`examples/`) нет ничего вашего: адреса там вымышленные (в проекте нет ни номеров школ, ни названий площадок). Убедитесь, что и в ваших правках не появились реальные IP-адреса, имена хостов школы, токены.

**1.2. Проверьте, что в проекте нет секретов.** Это самый важный шаг: всё, что попало на публичный GitHub, считайте опубликованным навсегда, даже если потом удалить.

```sh
./scripts/check-secrets.sh
```

Должно быть `nothing suspicious found`. Дополнительно, глазами:

```sh
grep -rniE 'password|token|secret|BEGIN .*PRIVATE' --exclude-dir=.git --exclude-dir=dist . | grep -v 'docs/\|README\|CHANGELOG\|SECURITY\|scripts/check-secrets.sh'
```

Рабочие файлы вашей школы (`rollout.conf` с реальными адресами, `fleet_key`, `agent.conf` с токеном справочника, `hosts.tsv`) в репозиторий **не кладите** — они уже перечислены в `.gitignore`. Храните их отдельно (например, в закрытом хранилище школы).

**1.3. Проверьте, что всё собирается и тесты проходят** (нужен Go ≥ 1.22: `go version`):

```sh
make test
make packages     # необязательно: соберёт пакеты в dist/ (папка dist/ в git не попадёт)
```

## Шаг 2. Создайте репозиторий на GitHub

1. Откройте [github.com/new](https://github.com/new).
2. **Repository name:** `filedrop` (можно любое другое имя — тогда замените его ниже).
3. **Description:** `Drop a file into the "Shared folder" on a laptop — it appears on the classroom panel. Linux, Go, SFTP.`
4. **Public** — чтобы проектом могли пользоваться другие (Private тоже работает, но релизы и Actions будут видны только вам и приглашённым; для приватных репозиториев лимит бесплатных минут Actions ограничен).
5. **Не** отмечайте *Add a README*, *Add .gitignore* и *Choose a license* — всё это уже есть в проекте, иначе GitHub создаст конфликтующий первый коммит.
6. Нажмите **Create repository**. Откроется пустая страница с адресом репозитория:
   `https://github.com/ЛОГИН/filedrop.git`

## Шаг 3. Загрузите код (git init → push)

### 3.1. Первый коммит

В папке проекта:

```sh
git init -b main
git add .
git status            # просмотрите список файлов: не должно быть ключей, dist/, rollout.conf
git commit -m "filedrop 2.0.0: initial public release"
```

### 3.2. Подключитесь к GitHub — один из двух способов

**Способ A (проще): GitHub CLI `gh`.** Установите ([cli.github.com](https://cli.github.com/)), затем:

```sh
gh auth login                  # GitHub.com → HTTPS → Login with a web browser; повторите код в браузере
git remote add origin https://github.com/ЛОГИН/filedrop.git
git push -u origin main
```

*(Репозиторий можно было создать и целиком из консоли: `gh repo create filedrop --public --source=. --push` — вместо шагов 2 и 3.2.)*

**Способ B: HTTPS + персональный токен.** При `git push` GitHub спросит логин и пароль, **вместо пароля нужен токен**:

1. *Settings → Developer settings → Personal access tokens → Fine-grained tokens → Generate new token*.
2. Resource owner — вы; Repository access — *Only select repositories* → `filedrop`; Permissions → **Contents: Read and write** и **Workflows: Read and write** (без второго GitHub отклонит push файлов из `.github/workflows`).
3. Скопируйте токен (показывается один раз) и вставьте его в ответ на запрос пароля:

```sh
git remote add origin https://github.com/ЛОГИН/filedrop.git
git push -u origin main
```

Токен — это пароль: никому не отправляйте и не коммитьте. Чтобы не вводить его каждый раз: `git config --global credential.helper store` (Linux) — токен сохранится в открытом виде в `~/.git-credentials`; на Windows/macOS подойдёт встроенный менеджер учётных данных.

**Способ C: SSH-ключ.** `ssh-keygen -t ed25519 -C "git@github"`, затем содержимое `~/.ssh/id_ed25519.pub` добавьте в *Settings → SSH and GPG keys*, адрес remote — `git@github.com:ЛОГИН/filedrop.git`. (Это отдельный ключ для GitHub, не путайте с ключом парка filedrop!)

### 3.3. Проверьте результат

Обновите страницу репозитория: должны появиться файлы и отрисованный `README`. На вкладке **Actions** запустился workflow **CI** — через 2–4 минуты он должен стать зелёным ✔. Если красный — откройте его, посмотрите, какой шаг упал (`gofmt`, `shellcheck`, `tests`…), исправьте локально, `git commit`, `git push`.

> Если Actions выключены (*Actions are disabled*): вкладка **Actions → I understand my workflows, go ahead and enable them**.

## Шаг 4. Настройка страницы репозитория

1. **About** (шестерёнка справа вверху на главной странице): добавьте описание и **Topics** — `classroom`, `interactive-panel`, `file-transfer`, `sftp`, `golang`, `linux`, `education`, `school`. Так проект проще найти.
2. *Settings → General*: отключите ненужные *Wikis* и *Projects*, оставьте *Issues* — туда будут писать об ошибках.
3. *Settings → Code security*: включите **Private vulnerability reporting**, **Dependabot alerts** и **Secret scanning** (последний — страховка на случай, если секрет всё-таки попадёт в репозиторий).
4. *Settings → Branches → Add branch ruleset* (необязательно, но полезно, когда появятся помощники): для `main` потребовать pull request и успешный workflow **CI** перед слиянием.

## Шаг 5. Первый релиз с пакетами

Релиз собирается автоматически, когда вы отправляете тег вида `vX.Y.Z`: workflow **Release** прогонит тесты, соберёт `.deb`, `.rpm`, `.tar.gz` (amd64 и arm64), справочник и файл `SHA256SUMS` и приложит всё к странице релиза.

```sh
cat VERSION                                  # 2.0.0 — должно совпадать с тегом
git tag -a v2.0.0 -m "filedrop 2.0.0"
git push origin v2.0.0
```

Откройте **Actions → Release** и дождитесь зелёной галочки; затем справа на главной странице репозитория, в блоке **Releases**, появится **v2.0.0** со всеми файлами (*Assets*). Описание изменений GitHub сгенерирует сам; при желании отредактируйте его кнопкой ✏ на странице релиза и вставьте раздел из `CHANGELOG.md`.

**Если Release завершился ошибкой `Resource not accessible by integration`:** *Settings → Actions → General → Workflow permissions* → **Read and write permissions** → Save, затем перезапустите упавший запуск (*Re-run jobs*).

**Ручной вариант без Actions** (если нужно срочно): `make packages`, затем на странице репозитория *Releases → Draft a new release*, выберите/создайте тег `v2.0.0`, перетащите в поле *Attach binaries* все файлы из `dist/`, нажмите **Publish release**.

### Проверка на двух схемах сети (рекомендуется перед раскаткой)

Программа не привязана к схеме сети, но один раз проверьте обе, на какой-нибудь паре «ноутбук + панель» (подробности — [docs/networks.ru.md](docs/networks.ru.md)):

1. **Одна подсеть:** на ноутбуке `filedrop-agent check` должен показать `(same subnet as …)` и `port 22 is reachable`.
2. **Разные подсети:** то же, но с пометкой `(other subnet, routed)`; если порт недоступен — между подсетями закрыт TCP 22 (подсеть ноутбука → подсеть панели).
3. Положите файл в «Общую папку» — он должен появиться на панели в обеих схемах. Если у сервера справочника несколько адресов, перечислите их все в `DIRECTORY_URL` — одна и та же строка подходит для всех машин.

### Проверка скачанного релиза

```sh
sha256sum -c SHA256SUMS --ignore-missing     # в папке со скачанными файлами
sudo apt install ./filedrop-agent_2.0.0_amd64.deb
filedrop-agent version                       # filedrop-agent 2.0.0
```

## Шаг 6. Как выпускать новые версии

1. Внесите изменения, `make test`, `./scripts/check-secrets.sh`.
2. Увеличьте номер в файле `VERSION` (`2.0.1` — исправление ошибки, `2.1.0` — новая возможность, `3.0.0` — несовместимое изменение) и допишите раздел в `CHANGELOG.md`.
3. `git add -A && git commit -m "Release 2.0.1" && git push`.
4. `git tag -a v2.0.1 -m "filedrop 2.0.1" && git push origin v2.0.1` — релиз соберётся сам.

Номер в теге — единственный источник версии для пакетов, собранных в Actions (`VERSION` нужен для локальной сборки): держите их одинаковыми.

## Шаг 7. Типичные проблемы

| Сообщение | Что делать |
|---|---|
| `remote: Permission denied` / `Authentication failed` | Вместо пароля нужен токен или `gh auth login` (шаг 3.2); проверьте логин в адресе remote: `git remote -v` |
| `refusing to allow a Personal Access Token to create or update workflow … without workflow scope` | У токена нет права **Workflows** — создайте токен заново с ним (или используйте `gh auth login`) |
| `failed to push some refs … (fetch first)` | В репозитории уже есть коммит (вы отметили README/лицензию при создании). Либо `git pull --rebase origin main --allow-unrelated-histories` и снова `git push`, либо удалите репозиторий и создайте пустой |
| Скрипты в Windows-клоне падают с `bad interpreter` / `\r` | Окончания строк CRLF; в проекте есть `.gitattributes`, который заставляет git использовать LF. Клонируйте заново после его добавления |
| `error: src refspec main does not match any` | Нет ни одного коммита или ветка называется `master`: `git branch -M main`, затем `git push -u origin main` |
| CI красный на `shellcheck` | `make lint` локально покажет то же самое; установка: `pip install shellcheck-py` |

## Шаг 8. Если секрет всё-таки попал в репозиторий

Считайте его **скомпрометированным немедленно** — удаление из истории не вернёт приватность.

1. **Замените ключ парка:** `filedrop-keygen ./new-key`, затем `sudo filedrop-setup --key ./new-key/fleet_key` на всех машинах (или `scripts/rollout.sh`). Токен справочника — `sudo ./install.sh НОВЫЙ_ТОКЕН` на сервере справочника и правка `DIRECTORY_TOKEN` на агентах.
2. Если токен GitHub — отзовите его в *Settings → Developer settings*.
3. Затем почистите историю: [`git filter-repo`](https://github.com/newren/git-filter-repo) (`git filter-repo --path путь/к/файлу --invert-paths`) и `git push --force`, либо, если репозиторий новый и пока никому не нужен, просто удалите его на GitHub и создайте заново.

## Краткая шпаргалка

```sh
./scripts/check-secrets.sh && make test            # проверка перед публикацией
git init -b main && git add . && git commit -m "filedrop 2.0.0"
git remote add origin https://github.com/ЛОГИН/filedrop.git
git push -u origin main                            # код на GitHub
git tag -a v2.0.0 -m "filedrop 2.0.0" && git push origin v2.0.0   # релиз с пакетами
```
