$ErrorActionPreference = 'Stop'

$root = 'S:\RcloneOverlayTest'
$null = New-Item -ItemType Directory -Path $root -Force

$files = @(
    @{ Name = 'synced.txt'; State = 'synced' },
    @{ Name = 'syncing.txt'; State = 'syncing' },
    @{ Name = 'error.txt'; State = 'error' }
)

foreach ($file in $files) {
    $path = Join-Path $root $file.Name
    Set-Content -Path $path -Value "status=$($file.State)"
    Write-Host "Prepared $path -> $($file.State)"
}

Write-Host 'Overlay verification checklist:'
Write-Host '  - synced.txt should show green overlay'
Write-Host '  - syncing.txt should show blue overlay'
Write-Host '  - error.txt should show red overlay'
