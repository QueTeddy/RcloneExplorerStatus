$ErrorActionPreference = 'Stop'

$rclone = 'C:\self_installed\rclone\rclone.exe'
if (-not (Test-Path $rclone)) {
    throw "rclone not found at $rclone"
}

$stackName = 'explorer-local'
$downloadUrl = aws cloudformation describe-stacks --region us-east-1 --stack-name $stackName --query "Stacks[0].Outputs[?OutputKey=='DownloadUrl'].OutputValue | [0]" --output text
if (-not $downloadUrl -or $downloadUrl -eq 'None') {
    throw "CloudFront DownloadUrl is not ready. Wait until stack $stackName is CREATE_COMPLETE."
}
if (-not $downloadUrl.EndsWith('/')) {
    $downloadUrl = $downloadUrl + '/'
}

$pullKey = [Environment]::GetEnvironmentVariable('RCLONE_PULL_KEY', 'User')
if (-not $pullKey) { $pullKey = $env:RCLONE_PULL_KEY }
if (-not $pullKey) {
    throw 'Set the user environment variable RCLONE_PULL_KEY to the CloudFront pull secret, then open a new terminal.'
}
$pullKey = $pullKey.Trim()

& $rclone config update s3 download_url $downloadUrl
if ($LASTEXITCODE -ne 0) {
    throw 'rclone config update failed while setting download_url'
}
Write-Host "rclone s3 downloads will use $downloadUrl"

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

$args = @(
    'mount',
    's3:local-sync-folder-reclone',
    'S:',
    '--vfs-cache-mode', 'full',
    '--vfs-cache-max-size', '20G',
    '--vfs-read-chunk-size', '128M',
    '--s3-chunk-size', '64M',
    '--s3-download-url', $downloadUrl,
    '--header-download', "X-Pull-Key:$pullKey",
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
    Select-Object ProcessId |
    Format-List
