# Rclone Explorer Sync Status

Native Windows Explorer overlay icons and a background status service for files managed by **rclone**. Explorer stays the UI; rclone stays the sync engine. This app only presents state.

## What is included

- configurable mount / rclone / RC settings
- JSON status store with atomic writes and corruption recovery
- Explorer overlays for **synced**, **syncing**, and **error**
- hidden monitor that polls rclone RC (`core/stats`, `vfs/queue`, `core/transferred`)
- inventory **audit** / **remaining** / **resync** via rclone (`lsjson` / `copyto`) — never a direct S3 client
- installer that validates prerequisites (Windows, Go, VS C++ tools, rclone, icons)
- optional short CLI on PATH: **`rcs`**
- Start Menu app, Explorer right-click Sync / Audit
- install / uninstall scripts

## Quick start

### Install

```powershell
powershell -ExecutionPolicy Bypass -File .\install.ps1
```

The installer asks for:

1. **rclone** (offers winget install or opens the download page if missing)
2. whether to add **`rcs`** to your user PATH
3. the local rclone mount folder (for example `S:\`)

It then builds binaries + the overlay DLL, registers COM handlers, Start Menu shortcuts, and context menus, and starts the background monitor.

Restart Windows Explorer after install. Open a **new** terminal so PATH picks up `rcs`.

### Enable automatic overlays

Remount rclone with loopback RC (required for live status):

```powershell
rclone mount s3:local-sync-folder-reclone S:\ --vfs-cache-mode full --links --rc --rc-addr 127.0.0.1:5572 --rc-no-auth
```

Keep RC on `127.0.0.1` only.

### Everyday commands (`rcs`)

```bash
rcs status
rcs doctor
rcs remaining
rcs remaining "S:/courses"
rcs remaining "S:/courses" --items
rcs audit "S:/courses"
rcs resync "S:/courses"
rcs resync "S:/path/to/file.mp4"
rcs logs --follow
rcs set "S:/path/to/file.txt" synced
```

| Command | Purpose |
|---------|---------|
| `remaining` | Folders + items still missing on the remote (add `--items` to list files) |
| `audit` | Compare mount vs remote, update Explorer overlays |
| `resync` | Upload missing local files (or one file) via rclone |
| `doctor` | Health checks (mount, state, rclone, RC) |
| `set` | Manually set overlay state for a file |

### Explorer integration

- Right-click a **file** under the mount → **Sync with Rclone Explorer Status**
- Right-click a **folder** → **Audit folder with Rclone Explorer Status**
- Start Menu → **Rclone Explorer Status** (status panel)

## Status model

Supported states: `unknown`, `pending`, `syncing`, `synced`, `error`, `conflict`, `offline`, `cloud_only`.

First-release overlays:

| State | Overlay |
|-------|---------|
| synced | green check |
| syncing / pending | blue arrows |
| error / conflict | red X |

## Build from source

```bash
go test ./...
go build ./...
```

Manual binaries:

```powershell
New-Item -ItemType Directory -Force .\bin | Out-Null
go build -o .\bin\rclone-explorer-status.exe .\cmd\service
go build -ldflags '-H=windowsgui' -o .\bin\rclone-explorer-status-monitor.exe .\cmd\service
go build -ldflags '-H=windowsgui' -o .\bin\rclone-explorer-status-shell.exe .\cmd\shellhelper
go build -ldflags '-H=windowsgui' -o .\bin\rclone-explorer-status-app.exe .\cmd\app
python .\scripts\generate_icons.py
```

## Install locations

| What | Path |
|------|------|
| App (machine) | `%ProgramFiles%\RcloneExplorerStatus` |
| App (user fallback) | `%LOCALAPPDATA%\RcloneExplorerStatus` |
| Config | `%APPDATA%\RcloneExplorerStatus\config.json` |
| State | `%LOCALAPPDATA%\RcloneExplorerStatus\state.json` |
| Logs | `%LOCALAPPDATA%\RcloneExplorerStatus\logs\` |

## Uninstall

```powershell
powershell -ExecutionPolicy Bypass -File .\uninstall.ps1
```

Removes COM handlers, binaries, context menus, Start Menu entries, and the `rcs` PATH entry. Config, state, and logs are preserved.

## Architecture rule

```text
Explorer ← overlays ← Go status store ← rclone RC / lsjson / copyto ← S3
```

The Go app never talks to S3 directly and never stores AWS credentials.

## More detail

See [INNER-WORKINGS.md](INNER-WORKINGS.md) for the blog-style walkthrough: the mount, the VFS cache, the status poll, the overlay DLL, and the limits.

See [INSTALLATION.md](INSTALLATION.md) for prerequisites, Build Tools troubleshooting, and verification steps.

## Requirements and limitations

- Windows 10/11 amd64
- Automatic live overlays need rclone RC on `127.0.0.1:5572`
- Windows has a limited number of overlay handler slots (other sync clients may compete)
- Overlay icons are corner badges that scale with Explorer icon size
