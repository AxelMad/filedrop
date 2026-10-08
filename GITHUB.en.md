# Publishing filedrop on GitHub — step by step

*Русская версия: [GITHUB.md](GITHUB.md)*

Written for someone who has never published a project on GitHub. About 20–30 minutes. Commands are for Linux/macOS; on Windows use **Git Bash** (installed with Git for Windows) — the commands are the same.

---

## Step 0. What you need

1. **A GitHub account** — [github.com/signup](https://github.com/signup). Your username becomes part of the project's address (`github.com/USERNAME/filedrop`).
2. **Two-factor authentication** (*Settings → Password and authentication*). GitHub requires it from everyone who contributes code.
3. **Git**: `git --version`. If missing: `sudo apt install git`, `sudo dnf install git`, or [git-scm.com](https://git-scm.com/download/win) on Windows.
4. Introduce yourself to git once. To keep your real e-mail out of the history use GitHub's private address: *Settings → Emails → Keep my email addresses private* (shown there as `12345+USERNAME@users.noreply.github.com`).

```sh
git config --global user.name  "Your Name"
git config --global user.email "12345+USERNAME@users.noreply.github.com"
git config --global init.defaultBranch main
```

## Step 1. Prepare the project folder

Unpack the repository archive (`filedrop-repo.tar.gz`) and enter the folder:

```sh
tar -xzf filedrop-repo.tar.gz
cd filedrop
ls          # cmd packaging scripts docs examples README.md LICENSE ...
```

**1.1. Put in your details.**

- `LICENSE` — replace `filedrop contributors` with your name or organisation.
- `CHANGELOG.md` — in the last line replace `OWNER` with your GitHub username.
- `scripts/build.sh` — the default `REPO_URL` contains `OWNER`; it only matters for local builds (GitHub Actions fills in the real address). You may leave it.
- Make sure nothing of your own site slipped into the files: real IP addresses, internal hostnames, tokens.

**1.2. Check that there are no secrets.** The most important step: anything pushed to a public GitHub repository must be considered published forever, even if deleted later.

```sh
./scripts/check-secrets.sh
```

Expect `nothing suspicious found`. Your site's working files (`rollout.conf` with real addresses, `fleet_key`, an `agent.conf` containing the directory token, `hosts.tsv`) do **not** belong in the repository — `.gitignore` already lists them. Keep them elsewhere (a private store).

**1.3. Check that it builds and the tests pass** (Go ≥ 1.22):

```sh
make test
make packages     # optional: builds packages into dist/ (dist/ is git-ignored)
```

## Step 2. Create the repository on GitHub

1. Open [github.com/new](https://github.com/new).
2. **Repository name:** `filedrop` (another name works, substitute it below).
3. **Description:** `Drop a file into the "Shared folder" on a laptop — it appears on the classroom panel. Linux, Go, SFTP.`
4. **Public** (anyone can use the project and download releases) — or Private if you prefer.
5. Do **not** tick *Add a README*, *Add .gitignore*, *Choose a license* — the project already has them, and GitHub would create a conflicting first commit.
6. Click **Create repository**. You get an empty page with the address `https://github.com/USERNAME/filedrop.git`.

## Step 3. Upload the code (git init → push)

### 3.1. First commit

```sh
git init -b main
git add .
git status            # review the list: no keys, no dist/, no rollout.conf
git commit -m "filedrop 2.0.0: initial public release"
```

### 3.2. Connect to GitHub — pick one way

**Way A (easiest): GitHub CLI `gh`.** Install it from [cli.github.com](https://cli.github.com/), then:

```sh
gh auth login                  # GitHub.com → HTTPS → Login with a web browser
git remote add origin https://github.com/USERNAME/filedrop.git
git push -u origin main
```

*(You could even skip step 2 and run: `gh repo create filedrop --public --source=. --push`.)*

**Way B: HTTPS + personal access token.** `git push` asks for a username and password — **use a token instead of the password**:

1. *Settings → Developer settings → Personal access tokens → Fine-grained tokens → Generate new token*.
2. Repository access → *Only select repositories* → `filedrop`; Permissions → **Contents: Read and write** and **Workflows: Read and write** (without the latter GitHub rejects pushes that contain `.github/workflows`).
3. Copy the token (shown once) and paste it when asked for the password.

```sh
git remote add origin https://github.com/USERNAME/filedrop.git
git push -u origin main
```

A token is a password: never share or commit it. `git config --global credential.helper store` saves it in plain text in `~/.git-credentials`; Windows/macOS have secure built-in credential managers.

**Way C: SSH key.** `ssh-keygen -t ed25519 -C "git@github"`, add `~/.ssh/id_ed25519.pub` under *Settings → SSH and GPG keys*, use the remote `git@github.com:USERNAME/filedrop.git`. (This is a separate key for GitHub — do not confuse it with the filedrop fleet key!)

### 3.3. Verify

Reload the repository page: the files and the rendered `README` should be there. On the **Actions** tab the **CI** workflow starts; after 2–4 minutes it should be green ✔. If it is red, open it, see which step failed (`gofmt`, `shellcheck`, tests …), fix locally, commit, push.

> If Actions are disabled: **Actions → I understand my workflows, go ahead and enable them**.

## Step 4. Polish the repository page

1. **About** (gear icon at the top right of the main page): add a description and **Topics** — `classroom`, `interactive-panel`, `file-transfer`, `sftp`, `golang`, `linux`, `education`, `school`.
2. *Settings → General*: untick *Wikis* and *Projects* if unused; keep *Issues* for bug reports.
3. *Settings → Code security*: enable **Private vulnerability reporting**, **Dependabot alerts** and **Secret scanning** (a safety net if a secret ever slips in).
4. *Settings → Branches → Add branch ruleset* (optional): require a pull request and a passing **CI** for `main` once you have collaborators.

## Step 5. The first release with packages

A release is built automatically when you push a tag like `vX.Y.Z`: the **Release** workflow runs the tests, builds `.deb`, `.rpm`, `.tar.gz` (amd64 and arm64), the directory service and `SHA256SUMS`, and attaches everything to the release page.

```sh
cat VERSION                                  # 2.0.0 — should match the tag
git tag -a v2.0.0 -m "filedrop 2.0.0"
git push origin v2.0.0
```

Watch **Actions → Release** turn green; then **v2.0.0** appears under **Releases** on the main page with all files as *Assets*. GitHub generates the notes; edit them with the ✏ button and paste the section from `CHANGELOG.md` if you like.

**If the workflow fails with `Resource not accessible by integration`:** *Settings → Actions → General → Workflow permissions* → **Read and write permissions** → Save, then *Re-run jobs*.

**Manual release without Actions** (in a hurry): `make packages`, then *Releases → Draft a new release*, create the tag `v2.0.0`, drag every file from `dist/` into *Attach binaries*, **Publish release**.

### Check both network layouts (recommended before a fleet rollout)

The program does not depend on a particular network layout, but verify both once on any laptop + panel pair (details: [docs/networks.md](docs/networks.md)):

1. **Same subnet:** `filedrop-agent check` on the laptop must show `(same subnet as …)` and `port 22 is reachable`.
2. **Different subnets:** the same, labelled `(other subnet, routed)`; if the port is unreachable, TCP 22 is blocked between the subnets (laptop subnet → panel subnet).
3. Drop a file into the shared folder — it must appear on the panel in both layouts. If the directory server has several addresses, list all of them in `DIRECTORY_URL`; the same line works on every machine.

### Verifying a downloaded release

```sh
sha256sum -c SHA256SUMS --ignore-missing     # in the folder with the downloaded files
sudo apt install ./filedrop-agent_2.0.0_amd64.deb
filedrop-agent version                       # filedrop-agent 2.0.0
```

## Step 6. Releasing new versions

1. Make your changes; `make test` and `./scripts/check-secrets.sh`.
2. Bump `VERSION` (`2.0.1` bug fix, `2.1.0` new feature, `3.0.0` breaking change) and add a section to `CHANGELOG.md`.
3. `git add -A && git commit -m "Release 2.0.1" && git push`.
4. `git tag -a v2.0.1 -m "filedrop 2.0.1" && git push origin v2.0.1` — the release builds itself.

For packages built in Actions the tag is the source of the version (`VERSION` serves local builds): keep them equal.

## Step 7. Common problems

| Message | What to do |
|---|---|
| `remote: Permission denied` / `Authentication failed` | Use a token instead of the password, or `gh auth login` (step 3.2); check the username in `git remote -v` |
| `refusing to allow a Personal Access Token to create or update workflow … without workflow scope` | The token lacks **Workflows** permission — recreate it (or use `gh auth login`) |
| `failed to push some refs … (fetch first)` | The repository already has a commit (you ticked README/license). Either `git pull --rebase origin main --allow-unrelated-histories` and push again, or delete the repository and create an empty one |
| Scripts fail on a Windows clone with `bad interpreter` / `\r` | CRLF line endings. `.gitattributes` forces LF; clone again after it is in place |
| `error: src refspec main does not match any` | No commit yet, or the branch is named `master`: `git branch -M main`, then `git push -u origin main` |
| CI red on `shellcheck` | `make lint` shows the same locally; install with `pip install shellcheck-py` |

## Step 8. If a secret did get into the repository

Consider it **compromised immediately** — removing it from history does not restore privacy.

1. **Rotate the fleet key:** `filedrop-keygen ./new-key`, then `sudo filedrop-setup --key ./new-key/fleet_key` on every machine (or `scripts/rollout.sh`). Directory token: `sudo ./install.sh NEW_TOKEN` on the directory server and update `DIRECTORY_TOKEN` on the agents.
2. A GitHub token: revoke it under *Settings → Developer settings*.
3. Then clean the history with [`git filter-repo`](https://github.com/newren/git-filter-repo) (`git filter-repo --path path/to/file --invert-paths`) and `git push --force` — or, if the repository is brand new and nobody uses it, delete it on GitHub and create it anew.

## Cheat sheet

```sh
./scripts/check-secrets.sh && make test            # before publishing
git init -b main && git add . && git commit -m "filedrop 2.0.0"
git remote add origin https://github.com/USERNAME/filedrop.git
git push -u origin main                            # code on GitHub
git tag -a v2.0.0 -m "filedrop 2.0.0" && git push origin v2.0.0   # release with packages
```
