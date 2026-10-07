$ErrorActionPreference = 'Stop'

$existing = Get-CimInstance Win32_Process -Filter "Name='rclone.exe'" -ErrorAction SilentlyContinue
foreach ($proc in $existing) {
    Write-Host "Stopping rclone PID $($proc.ProcessId)..."
    Stop-Process -Id $proc.ProcessId -Force -ErrorAction SilentlyContinue
}

Start-Sleep -Seconds 2
for ($i = 0; $i -lt 20; $i++) {
    if (-not (Test-Path 'S:\')) { break }
    Start-Sleep -Seconds 1
}

$rclone = 'C:\self_installed\rclone\rclone.exe'
if (-not (Test-Path $rclone)) {
    throw "rclone not found at $rclone"
}

$args = @(
    'mount',
    's3:local-sync-folder-reclone',
    'S:',
    '--vfs-cache-mode', 'full',
    '--links',
    '--rc',
    '--rc-addr', '127.0.0.1:5572',
    '--rc-no-auth'
)

Write-Host 'Starting rclone with RC on 127.0.0.1:5572...'
$started = Start-Process -FilePath $rclone -ArgumentList $args -WindowStyle Hidden -PassThru
Write-Host "Started rclone PID $($started.Id)"

$ok = $false
for ($i = 0; $i -lt 20; $i++) {
    Start-Sleep -Seconds 1
    try {
        $response = Invoke-WebRequest -Uri 'http://127.0.0.1:5572/core/version' -Method POST -UseBasicParsing -TimeoutSec 2
        Write-Host "RC OK status=$($response.StatusCode)"
        Write-Host $response.Content
        $ok = $true
        break
    } catch {
        Write-Host "Waiting for RC... $($_.Exception.Message)"
    }
}

if (-not $ok) {
    throw 'rclone RC did not become reachable on 127.0.0.1:5572'
}

Write-Host "S: accessible=$(Test-Path 'S:\')"
Get-CimInstance Win32_Process -Filter "Name='rclone.exe'" |
    Select-Object ProcessId, CommandLine |
    Format-List
