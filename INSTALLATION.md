# Installation Guide

This project includes a native COM overlay handler, a Go status service, and a Windows GUI installer for selecting the local rclone mount directory.

## Prerequisites

`install.ps1` validates these and prompts when something is missing:

- Windows 10 or Windows 11 (amd64)
- Go 1.26 or later
- PowerShell 5.1 or PowerShell 7+
- Visual Studio 2022 Build Tools with the C++ desktop workload and a Windows 10/11 SDK
- rclone on PATH or in a common install location (installer can run `winget install Rclone.Rclone` or open the download page)
- overlay icons under `assets\` (generated automatically with Python if missing)

rclone should already mount the remote to a local drive or directory before you select that path in the installer.

## Build steps

From the repository root:

```powershell
go test ./...
go build ./...
```

To build the main executables into `bin` manually:

```powershell
New-Item -ItemType Directory -Force .\bin | Out-Null
go build -o .\bin\rclone-explorer-status.exe .\cmd\service
go build -ldflags '-H=windowsgui' -o .\bin\rclone-explorer-status-monitor.exe .\cmd\service
go build -ldflags '-H=windowsgui' -o .\bin\rclone-explorer-status-installer.exe .\cmd\installer
go build -ldflags '-H=windowsgui' -o .\bin\rclone-explorer-status-shell.exe .\cmd\shellhelper
go build -ldflags '-H=windowsgui' -o .\bin\rclone-explorer-status-app.exe .\cmd\app
Copy-Item .\bin\rclone-explorer-status.exe .\bin\rcs.exe -Force
```

## Install the application

Run the installer from the repository root:

```powershell
powershell -ExecutionPolicy Bypass -File .\install.ps1
```

The script will:

1. validate Windows / architecture / Go / VS C++ tools / rclone / icons (prompting to install or open download pages when needed)
2. ask whether to add the short **`rcs`** command to your user PATH
3. open the folder picker in your current user session before requesting UAC, so mapped drives such as `S:\` remain visible
4. request administrator permission for machine-wide COM registration and pass the selected mount path to the elevated process
5. create the application directory
6. build the Go CLI (`rclone-explorer-status.exe` + `rcs`), hidden monitor, shell helper, Start Menu app, GUI installer, and x64 native COM DLL
7. install overlay icons
8. save the selected folder and rclone path in `%APPDATA%\RcloneExplorerStatus\config.json`
9. register the three prioritized overlay COM classes and notify Explorer
10. register Explorer right-click Sync / Audit verbs and Start Menu shortcuts
11. optionally add the install `bin` folder to your user PATH
12. start the monitor in the current user session and register it to start at sign-in
13. write a detailed install log

Select the mounted S3 drive or directory, for example `S:\`. Do not select the application install directory. The mount must already exist and be accessible in the current user session when selected. The elevated stage uses the saved path and does not reopen the folder picker.

After installation:

- restart Windows Explorer (Task Manager > Windows Explorer > Restart) so it reloads shell handlers
- open a **new** terminal (PowerShell, cmd, or Git Bash) so `rcs` is on PATH

```bash
rcs doctor
rcs remaining
```

## Default install locations

- app files: `%ProgramFiles%\RcloneExplorerStatus`
- config: `%APPDATA%\RcloneExplorerStatus\config.json`
- state: `%LOCALAPPDATA%\RcloneExplorerStatus\state.json`
- logs: `%LOCALAPPDATA%\RcloneExplorerStatus\logs\`

## Show a test overlay

Select an existing file inside the configured mount and set a state from an elevated or normal PowerShell prompt:

```powershell
& "$env:ProgramFiles\RcloneExplorerStatus\bin\rclone-explorer-status.exe" set "S:\path\to\file.txt" synced
```

Supported states are `synced`, `syncing`, and `error`. The path must exist and be inside the selected mount. The service writes the record and sends an Explorer item refresh notification.

## Run the service

After installation, run the service binary from the install location:

```powershell
& "$env:LOCALAPPDATA\RcloneExplorerStatus\bin\rclone-explorer-status.exe"
```

or, if installed under Program Files:

```powershell
& "$env:ProgramFiles\RcloneExplorerStatus\bin\rclone-explorer-status.exe"
```

The installer starts the hidden monitor automatically and configures it to start at the current user's next sign-in. The CLI service executable is also available for manual commands and foreground diagnostics.

## Enable automatic rclone status updates

The monitor requires an RC-enabled rclone mount. Stop the currently running mount and restart it with RC bound only to loopback. For the default config in this project:

```powershell
rclone mount s3:local-sync-folder-reclone S:\ --vfs-cache-mode full --links --rc --rc-addr 127.0.0.1:5572 --rc-no-auth
```

Do not bind rclone RC to a LAN or public interface. RC can control the rclone process; `--rc-no-auth` is appropriate only while bound to `127.0.0.1`. The monitor polls rclone's `core/stats`, `vfs/queue`, and `core/transferred` endpoints for active, queued, successful, and failed transfers.

## Uninstall

Run `uninstall.ps1` from an elevated PowerShell session if the app was installed under Program Files:

```powershell
powershell -ExecutionPolicy Bypass -File .\uninstall.ps1
```

The uninstaller unregisters the COM handlers and removes installed binaries. It preserves the selected mount configuration, state database, and logs under the user profile.

If Explorer has `RcloneOverlay.dll` loaded, the uninstaller unregisters the COM handler, schedules the DLL for deletion at reboot, and removes the other files it can. Reboot Windows, then run `uninstall.ps1` again if the empty installation directories remain. User config, state, and logs are preserved.

## Verify installation

Check the log file:

```powershell
Get-Content "$env:LOCALAPPDATA\RcloneExplorerStatus\logs\install.log" -Tail 50
```

Follow new installer and UAC child-process entries in the invoking terminal; the same timestamped entries are appended to `install.log`. For service/app logs, run:

```powershell
& "$env:ProgramFiles\RcloneExplorerStatus\bin\rclone-explorer-status.exe" logs --follow
```

Press Ctrl+C to stop following. Both installer and uninstaller record caught errors and the failing script location in their respective log files.

Check the loaded mount configuration:

```powershell
$env:LOCALAPPDATA\RcloneExplorerStatus\bin\rclone-explorer-status.exe status
```

## Troubleshooting

### PowerShell says the installer is not for Windows

Use Windows PowerShell or PowerShell 7 on a Windows machine. The installer checks the runtime environment before creating directories.

### Visual Studio C++ Build Tools are missing

The installer distinguishes an installed Build Tools product from the required C++ compiler workload. If Build Tools is installed but the C++ component is missing, run this from elevated PowerShell to modify that installation:

```powershell
& "${env:ProgramFiles(x86)}\Microsoft Visual Studio\Installer\setup.exe" modify --installPath "${env:ProgramFiles(x86)}\Microsoft Visual Studio\2022\BuildTools" --add Microsoft.VisualStudio.Workload.VCTools --includeRecommended --passive --norestart
```

`Microsoft.VisualStudio.Workload.NativeDesktop` is not a valid workload for this Build Tools installation and can be ignored while setup still exits with code 0. Wait for the installer UI to finish, then verify that the x64 compiler component is installed:

```powershell
& "${env:ProgramFiles(x86)}\Microsoft Visual Studio\Installer\vswhere.exe" -latest -products "*" -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
```

That command should print the Build Tools installation path. Then rerun `install.ps1`. The COM overlay must be built as a native x64 DLL because Explorer loads it in-process.

If Visual Studio Installer reports `0x80070003` while updating a channel manifest under `%LOCALAPPDATA%\Temp`, close other Visual Studio Installer windows and run the modify command from elevated PowerShell with a stable, writable temp directory:

```powershell
$stableTemp = Join-Path $env:LOCALAPPDATA 'Temp'
New-Item -ItemType Directory -Force -Path $stableTemp | Out-Null
$env:TEMP = $stableTemp
$env:TMP = $stableTemp
$probe = Join-Path $stableTemp 'vs-temp-write-test.tmp'
Set-Content -Path $probe -Value 'ok'
Remove-Item -Path $probe -Force
& "${env:ProgramFiles(x86)}\Microsoft Visual Studio\Installer\setup.exe" modify --installPath "${env:ProgramFiles(x86)}\Microsoft Visual Studio\2022\BuildTools" --add Microsoft.VisualStudio.Workload.VCTools --includeRecommended --passive --norestart
```

Keep the PowerShell window open and allow setup to complete. If it fails again, check whether a temp cleaner, antivirus policy, or permissions policy is removing folders under `%LOCALAPPDATA%\Temp` during setup.

### The build fails

Run:

```powershell
go version
go env
```

and verify the Go toolchain is installed correctly.

## Notes

The COM integration displays only file states present in the status store. If RC is unreachable, `doctor` and the service log report that automatic updates are paused; manually setting status with `set` remains available. Windows supports only a limited number of overlay handlers, so competing sync clients may prevent some icons from loading. This installer prefixes the Rclone handlers to improve their chance of being selected, but Explorer must be restarted after changing registrations.
