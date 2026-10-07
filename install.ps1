param(
    [string]$MountPath,
    [string]$MountPathBase64
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$AppName = 'RcloneExplorerStatus'
$LogRoot = Join-Path $env:LOCALAPPDATA $AppName
$LogFile = Join-Path $LogRoot 'logs\install.log'

function Write-InstallLog {
    param(
        [Parameter(Mandatory = $true)][string]$Level,
        [Parameter(Mandatory = $true)][string]$Message
    )

    $timestamp = (Get-Date).ToUniversalTime().ToString('o')
    $line = "[$timestamp] [$Level] $Message"
    New-Item -ItemType Directory -Path (Split-Path $LogFile -Parent) -Force | Out-Null
    Add-Content -Path $LogFile -Value $line
    Write-Host $line
}

function Read-NewInstallLogLines {
    param([long]$Offset)

    $stream = [System.IO.File]::Open($LogFile, [System.IO.FileMode]::Open, [System.IO.FileAccess]::Read, [System.IO.FileShare]::ReadWrite)
    try {
        $null = $stream.Seek($Offset, [System.IO.SeekOrigin]::Begin)
        $reader = New-Object System.IO.StreamReader($stream)
        try {
            $content = $reader.ReadToEnd()
            $newOffset = $stream.Position
        } finally {
            $reader.Dispose()
        }
    } finally {
        $stream.Dispose()
    }

    foreach ($line in ($content -split "`r?`n")) {
        if ($line) { Write-Host $line }
    }
    return $newOffset
}

function Invoke-LoggedInstallCommand {
    param(
        [Parameter(Mandatory = $true)][string]$Executable,
        [Parameter(Mandatory = $true)][string[]]$Arguments,
        [Parameter(Mandatory = $true)][string]$Description
    )

    Write-InstallLog -Level INFO -Message "Starting $Description."
    & $Executable @Arguments 2>&1 | ForEach-Object {
        if ($null -ne $_) {
            $outputLine = $_.ToString()
            if (-not [string]::IsNullOrWhiteSpace($outputLine)) {
                Write-InstallLog -Level DEBUG -Message $outputLine
            }
        }
    }
    $commandExitCode = $LASTEXITCODE
    Write-InstallLog -Level INFO -Message "$Description exited with code $commandExitCode."
    if ($commandExitCode -ne 0) {
        throw "$Description failed with exit code $commandExitCode."
    }
}

function Select-RcloneMountFolder {
    Add-Type -AssemblyName System.Windows.Forms
    $dialog = New-Object System.Windows.Forms.FolderBrowserDialog
    $dialog.Description = 'Select the local drive or folder where rclone mounted the S3 remote.'
    $dialog.RootFolder = [System.Environment+SpecialFolder]::MyComputer
    $dialog.ShowNewFolderButton = $false

    $suggestedDrive = Get-PSDrive -PSProvider FileSystem |
        Where-Object { $_.Name -ne 'C' -and (Test-Path -LiteralPath $_.Root) } |
        Select-Object -First 1
    if ($suggestedDrive) {
        $dialog.SelectedPath = $suggestedDrive.Root
    }

    try {
        if ($dialog.ShowDialog() -ne [System.Windows.Forms.DialogResult]::OK) {
            throw 'Folder selection was cancelled.'
        }
        return $dialog.SelectedPath
    } finally {
        $dialog.Dispose()
    }
}

function Invoke-ElevatedInstaller {
	Write-InstallLog -Level INFO -Message 'Launching elevated installer; live child-process log entries will follow.'
    $offset = (Get-Item -LiteralPath $LogFile).Length
    $encodedMountPath = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($MountPath))
    $arguments = @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', "`"$PSCommandPath`"", '-MountPathBase64', $encodedMountPath)
    try {
        $process = Start-Process -FilePath 'powershell.exe' -Verb RunAs -ArgumentList $arguments -PassThru
    } catch {
        Write-InstallLog -Level ERROR -Message "Administrator approval was cancelled or elevated PowerShell could not start: $($_.Exception.Message)"
        return 1223
    }

    Write-Host "Elevated installer started (PID $($process.Id)); streaming its log output."
    while (-not $process.HasExited) {
        $offset = Read-NewInstallLogLines -Offset $offset
        $null = $process.WaitForExit(300)
        $process.Refresh()
    }
    $offset = Read-NewInstallLogLines -Offset $offset
    Write-InstallLog -Level INFO -Message "Elevated installer exited with code $($process.ExitCode)."
    return $process.ExitCode
}

trap {
    $failure = $_
    try {
        Write-InstallLog -Level ERROR -Message "Unhandled installation failure: $($failure.Exception.Message)"
        Write-InstallLog -Level DEBUG -Message "Failure location: $($failure.InvocationInfo.PositionMessage)"
    } catch {
        Write-Host "Could not write installation failure to log: $($_.Exception.Message)"
    }
    exit 1
}

Write-InstallLog -Level INFO -Message 'Starting installation for RcloneExplorerStatus.'

if (-not [string]::IsNullOrWhiteSpace($MountPathBase64)) {
    try {
        $MountPath = [Text.Encoding]::Unicode.GetString([Convert]::FromBase64String($MountPathBase64))
    } catch {
        throw "The mount path passed to the elevated installer could not be decoded: $($_.Exception.Message)"
    }
}

if ([string]::IsNullOrWhiteSpace($MountPath)) {
    Write-InstallLog -Level INFO -Message 'Opening mount-folder picker before elevation so user-mapped drives remain visible.'
    try {
        $MountPath = Select-RcloneMountFolder
    } catch {
        Write-InstallLog -Level WARN -Message "Mount-folder selection did not complete: $($_.Exception.Message)"
        throw
    }
    if (-not (Test-Path -LiteralPath $MountPath -PathType Container)) {
        throw "Selected mount folder is not accessible in this session: $MountPath"
    }
    $MountPath = [System.IO.Path]::GetFullPath($MountPath)
    Write-InstallLog -Level INFO -Message "Selected rclone mount in current user session: $MountPath"
} else {
    $MountPath = $MountPath.Trim().Trim('"')
    $MountPath = [System.IO.Path]::GetFullPath($MountPath)
    Write-InstallLog -Level INFO -Message "Using mount path selected before elevation: $MountPath"
}

$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '.')).Path
if (-not (Test-Path (Join-Path $RepoRoot 'go.mod'))) {
    throw "Go project root not found at $RepoRoot. Run this installer from the source distribution."
}

$VSWhere = Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio\Installer\vswhere.exe'
if (-not (Test-Path $VSWhere)) {
    throw 'Visual Studio C++ Build Tools were not found. Install Visual Studio 2022 Build Tools with the C++ desktop workload and Windows 10/11 SDK, then rerun install.ps1. See https://visualstudio.microsoft.com/visual-cpp-build-tools/.'
}

Write-InstallLog -Level INFO -Message "Searching for installed Visual Studio instances with $VSWhere"
$VSInstances = @(& $VSWhere -latest -products '*' -property installationPath 2>&1)
$vsWhereExitCode = $LASTEXITCODE
foreach ($instanceLine in $VSInstances) {
    Write-InstallLog -Level DEBUG -Message "Visual Studio instance: $($instanceLine.ToString())"
}
$VSInstall = $VSInstances | Where-Object { $_ -is [string] -and -not [string]::IsNullOrWhiteSpace($_) } | Select-Object -First 1
if ($vsWhereExitCode -ne 0 -or -not $VSInstall -or -not (Test-Path $VSInstall)) {
    Write-InstallLog -Level ERROR -Message "No Visual Studio installation was returned by vswhere (exit code $vsWhereExitCode)."
    throw 'Visual Studio Build Tools were not found. Install Visual Studio 2022 Build Tools, then rerun install.ps1.'
}

Write-InstallLog -Level INFO -Message "Visual Studio instance found at $VSInstall; checking for the C++ compiler component."
$CxxInstances = @(& $VSWhere -latest -products '*' -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath 2>&1)
$cxxWhereExitCode = $LASTEXITCODE
foreach ($componentLine in $CxxInstances) {
    Write-InstallLog -Level DEBUG -Message "C++ toolchain instance: $($componentLine.ToString())"
}
$CxxInstall = $CxxInstances | Where-Object { $_ -is [string] -and -not [string]::IsNullOrWhiteSpace($_) } | Select-Object -First 1
if ($cxxWhereExitCode -ne 0 -or -not $CxxInstall -or -not (Test-Path $CxxInstall)) {
    $SetupExe = Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio\Installer\setup.exe'
    $repairCommand = "& `"$SetupExe`" modify --installPath `"$VSInstall`" --add Microsoft.VisualStudio.Workload.VCTools --includeRecommended --passive --norestart"
    Write-InstallLog -Level ERROR -Message "Visual Studio is installed at $VSInstall, but Microsoft.VisualStudio.Component.VC.Tools.x86.x64 is missing."
    Write-InstallLog -Level INFO -Message "Run this in elevated PowerShell to add the required C++ workload: $repairCommand"
    throw 'Visual Studio Build Tools are installed, but the C++ compiler workload is missing. Add the VCTools workload to the existing Build Tools installation with the command shown above, wait for setup to finish, then rerun install.ps1.'
}

$VSInstall = $CxxInstall
$MSBuildPath = Join-Path $VSInstall 'MSBuild\Current\Bin\MSBuild.exe'
if (-not (Test-Path $MSBuildPath)) {
    throw "MSBuild was not found at $MSBuildPath. Add the C++ desktop workload to Visual Studio Build Tools and rerun install.ps1."
}
$OverlayProject = Join-Path $RepoRoot 'native\overlay\RcloneOverlay.vcxproj'
Write-InstallLog -Level INFO -Message "C++ build toolchain found: $VSInstall"

$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($identity)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-InstallLog -Level INFO -Message 'Requesting administrator permission to register the Explorer COM handler.'
    $elevatedExitCode = Invoke-ElevatedInstaller
    if ($elevatedExitCode -ne 0) {
        Write-InstallLog -Level ERROR -Message "Elevated installation did not complete (exit code $elevatedExitCode)."
    } else {
        $monitorPath = Join-Path $env:ProgramFiles "$AppName\bin\rclone-explorer-status-monitor.exe"
        if (-not (Test-Path $monitorPath)) {
            $monitorPath = Join-Path $env:LOCALAPPDATA "$AppName\bin\rclone-explorer-status-monitor.exe"
        }
        if (Test-Path $monitorPath) {
            Get-CimInstance Win32_Process -Filter "Name='rclone-explorer-status-monitor.exe'" -ErrorAction SilentlyContinue |
                Where-Object { $_.ExecutablePath -eq $monitorPath } |
                ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
            $monitorProcess = Start-Process -FilePath $monitorPath -WindowStyle Hidden -PassThru
            Write-InstallLog -Level INFO -Message "Started user-session status monitor (PID $($monitorProcess.Id)): $monitorPath"
        } else {
            Write-InstallLog -Level WARN -Message "Installed background monitor was not found at $monitorPath; start it manually after installation."
        }
    }
    exit $elevatedExitCode
}

if (-not $env:OS -and -not $IsWindows) {
    $platform = [System.Runtime.InteropServices.RuntimeInformation]::OSDescription
    if ($platform -notmatch 'Windows') {
        throw 'This installer is intended for Windows.'
    }
}

$InstallRoot = Join-Path $env:ProgramFiles $AppName
try {
    New-Item -ItemType Directory -Path $InstallRoot -Force | Out-Null
    $null = [System.IO.File]::OpenWrite((Join-Path $InstallRoot 'install.test')).Close()
    Remove-Item -Path (Join-Path $InstallRoot 'install.test') -Force -ErrorAction SilentlyContinue
    Write-InstallLog -Level INFO -Message "Installing to Program Files: $InstallRoot"
} catch {
    $InstallRoot = Join-Path $env:LOCALAPPDATA $AppName
    Write-InstallLog -Level WARN -Message "Program Files is not writable; falling back to user install path: $InstallRoot"
}

$ConfigDir = Join-Path $env:APPDATA $AppName
$StateDir = Join-Path $env:LOCALAPPDATA $AppName
$BinDir = Join-Path $InstallRoot 'bin'

New-Item -ItemType Directory -Path $ConfigDir -Force | Out-Null
New-Item -ItemType Directory -Path $StateDir -Force | Out-Null
New-Item -ItemType Directory -Path $BinDir -Force | Out-Null

$BinaryPath = Join-Path $BinDir 'rclone-explorer-status.exe'
$MonitorPath = Join-Path $BinDir 'rclone-explorer-status-monitor.exe'
$InstallerPath = Join-Path $BinDir 'rclone-explorer-status-installer.exe'
$OverlayPath = Join-Path $BinDir 'RcloneOverlay.dll'
$AssetsDir = Join-Path $InstallRoot 'assets'

if (-not (Test-Path (Join-Path $RepoRoot 'go.mod'))) {
    throw "Go project root not found at $RepoRoot. Run this installer from the source distribution."
}

Write-InstallLog -Level INFO -Message "Building service and GUI installer from repository root: $RepoRoot"
Push-Location $RepoRoot
try {
    Invoke-LoggedInstallCommand -Executable 'go' -Arguments @('build', '-o', $BinaryPath, '.\cmd\service') -Description 'Go service build'
    if (-not (Test-Path $BinaryPath)) {
        throw 'Building the service binary failed.'
    }
    Write-InstallLog -Level INFO -Message 'Go service build completed.'
    Invoke-LoggedInstallCommand -Executable 'go' -Arguments @('build', '-ldflags', '-H=windowsgui', '-o', $MonitorPath, '.\cmd\service') -Description 'Hidden background monitor build'
    if (-not (Test-Path $MonitorPath)) {
        throw 'Building the hidden background monitor failed.'
    }
    Write-InstallLog -Level INFO -Message 'Hidden background monitor build completed.'
    Invoke-LoggedInstallCommand -Executable 'go' -Arguments @('build', '-ldflags', '-H=windowsgui', '-o', $InstallerPath, '.\cmd\installer') -Description 'Go GUI installer build'
    if (-not (Test-Path $InstallerPath)) {
        throw 'Building the GUI installer failed.'
    }
    Write-InstallLog -Level INFO -Message 'Go installer build completed.'
    Invoke-LoggedInstallCommand -Executable $MSBuildPath -Arguments @($OverlayProject, '/m', '/p:Configuration=Release', '/p:Platform=x64', "/p:OutDir=$BinDir/") -Description 'Native Explorer COM handler build'
    if (-not (Test-Path $OverlayPath)) {
        throw 'Building the native Explorer COM handler failed.'
    }
    Write-InstallLog -Level INFO -Message 'Native Explorer handler build completed.'
    Write-InstallLog -Level INFO -Message "Service binary created at $BinaryPath"
    Write-InstallLog -Level INFO -Message "Background monitor created at $MonitorPath"
    Write-InstallLog -Level INFO -Message "GUI installer created at $InstallerPath"
    Write-InstallLog -Level INFO -Message "Explorer COM handler created at $OverlayPath"
}
finally {
    Pop-Location
}

New-Item -ItemType Directory -Path $AssetsDir -Force | Out-Null
Write-InstallLog -Level INFO -Message "Copying overlay icon assets to $AssetsDir"
Copy-Item -Path (Join-Path $RepoRoot 'assets\*') -Destination $AssetsDir -Force
Write-InstallLog -Level INFO -Message "Overlay icons installed at $AssetsDir"

Write-InstallLog -Level INFO -Message "Writing selected mount path to user config: $MountPath"
& $InstallerPath --mount-path $MountPath
$installerExitCode = $LASTEXITCODE
Write-InstallLog -Level INFO -Message "Folder selection/configuration process exited with code $installerExitCode."
if ($installerExitCode -ne 0) {
    Write-InstallLog -Level ERROR -Message "Folder selection/configuration process failed with exit code $installerExitCode."
    throw "The GUI installer did not complete successfully (exit code $installerExitCode)."
}
Write-InstallLog -Level INFO -Message 'Mount folder selection and config write completed.'

$RegSvr32 = Join-Path $env:WINDIR 'System32\regsvr32.exe'
Invoke-LoggedInstallCommand -Executable $RegSvr32 -Arguments @('/s', $OverlayPath) -Description 'Explorer COM handler registration'
Write-InstallLog -Level INFO -Message 'Explorer COM overlay handlers registered.'

$RunKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run'
New-Item -Path $RunKey -Force | Out-Null
Set-ItemProperty -Path $RunKey -Name $AppName -Value "`"$MonitorPath`""
Write-InstallLog -Level INFO -Message "Registered background monitor to start at user logon: $MonitorPath"

Write-InstallLog -Level INFO -Message "Installation complete. Application root: $InstallRoot"
Write-Host 'Installation complete.'
Write-Host "Binary: $BinaryPath"
Write-Host "Config: $(Join-Path $ConfigDir 'config.json')"
Write-Host "Log: $LogFile"
Write-Host 'Restart Windows Explorer before testing overlays; start rclone with the loopback RC flags described in INSTALLATION.md for automatic status updates.'
