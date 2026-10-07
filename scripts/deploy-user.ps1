# Non-elevated deploy: copies into LOCALAPPDATA if Program Files is locked,
# registers HKCU context menus and Start Menu shortcuts.
$ErrorActionPreference = 'Stop'

$repo = 'c:\projects\work\queteddy\explorer_status'
$pf = 'C:\Program Files\RcloneExplorerStatus'
$la = Join-Path $env:LOCALAPPDATA 'RcloneExplorerStatus'
$built = Join-Path $repo 'bin'

function Write-InstallLog {
    param([string]$Level, [string]$Message)
    Write-Host "[$Level] $Message"
}

. (Join-Path $repo 'scripts\install-prereqs.ps1')

$installRoot = $pf
$bin = Join-Path $installRoot 'bin'
$assets = Join-Path $installRoot 'assets'
$canWritePF = $false
try {
    New-Item -ItemType Directory -Force -Path $bin, $assets | Out-Null
    $probe = Join-Path $bin 'write-test.tmp'
    Set-Content -Path $probe -Value 'ok'
    Remove-Item $probe -Force
    $canWritePF = $true
} catch {
    $installRoot = $la
    $bin = Join-Path $installRoot 'bin'
    $assets = Join-Path $installRoot 'assets'
    New-Item -ItemType Directory -Force -Path $bin, $assets | Out-Null
    Write-Host "Program Files not writable; deploying to $installRoot"
}

Get-Process rclone-explorer-status* -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 1

$names = @(
    'rclone-explorer-status.exe',
    'rclone-explorer-status-monitor.exe',
    'rclone-explorer-status-shell.exe',
    'rclone-explorer-status-app.exe'
)
foreach ($name in $names) {
    Copy-Item -Path (Join-Path $built $name) -Destination $bin -Force
}
Copy-Item -Path (Join-Path $repo 'assets\*') -Destination $assets -Force -Exclude '_preview.png'

$shell = Join-Path $bin 'rclone-explorer-status-shell.exe'
$app = Join-Path $bin 'rclone-explorer-status-app.exe'
$cli = Join-Path $bin 'rclone-explorer-status.exe'
Register-ExplorerContextMenus -ShellHelperPath $shell -IconPath (Join-Path $assets 'synced.ico')
Install-StartMenuShortcuts -AppExe $app -CliExe $cli -InstallRoot $installRoot
$rcs = Install-RcsShim -BinDir $bin -CliExe $cli
$configPath = Join-Path $env:APPDATA 'RcloneExplorerStatus\config.json'
Save-InstallBinPath -ConfigPath $configPath -BinDir $bin
Add-BinDirToUserPath -BinDir $bin

Start-Process -FilePath (Join-Path $bin 'rclone-explorer-status-monitor.exe') -WindowStyle Hidden
Write-Host "Deployed to $installRoot (ProgramFilesWritable=$canWritePF)"
Write-Host "Right-click Sync -> $shell"
Write-Host "Start Menu app -> $app"
Write-Host "Short command: rcs  ($rcs)"
Write-Host "Open a new bash/PowerShell window, then run: rcs remaining"
