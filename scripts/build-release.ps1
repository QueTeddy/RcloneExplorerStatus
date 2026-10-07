$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$OutDir = Join-Path $RepoRoot 'dist\RcloneExplorerStatus'
$BinDir = Join-Path $OutDir 'bin'

New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
Push-Location $RepoRoot
try {
    if (-not (Test-Path 'assets\synced.ico')) {
        python .\scripts\generate_icons.py
    }

    go build -o (Join-Path $BinDir 'rclone-explorer-status.exe') .\cmd\service
    go build -ldflags '-H=windowsgui' -o (Join-Path $BinDir 'rclone-explorer-status-monitor.exe') .\cmd\service
    go build -ldflags '-H=windowsgui' -o (Join-Path $BinDir 'rclone-explorer-status-installer.exe') .\cmd\installer
    go build -ldflags '-H=windowsgui' -o (Join-Path $BinDir 'rclone-explorer-status-shell.exe') .\cmd\shellhelper
    go build -ldflags '-H=windowsgui' -o (Join-Path $BinDir 'rclone-explorer-status-app.exe') .\cmd\app

    New-Item -ItemType Directory -Force -Path (Join-Path $OutDir 'scripts') | Out-Null
    Copy-Item .\install.ps1, .\uninstall.ps1, .\README.md, .\LICENSE, .\INSTALLATION.md -Destination $OutDir -Force
    Copy-Item .\scripts\install-prereqs.ps1, .\scripts\generate_icons.py -Destination (Join-Path $OutDir 'scripts') -Force
    Copy-Item .\assets -Destination $OutDir -Recurse -Force

    Write-Host "Release folder ready: $OutDir"
    Write-Host 'Run install.ps1 from that folder (still needs Go + VS Build Tools to build the native overlay), or use the repo install.ps1.'
}
finally {
    Pop-Location
}
