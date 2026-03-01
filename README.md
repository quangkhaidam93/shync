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

[Getting Started](#-getting-started) · [Features](#-features) · [Commands](#-commands) · [Backends](#-supported-backends) · [Docs](docs/)

---

</div>

## ✨ Features

| | Feature | Description |
|---|---------|-------------|
| 🔄 | **Two-way sync** | Upload and download configs with a single command |
| 📊 | **Progress bar** | Real-time upload progress with speed and ETA |
| 🔐 | **Secure auth** | OAuth 2.0 + PKCE for Google Drive, 2FA support for Synology |
| 🏠 | **Your storage** | Files live on your Google Drive or Synology NAS — not a third-party server |
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
shync up ~/.zshrc

# 3. On another machine, pull it down
shync down .zshrc

# 4. See what's tracked
shync list
```

## 🗂️ Commands

| Command | Description |
|---------|-------------|
| `shync init` | Interactive setup — pick a backend, configure credentials, and authenticate |
| `shync auth` | Re-authenticate or switch accounts |
| `shync add` | Pick files to track from supported file patterns |
| `shync up [file]` | Upload a file to remote storage |
| `shync down [file]` | Download a file from remote storage |
| `shync list` | List all tracked files with remote status |
| `shync check` | Check sync status of all tracked files by comparing content |
| `shync diff [file]` | Show side-by-side differences between local and remote versions |
| `shync view [file]` | View a tracked file in read-only vim (local or remote) |
| `shync remove` | Remove tracked files from config |
| `shync backend` | Change the active storage backend |
| `shync config get\|set\|list\|remove` | View or modify configuration |
| `shync clean` | Remove stale backups |
| `shync update` | Self-update to the latest release |
| `shync uninstall` | Remove shync and all config data |
| `shync version` | Print version, commit, and build info |

> **Flag:** `--config <path>` overrides the default config location (`~/.config/shync/config.toml`)

## 🗄️ Supported Backends

<table>
<tr>
<td align="center" width="50%">

### ☁️ Google Drive

OAuth 2.0 with PKCE \
Files stored in a `shync` folder \
Account picker on every auth

</td>
<td align="center" width="50%">

### 🖥️ Synology NAS

Username / password auth \
2FA with OTP support \
Files stored in user's Synology Drive

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
