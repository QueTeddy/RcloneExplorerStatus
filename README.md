# Rclone Explorer Sync Status

This repository contains a native Windows Explorer overlay handler, a Go status service, and an installer that lets the user select the local rclone mount directory.

## What is included

- configurable application settings
- file status model and state transitions
- JSON-based state store with atomic writes and corruption handling
- status-to-overlay mapping for synced, syncing, and error
- GUI mount-folder selection during installation
- native 64-bit `IShellIconOverlayIdentifier` COM handlers for synced, syncing, and error
- Explorer refresh notification after a file status changes
- hidden background monitor polling rclone's RC stats, VFS upload queue, and completed transfers
- current-user logon startup registration for the monitor
- installation and uninstallation scripts
- test coverage for normalization, transitions, and persistence

The overlay handler reads exact file-state records from the local status store. The monitor reads rclone's RC endpoints and updates those records while running. Rclone must be restarted with RC enabled as shown below; the installer cannot turn RC on inside an already-running rclone process.

## Status model

The current implementation supports the PRD-required core states:

- `unknown`
- `pending`
- `syncing`
- `synced`
- `error`
- `conflict`
- `offline`
- `cloud_only`

The active Explorer overlay proof is built around the first-release minimum:

- `synced`
- `syncing`
- `error`

## Usage

### 1. Build the project

```bash
go test ./...
go build ./...
```

### 2. Install on Windows

From the repo root:

```powershell
powershell -ExecutionPolicy Bypass -File .\install.ps1
```

The installer opens the mount picker before requesting administrator permission so per-user mapped drives such as `S:\` are visible. It then builds the binaries and COM DLL, registers the shell handler, and starts the hidden monitor. Restart Windows Explorer after installation to reload shell handlers.

To open the folder picker directly after building:

```powershell
go run .\cmd\installer
```

### 3. Run the service

```powershell
& "$env:ProgramFiles\RcloneExplorerStatus\bin\rclone-explorer-status.exe"
```

If installation used the per-user fallback, run `%LOCALAPPDATA%\RcloneExplorerStatus\bin\rclone-explorer-status.exe` instead.

The installer also starts a hidden monitor immediately and registers it to run at your next sign-in.

### 4. Enable automatic rclone status updates

Stop the currently running rclone mount, then restart it with the loopback-only RC listener. Example for this configured remote and mount:

```powershell
rclone mount s3:local-sync-folder-reclone S:\ --vfs-cache-mode full --links --rc --rc-addr 127.0.0.1:5572 --rc-no-auth
```

Keep RC bound to `127.0.0.1`; the RC API can control rclone and must not be exposed to the network. `--rc-no-auth` is used here only with that loopback bind. The monitor polls VFS queue and transfer endpoints and updates file states automatically.

### 5. Check status

```powershell
"$env:LOCALAPPDATA\RcloneExplorerStatus\bin\rclone-explorer-status.exe" status
```

### 6. Run the doctor check

```powershell
"$env:LOCALAPPDATA\RcloneExplorerStatus\bin\rclone-explorer-status.exe" doctor
```

### 7. View logs

```powershell
& "$env:ProgramFiles\RcloneExplorerStatus\bin\rclone-explorer-status.exe" logs
& "$env:ProgramFiles\RcloneExplorerStatus\bin\rclone-explorer-status.exe" logs --follow
Get-Content "$env:LOCALAPPDATA\RcloneExplorerStatus\logs\install.log" -Tail 50
```

`logs --follow` prints the last 50 application entries, then streams new entries until Ctrl+C. The service also writes each entry immediately to `service.log`; the PowerShell installer streams elevated progress into `install.log` and the invoking terminal.

### 8. Show a test overlay

Choose an existing file under the selected mount and set its state:

```powershell
& "$env:ProgramFiles\RcloneExplorerStatus\bin\rclone-explorer-status.exe" set "S:\path\to\file.txt" synced
```

Use `syncing` or `error` for the other overlays. The command saves the status and notifies Explorer to refresh that file. Automatic updates still require rclone RC to be enabled.

## Installation details

See [INSTALLATION.md](INSTALLATION.md) for the full Windows installation and verification guide.

## Current build

```bash
go test ./...
go build ./...
```

## Logging

The application writes structured logs to `%LOCALAPPDATA%\RcloneExplorerStatus\logs\service.log` and the installer writes `%LOCALAPPDATA%\RcloneExplorerStatus\logs\install.log`.

## Requirements and limitations

Automatic updates require the mounted rclone process to expose RC on `127.0.0.1:5572`; this app cannot enable RC inside an already-running mount. The hidden monitor polls the VFS upload queue and transfer results, and starts at future user sign-ins. Restart the rclone mount with loopback RC enabled and restart Windows Explorer after installation. Other sync applications compete for Windows' limited overlay-handler slots.
