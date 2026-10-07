$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$AppName = 'RcloneExplorerStatus'
$LogFile = Join-Path $env:LOCALAPPDATA "$AppName\logs\uninstall.log"

function Write-UninstallLog {
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

function Read-NewUninstallLogLines {
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

function Invoke-ElevatedUninstaller {
	Write-UninstallLog -Level INFO -Message 'Launching elevated uninstaller; live child-process log entries will follow.'
    $offset = (Get-Item -LiteralPath $LogFile).Length
    $arguments = @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', "`"$PSCommandPath`"")
    try {
        $process = Start-Process -FilePath 'powershell.exe' -Verb RunAs -ArgumentList $arguments -PassThru
    } catch {
        Write-UninstallLog -Level ERROR -Message "Administrator approval was cancelled or elevated PowerShell could not start: $($_.Exception.Message)"
        return 1223
    }

    Write-Host "Elevated uninstaller started (PID $($process.Id)); streaming its log output."
    while (-not $process.HasExited) {
        $offset = Read-NewUninstallLogLines -Offset $offset
        $null = $process.WaitForExit(300)
        $process.Refresh()
    }
    $offset = Read-NewUninstallLogLines -Offset $offset
    Write-UninstallLog -Level INFO -Message "Elevated uninstaller exited with code $($process.ExitCode)."
    return $process.ExitCode
}

function Unregister-OverlayDll {
    param([Parameter(Mandatory = $true)][string]$DllPath)

    $regSvr32 = Join-Path $env:WINDIR 'System32\regsvr32.exe'
    $arguments = "/s /u `"$DllPath`""
    $process = Start-Process -FilePath $regSvr32 -ArgumentList $arguments -Wait -PassThru
    Write-UninstallLog -Level INFO -Message "regsvr32 unregistration exited with code $($process.ExitCode)."
    if ($process.ExitCode -ne 0) {
        throw "Unregistering the Explorer COM handler failed (exit code $($process.ExitCode))."
    }
}

function Schedule-FileDeletionAtReboot {
    param([Parameter(Mandatory = $true)][string]$FilePath)

    if (-not ('RcloneStatusFileCleanup' -as [type])) {
        Add-Type -TypeDefinition @'
using System.Runtime.InteropServices;
public static class RcloneStatusFileCleanup {
    [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    public static extern bool MoveFileEx(string existingFileName, string newFileName, int flags);
}
'@
    }

    if ([RcloneStatusFileCleanup]::MoveFileEx($FilePath, $null, 4)) {
        Write-UninstallLog -Level INFO -Message "Scheduled locked file for deletion at reboot using MoveFileEx: $FilePath"
        return
    }

    $errorCode = [Runtime.InteropServices.Marshal]::GetLastWin32Error()
    Write-UninstallLog -Level WARN -Message "MoveFileEx could not queue $FilePath (Win32 error $errorCode); using PendingFileRenameOperations."

    $sessionManagerPath = 'SYSTEM\CurrentControlSet\Control\Session Manager'
    $registryKey = [Microsoft.Win32.Registry]::LocalMachine.OpenSubKey($sessionManagerPath, $true)
    if ($null -eq $registryKey) {
        throw "Could not open HKLM\$sessionManagerPath to schedule locked-file cleanup."
    }
    try {
        $pending = $registryKey.GetValue(
            'PendingFileRenameOperations',
            [string[]]@(),
            [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames
        )
        $nativePath = '\??\' + [System.IO.Path]::GetFullPath($FilePath)
        if ($pending -notcontains $nativePath) {
            $pending = @($pending) + @($nativePath, '')
            $registryKey.SetValue(
                'PendingFileRenameOperations',
                [string[]]$pending,
                [Microsoft.Win32.RegistryValueKind]::MultiString
            )
        }
    } finally {
        $registryKey.Close()
    }

    if (-not ($pending -contains $nativePath)) {
        $errorCode = [Runtime.InteropServices.Marshal]::GetLastWin32Error()
        throw "Could not schedule locked file for deletion at reboot: $FilePath (Win32 error $errorCode)."
    }
    Write-UninstallLog -Level INFO -Message "Scheduled locked file for deletion at reboot using PendingFileRenameOperations: $FilePath"
}

function Remove-InstalledFiles {
    param([Parameter(Mandatory = $true)][string]$InstallPath)

    try {
        Remove-Item -Path $InstallPath -Recurse -Force -ErrorAction Stop
        Write-UninstallLog -Level INFO -Message "Removed installed application files: $InstallPath"
        return $true
    } catch {
        $message = $_.Exception.Message
        if ($message -match 'access.*denied|used by another process|being used by another process') {
            $lockedDll = Join-Path $InstallPath 'bin\RcloneOverlay.dll'
            if (-not (Test-Path -LiteralPath $lockedDll)) {
                throw
            }

            Write-UninstallLog -Level WARN -Message "Could not remove $InstallPath because Explorer still has a file loaded: $message"
            Schedule-FileDeletionAtReboot -FilePath $lockedDll
            Write-UninstallLog -Level WARN -Message "Scheduled the Explorer-loaded DLL for deletion at reboot: $lockedDll"

            Get-ChildItem -LiteralPath $InstallPath -File -Recurse -Force -ErrorAction SilentlyContinue |
                Where-Object { $_.FullName -ne $lockedDll } |
                ForEach-Object {
                    try {
                        Remove-Item -LiteralPath $_.FullName -Force -ErrorAction Stop
                    } catch {
                        Write-UninstallLog -Level WARN -Message "Could not remove remaining file $($_.FullName): $($_.Exception.Message)"
                    }
                }
            Get-ChildItem -LiteralPath $InstallPath -Directory -Recurse -Force -ErrorAction SilentlyContinue |
                Sort-Object { $_.FullName.Length } -Descending |
                ForEach-Object { Remove-Item -LiteralPath $_.FullName -Force -ErrorAction SilentlyContinue }

            $script:RebootRequired = $true
            return $false
        }
        throw
    }
}

trap {
    $failure = $_
    try {
        Write-UninstallLog -Level ERROR -Message "Unhandled uninstall failure: $($failure.Exception.Message)"
        Write-UninstallLog -Level DEBUG -Message "Failure location: $($failure.InvocationInfo.PositionMessage)"
    } catch {
        Write-Host "Could not write uninstall failure to log: $($_.Exception.Message)"
    }
    exit 1
}

Write-UninstallLog -Level INFO -Message 'Starting RcloneExplorerStatus uninstall.'
$RebootRequired = $false

$RunKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run'
if (Test-Path $RunKey) {
    Remove-ItemProperty -Path $RunKey -Name $AppName -ErrorAction SilentlyContinue
    Write-UninstallLog -Level INFO -Message 'Removed background monitor from current-user logon startup.'
}

Write-UninstallLog -Level INFO -Message 'Checking current Windows identity and administrator status.'
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($identity)
 $isAdministrator = $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
Write-UninstallLog -Level INFO -Message "Administrator token check completed (is_admin=$isAdministrator)."
if (-not $isAdministrator) {
    Write-UninstallLog -Level INFO -Message 'Requesting administrator permission to unregister the Explorer COM handler.'
    $elevatedExitCode = Invoke-ElevatedUninstaller
    if ($elevatedExitCode -ne 0) {
        Write-UninstallLog -Level ERROR -Message "Elevated uninstall did not complete (exit code $elevatedExitCode)."
    }
    exit $elevatedExitCode
}

$MachineInstallPath = Join-Path $env:ProgramFiles $AppName
if (Test-Path $MachineInstallPath) {
    Write-UninstallLog -Level INFO -Message "Checking machine install location: $MachineInstallPath"
    $MonitorPath = Join-Path $MachineInstallPath 'bin\rclone-explorer-status-monitor.exe'
    $runningMonitor = Get-CimInstance Win32_Process -Filter "Name='rclone-explorer-status-monitor.exe'" -ErrorAction SilentlyContinue |
        Where-Object { $_.ExecutablePath -eq $MonitorPath }
    foreach ($process in $runningMonitor) {
        Stop-Process -Id $process.ProcessId -Force
        Write-UninstallLog -Level INFO -Message "Stopped background monitor process $($process.ProcessId)."
    }
    $OverlayPath = Join-Path $MachineInstallPath 'bin\RcloneOverlay.dll'
    if (Test-Path $OverlayPath) {
        Write-UninstallLog -Level INFO -Message "Unregistering machine Explorer COM handler: $OverlayPath"
        Unregister-OverlayDll -DllPath $OverlayPath
        Write-UninstallLog -Level INFO -Message 'Explorer COM overlay handlers unregistered.'
    } else {
        Write-UninstallLog -Level WARN -Message "No native overlay DLL found at $OverlayPath; registry unregistration skipped."
    }
    Write-UninstallLog -Level INFO -Message "Removing machine application files: $MachineInstallPath"
    $null = Remove-InstalledFiles -InstallPath $MachineInstallPath
} else {
    Write-UninstallLog -Level INFO -Message "No machine install found at $MachineInstallPath."
}

$UserInstallPath = Join-Path $env:LOCALAPPDATA "$AppName\bin"
if (Test-Path $UserInstallPath) {
    Write-UninstallLog -Level INFO -Message "Checking per-user install location: $UserInstallPath"
    $OverlayPath = Join-Path $UserInstallPath 'RcloneOverlay.dll'
    if (Test-Path $OverlayPath) {
        Write-UninstallLog -Level INFO -Message "Unregistering per-user Explorer COM handler: $OverlayPath"
        Unregister-OverlayDll -DllPath $OverlayPath
        Write-UninstallLog -Level INFO -Message 'Per-user Explorer COM overlay handlers unregistered.'
    } else {
        Write-UninstallLog -Level WARN -Message "No native overlay DLL found at $OverlayPath; registry unregistration skipped."
    }
    Write-UninstallLog -Level INFO -Message "Removing per-user application binaries: $UserInstallPath"
    $null = Remove-InstalledFiles -InstallPath $UserInstallPath
} else {
    Write-UninstallLog -Level INFO -Message "No per-user binaries found at $UserInstallPath."
}

Write-UninstallLog -Level INFO -Message 'User configuration, status state, and logs were preserved.'
if ($RebootRequired) {
    Write-UninstallLog -Level WARN -Message 'Reboot Windows to delete the Explorer-loaded DLL. Rerun uninstall.ps1 after reboot to remove any remaining empty installation directories.'
    Write-Host 'Uninstall registry cleanup complete. Reboot Windows, then rerun uninstall.ps1 to remove remaining files. User config, state, and logs were preserved.'
} else {
    Write-Host 'Uninstall complete. Configuration, state, and logs were preserved.'
}
