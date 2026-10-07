$ErrorActionPreference = 'Stop'

$repo = 'c:\projects\work\queteddy\explorer_status'
$installRoot = 'C:\Program Files\RcloneExplorerStatus'
$bin = Join-Path $installRoot 'bin'
$assets = Join-Path $installRoot 'assets'

New-Item -ItemType Directory -Force -Path $bin, $assets | Out-Null

Write-Host 'Stopping running Rclone Explorer Status processes...'
Get-Process rclone-explorer-status* -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 1

$built = Join-Path $repo 'bin'
$names = @(
    'rclone-explorer-status.exe',
    'rclone-explorer-status-monitor.exe',
    'rclone-explorer-status-shell.exe',
    'rclone-explorer-status-app.exe'
)
foreach ($name in $names) {
    $src = Join-Path $built $name
    if (Test-Path $src) {
        Copy-Item -Path $src -Destination $bin -Force
        Write-Host "copied $name"
    }
}
Copy-Item -Path (Join-Path $repo 'assets\*') -Destination $assets -Force -Exclude '_preview.png'
Write-Host "Deployed binaries and icons to $installRoot"

. (Join-Path $repo 'scripts\install-prereqs.ps1')
# Provide a logger stub when deploying outside install.ps1
if (-not (Get-Command Write-InstallLog -ErrorAction SilentlyContinue)) {
    function Write-InstallLog {
        param([string]$Level, [string]$Message)
        Write-Host "[$Level] $Message"
    }
}
Register-ExplorerContextMenus -ShellHelperPath (Join-Path $bin 'rclone-explorer-status-shell.exe') -IconPath (Join-Path $assets 'synced.ico')
Install-StartMenuShortcuts -AppExe (Join-Path $bin 'rclone-explorer-status-app.exe') -CliExe (Join-Path $bin 'rclone-explorer-status.exe') -InstallRoot $installRoot

$dll = Join-Path $bin 'RcloneOverlay.dll'
if (Test-Path $dll) {
    & regsvr32.exe /s $dll
    Write-Host 'Overlay DLL re-registered'
}

Add-Type -Namespace Native -Name Shell -MemberDefinition @'
[DllImport("shell32.dll")] public static extern void SHChangeNotify(int wEventId, uint uFlags, IntPtr dwItem1, IntPtr dwItem2);
'@
[Native.Shell]::SHChangeNotify(0x08000000, 0x0000, [IntPtr]::Zero, [IntPtr]::Zero)
Write-Host 'DONE'
