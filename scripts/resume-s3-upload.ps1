# Continues one multipart upload. A failed part is left on S3.
# Run this same script again to send only the missing parts.
param(
    [string]$File = "$env:LOCALAPPDATA\rclone\vfs\s3\local-sync-folder-reclone\games\Call of Duty Modern Warfare Campaign [DODI Repack]\data1.doi",
    [string]$Bucket = 'local-sync-folder-reclone',
    [string]$Key = 'games/Call of Duty Modern Warfare Campaign [DODI Repack]/data1.doi',
    [string]$Region = 'us-east-1',
    [long]$PartSize = 67108864,
    [string]$StatePath = "$env:LOCALAPPDATA\RcloneExplorerStatus\s3-resume-data1.json",
    [switch]$Abort
)

$ErrorActionPreference = 'Stop'
$logPath = Join-Path $env:LOCALAPPDATA 'RcloneExplorerStatus\logs\s3-resume.log'

function Format-ProcessArg {
    param([string]$Value)
    if ($Value -match '[\s"]') {
        return '"' + ($Value -replace '"', '\"') + '"'
    }
    return $Value
}

function Invoke-Aws {
    param([string[]]$AwsArgs)
    $aws = (Get-Command aws).Source
    $argumentList = @($AwsArgs) + @('--output', 'json', '--no-cli-pager')
    $quoted = ($argumentList | ForEach-Object { Format-ProcessArg $_ }) -join ' '
    $process = New-Object System.Diagnostics.Process
    $process.StartInfo = New-Object System.Diagnostics.ProcessStartInfo
    $process.StartInfo.FileName = $aws
    $process.StartInfo.Arguments = $quoted
    $process.StartInfo.UseShellExecute = $false
    $process.StartInfo.RedirectStandardOutput = $true
    $process.StartInfo.RedirectStandardError = $true
    $process.StartInfo.CreateNoWindow = $true
    [void]$process.Start()
    $stdout = $process.StandardOutput.ReadToEnd()
    $stderr = $process.StandardError.ReadToEnd()
    $process.WaitForExit()
    if ($stderr) { Add-Content -LiteralPath $logPath -Value $stderr }
    if ($process.ExitCode -ne 0) {
        throw "aws $($AwsArgs[0]) $($AwsArgs[1]) failed. $stderr"
    }
    if (-not $stdout.Trim()) { return $null }
    return $stdout.Trim() | ConvertFrom-Json
}

function Get-UploadedParts {
    param([string]$UploadId)
    $parts = @()
    $marker = 0
    do {
        $page = Invoke-Aws @(
            's3api', 'list-parts',
            '--bucket', $Bucket,
            '--key', $Key,
            '--region', $Region,
            '--upload-id', $UploadId,
            '--part-number-marker', "$marker",
            '--max-parts', '1000'
        )
        if ($page.Parts) {
            $batch = @($page.Parts)
            $parts += $batch
            $marker = [int]$batch[-1].PartNumber
        }
    } while ($page.IsTruncated)
    return ,$parts
}

function Write-PartFile {
    param(
        [System.IO.FileStream]$Stream,
        [long]$Offset,
        [int]$Count,
        [string]$Destination
    )
    [void]$Stream.Seek($Offset, [System.IO.SeekOrigin]::Begin)
    $buffer = New-Object byte[] $Count
    $got = 0
    while ($got -lt $Count) {
        $read = $Stream.Read($buffer, $got, $Count - $got)
        if ($read -le 0) { throw "Short read at offset $Offset" }
        $got += $read
    }
    [System.IO.File]::WriteAllBytes($Destination, $buffer)
}

if (-not (Get-Command aws -ErrorAction SilentlyContinue)) {
    throw 'aws is not on PATH'
}
New-Item -ItemType Directory -Force -Path (Split-Path $StatePath) | Out-Null
New-Item -ItemType Directory -Force -Path (Split-Path $logPath) | Out-Null

if (-not (Test-Path -LiteralPath $File)) {
    throw "Source file not found: $File"
}
$length = (Get-Item -LiteralPath $File).Length
if ($length -le 0) { throw "Source file is empty: $File" }

$state = $null
if (Test-Path -LiteralPath $StatePath) {
    $state = Get-Content -LiteralPath $StatePath -Raw | ConvertFrom-Json
}

if ($Abort) {
    if ($state -and $state.uploadId) {
        Invoke-Aws @(
            's3api', 'abort-multipart-upload',
            '--bucket', $Bucket,
            '--key', $Key,
            '--region', $Region,
            '--upload-id', $state.uploadId
        ) | Out-Null
        Write-Host "Aborted upload $($state.uploadId)"
    }
    if (Test-Path -LiteralPath $StatePath) { Remove-Item -LiteralPath $StatePath -Force }
    Write-Host 'Resume state removed. The next run starts a new upload.'
    exit 0
}

if ($state) {
    if ($state.fileLength -ne $length) {
        throw "File size changed from $($state.fileLength) to $length. Refusing to continue this upload."
    }
    if ([long]$state.partSize -ne $PartSize) {
        Write-Host "Using the original part size $($state.partSize), not $PartSize."
        $PartSize = [long]$state.partSize
    }
} else {
    $created = Invoke-Aws @(
        's3api', 'create-multipart-upload',
        '--bucket', $Bucket,
        '--key', $Key,
        '--region', $Region
    )
    $state = [pscustomobject]@{
        bucket     = $Bucket
        key        = $Key
        file       = $File
        region     = $Region
        partSize   = $PartSize
        uploadId   = $created.UploadId
        fileLength = $length
    }
    $state | ConvertTo-Json | Set-Content -LiteralPath $StatePath -Encoding UTF8
    Write-Host "Started upload $($state.uploadId)"
    Write-Host "State file: $StatePath"
}

$partCount = [int][Math]::Ceiling($length / [double]$PartSize)
$existing = Get-UploadedParts -UploadId $state.uploadId
$have = @{}
foreach ($part in $existing) { $have[[int]$part.PartNumber] = $true }
Write-Host ("Already on S3: {0} of {1} parts" -f $have.Count, $partCount)

$temp = Join-Path $env:TEMP 's3-resume-part.bin'
$stream = [System.IO.FileStream]::new(
    $File,
    [System.IO.FileMode]::Open,
    [System.IO.FileAccess]::Read,
    [System.IO.FileShare]::ReadWrite
)
try {
    for ($number = 1; $number -le $partCount; $number++) {
        if ($have.ContainsKey($number)) { continue }
        $offset = [long]($number - 1) * $PartSize
        $count = [int][Math]::Min($PartSize, $length - $offset)
        Write-PartFile -Stream $stream -Offset $offset -Count $count -Destination $temp

        $sent = $false
        for ($try = 1; $try -le 5; $try++) {
            try {
                Invoke-Aws @(
                    's3api', 'upload-part',
                    '--bucket', $Bucket,
                    '--key', $Key,
                    '--region', $Region,
                    '--upload-id', $state.uploadId,
                    '--part-number', "$number",
                    '--body', $temp
                ) | Out-Null
                $sent = $true
                break
            } catch {
                Write-Host "Part $number attempt $try failed. $($_.Exception.Message)"
                if ($try -lt 5) { Start-Sleep -Seconds ([int][Math]::Pow(2, $try)) }
            }
        }
        if (-not $sent) {
            Write-Host "Stopped on part $number of $partCount. Run this script again to continue."
            exit 2
        }
        Write-Host ("Uploaded part {0} of {1}" -f $number, $partCount)
    }
} finally {
    $stream.Dispose()
    if (Test-Path -LiteralPath $temp) { Remove-Item -LiteralPath $temp -Force }
}

$finished = Get-UploadedParts -UploadId $state.uploadId
if ($finished.Count -ne $partCount) {
    Write-Host "S3 has $($finished.Count) parts and this file needs $partCount. Run this script again."
    exit 2
}

$complete = @{
    Parts = @(
        $finished | Sort-Object { [int]$_.PartNumber } | ForEach-Object {
            [pscustomobject]@{ ETag = $_.ETag; PartNumber = [int]$_.PartNumber }
        }
    )
}
$completePath = Join-Path $env:TEMP 's3-resume-complete.json'
$complete | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $completePath -Encoding ASCII
$completeUri = 'file://' + ($completePath -replace '\\', '/')
try {
    Invoke-Aws @(
        's3api', 'complete-multipart-upload',
        '--bucket', $Bucket,
        '--key', $Key,
        '--region', $Region,
        '--upload-id', $state.uploadId,
        '--multipart-upload', $completeUri
    ) | Out-Null
} finally {
    if (Test-Path -LiteralPath $completePath) { Remove-Item -LiteralPath $completePath -Force }
}

$head = Invoke-Aws @(
    's3api', 'head-object',
    '--bucket', $Bucket,
    '--key', $Key,
    '--region', $Region
)
if ([long]$head.ContentLength -ne $length) {
    throw "Completed object is $($head.ContentLength) bytes. The local file is $length bytes."
}
Remove-Item -LiteralPath $StatePath -Force
Write-Host "Upload complete. Remote size $($head.ContentLength) bytes."
exit 0
