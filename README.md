<div align="center">

# <img src="https://img.shields.io/badge/shync-sync_your_configs-blue?style=for-the-badge&labelColor=0d1117&color=58a6ff" alt="shync" />

**Sync dotfiles and config files across all your devices.**

[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/license-MIT-green?style=flat-square)](LICENSE)
[![Release](https://img.shields.io/github/v/release/quangkhaidam93/shync?style=flat-square&color=ff6b6b)](https://github.com/quangkhaidam93/shync/releases)

<br />

```
  ___| |__  _   _ _ __   ___
 / __| '_ \| | | | '_ \ / __|
 \__ \ | | | |_| | | | | (__
 |___/_| |_|\__, |_| |_|\___|
            |___/
```

Keep your `.zshrc`, `.vimrc`, `.gitconfig`, and other dotfiles in sync
across every machine you work on — backed by storage you already own.

<br />

[Getting Started](#-getting-started) · [Features](#-features) · [Commands](#-commands) · [Snap](#-command-snippets) · [Backup](#-backup-backends) · [Backend Switch](#switching-active-backend) · [Backends](#-supported-backends) · [Docs](docs/)

---

</div>

## ✨ Features

| | Feature | Description |
|---|---------|-------------|
| 🔄 | **Two-way sync** | Upload and download configs with a single command |
| 📊 | **Progress bar** | Real-time upload progress with speed and ETA |
| 🔐 | **Secure auth** | OAuth 2.0 + PKCE for Google Drive, 2FA for Synology, PAT for GitHub Gist |
| 🏠 | **Your storage** | Files live on your Google Drive, Synology NAS, or GitHub Gist — not a third-party server |
| 📌 | **Command snippets** | Save frequently used shell commands and recall them instantly with regex search |
| 🔁 | **Backup backends** | Mirror all your remote files to multiple backends for redundancy |
| ⚡ | **Self-updating** | Built-in `shync update` pulls the latest release automatically |
| 🛠️ | **Zero config** | Interactive `shync init` walks you through setup in seconds |

## 📦 Installation

### Homebrew

```sh
brew install quangkhaidam93/tap/shync
```

### Shell script

```sh
curl -fsSL https://raw.githubusercontent.com/quangkhaidam93/shync/master/install.sh | sh
```

<details>
<summary>Install a specific version</summary>

```sh
VERSION=v0.1.0 curl -fsSL https://raw.githubusercontent.com/quangkhaidam93/shync/master/install.sh | sh
```

</details>

### Go install

```sh
go install github.com/quangkhaidam93/shync@latest
```

## 🚀 Getting Started

### GitHub Gist Setup

The fastest way to get started — just needs a GitHub account:

1. Go to [github.com/settings/tokens/new](https://github.com/settings/tokens/new)
2. Give it a name (e.g. "shync"), check the **gist** scope
3. Click **Generate token** and copy it

```sh
shync init    # select "gist", paste your token
```

shync creates a private gist to store your files. To sync another device, run `shync init` with the same token and enter the Gist ID from your config.

### Google Drive Setup

Before running `shync init`, you need OAuth credentials from Google Cloud Console:

1. **Create a project** — Go to [console.cloud.google.com/projectcreate](https://console.cloud.google.com/projectcreate) and create a new project (e.g. "shync")

2. **Enable Drive API** — Go to [console.cloud.google.com/apis/library/drive.googleapis.com](https://console.cloud.google.com/apis/library/drive.googleapis.com) and click **Enable**

3. **Configure branding** — Go to [console.cloud.google.com/auth/branding](https://console.cloud.google.com/auth/branding), set app name (e.g. "shync") and your email, click **Save**

4. **Set audience** — Go to [console.cloud.google.com/auth/audience](https://console.cloud.google.com/auth/audience), select **External**, add your Google email under **Test users**, click **Save**

5. **Create OAuth client** — Go to [console.cloud.google.com/auth/clients](https://console.cloud.google.com/auth/clients), click **+ Create Client**, select **Desktop app**, click **Create**, then **Download JSON**

6. **Copy credentials file**:
   ```sh
   mkdir -p $HOME/.config/shync
   cp ~/Downloads/client_secret_*.json $HOME/.config/shync/credentials.json
   ```

> **Note:** When authorizing, you may see "Google hasn't verified this app". This is normal — click **Advanced** > **Go to shync (unsafe)** to continue.

### Usage

```sh
# 1. Set up your storage backend
shync init

# 2. Upload a config file
shync push ~/.zshrc

# 3. On another machine, pull it down
shync pull .zshrc

# 4. See what's tracked
shync list
```

## 🗂️ Commands

| Command | Description |
|---------|-------------|
| `shync init` | Interactive setup — pick a backend, configure credentials, and authenticate |
| `shync auth` | Re-authenticate or switch accounts |
| `shync add` | Pick files to track from supported file patterns |
| `shync push [file]...` | Upload one or more files to remote storage (interactive multi-select if no args) |
| `shync pull [file]...` | Download one or more files from remote storage (interactive multi-select if no args) |
| `shync list` | List all tracked files with remote status |
| `shync check` | Check sync status of all tracked files by comparing content |
| `shync diff [file]` | Show side-by-side differences between local and remote versions |
| `shync view [file]` | View a tracked file in read-only vim (local or remote) |
| `shync remove` | Remove tracked files from config |
| `shync backend` | Change the active storage backend (setup wizard) |
| `shync backend list` | List all backends — active and backup — with credential status |
| `shync backend switch` | Promote a configured backend to active; demote old active to backup |
| `shync supported list` | List supported file patterns |
| `shync supported add <pattern>` | Add a filename pattern (e.g. `.vimrc`, `config.toml`) |
| `shync supported remove` | Remove a supported file pattern |
| `shync config get\|set\|list\|remove` | View or modify configuration |
| `shync clean` | Remove stale backups |
| `shync update` | Self-update to the latest release |
| `shync uninstall` | Remove shync and all config data |
| `shync version` | Print version, commit, and build info |

### 📌 Snap commands

| Command | Description |
|---------|-------------|
| `shync snap` | Open the snap picker (shortcut for `snap list`) |
| `shync snap list` | List saved snaps with live regex search; paste selected command to shell prompt |
| `shync snap add` | Save a new snippet — optionally pick from 10 recent shell history entries |
| `shync snap remove` | Multi-select snaps to delete |
| `shync snap sync` | Merge local snaps with the remote backup (union, local wins on conflicts) |

### 🔁 Backup commands

| Command | Description |
|---------|-------------|
| `shync backup list` | Show configured backup backends and their credential status |
| `shync backup add` | Add a backend as a backup destination (reuse or reconfigure credentials) |
| `shync backup remove` | Remove a backup backend |
| `shync backup sync` | Copy all remote files from the active backend to every backup backend |

### 🖥️ Backend commands

| Command | Description |
|---------|-------------|
| `shync backend list` | Show all backends (active ● and backup ○) with credential status |
| `shync backend switch` | Interactively switch active backend; old active is automatically added to backups |

> **Flag:** `--config <path>` overrides the default config location (`~/.config/shync/config.toml`)

## 📁 Batch Operations

Both `push` and `pull` support **interactive multi-select** when no files are specified:

```sh
# Interactive push — space to select, enter to confirm
shync push

# Interactive pull — space to select, enter to confirm
shync pull

# Command-line batch operations
shync push ~/.zshrc ~/.gitconfig
shync pull config.toml snaps.jsonl
```

When no arguments are provided, you'll enter an interactive picker where you can use:
- **Space** to toggle selection on individual files
- **Enter** to confirm and proceed with the selected files
- **Esc** / **Ctrl-C** to cancel

For batch operations via command-line, diff previews and confirmations are skipped for efficiency.

## 📌 Command Snippets

Save frequently used shell commands as named snippets, synced across all your devices.

Snaps are stored locally at `~/.config/shync/snaps.jsonl` (fast, no network for reads) and backed up to your active remote backend automatically on every change.

```sh
# Open the interactive picker — type to filter by regex, enter to paste to shell prompt
shync snap

# Save a new snippet (optionally pick from your last 10 shell history entries)
shync snap add

# Remove one or more snippets
shync snap remove

# Pull the latest backup from remote and merge with local
shync snap sync
```

The picker starts in search mode — type any regex pattern to filter by name or command, then press **Enter** to paste the selected command directly onto your shell prompt (ready to run or edit).

## 🔁 Backup Backends

Mirror all your remote files to one or more additional backends for redundancy. Rules:
- At most **one instance of each backend type** (one Google Drive, one Synology, one Gist)
- The **active backend** cannot also be a backup backend

```sh
# Add a backend as a backup destination
shync backup add

# See configured backup backends
shync backup list

# Copy all files from active backend → every backup backend
shync backup sync

# Remove a backup backend
shync backup remove
```

`backup add` walks you through credential setup for the chosen backend (or offers to reuse existing credentials if already configured). `backup sync` shows per-file progress and reports any errors.

### Switching active backend

Use `shync backend switch` to promote any configured backend to the active (primary) role. The old active backend is automatically added to the backup list so nothing is lost:

```sh
# See all backends and their roles
shync backend list

# Interactively switch — old active becomes a backup automatically
shync backend switch
```

## 🗄️ Supported Backends

<table>
<tr>
<td align="center" width="33%">

### ☁️ Google Drive

OAuth 2.0 with PKCE \
Files stored in a `shync` folder \
Account picker on every auth

</td>
<td align="center" width="33%">

### 🖥️ Synology NAS

Username / password auth \
2FA with OTP support \
Files stored in user's Synology Drive

</td>
<td align="center" width="33%">

### 🐙 GitHub Gist

Personal Access Token auth \
Files stored in a private gist \
No extra setup — just a GitHub account

</td>
</tr>
</table>

## 📖 Documentation

| Document | Description |
|----------|-------------|
| [Update Strategy](docs/UPDATE_STRATEGY.md) | Versioning, releases, and update methods |

## 🤝 Contributing

Contributions are welcome! Please open an issue or submit a pull request.

<div align="center">

---

Made with ❤️ by [@quangkhaidam93](https://github.com/quangkhaidam93)

</div>
