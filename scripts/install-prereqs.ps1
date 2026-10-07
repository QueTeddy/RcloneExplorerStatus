# Shared prerequisite checks for install.ps1 (dot-sourced).

function Test-IsWindows10OrNewer {
    $version = [Environment]::OSVersion.Version
    return ($version.Major -gt 10) -or ($version.Major -eq 10 -and $version.Build -ge 10240)
}

function Assert-WindowsRequirement {
    if ($env:OS -ne 'Windows_NT') {
        throw 'This application requires Windows 10 or Windows 11.'
    }
    if (-not (Test-IsWindows10OrNewer)) {
        throw 'Windows 10 or Windows 11 is required.'
    }
    Write-InstallLog -Level INFO -Message "Windows version OK: $([Environment]::OSVersion.VersionString)"
}

function Assert-ArchitectureRequirement {
    $arch = $env:PROCESSOR_ARCHITECTURE
    if ($arch -ne 'AMD64') {
        throw "Unsupported architecture '$arch'. This release targets 64-bit Windows (amd64)."
    }
    Write-InstallLog -Level INFO -Message 'Architecture OK: amd64'
}

function Assert-GoRequirement {
    $go = Get-Command go -ErrorAction SilentlyContinue
    if (-not $go) {
        $choice = Show-RequirementPrompt -Title 'Go toolchain required' -Message @"
Go is required to build Rclone Explorer Status from source.

Click Yes to open the Go download page.
Install Go 1.26+, ensure go.exe is on PATH, then rerun install.ps1.
"@
        if ($choice -eq 'Yes') {
            Start-Process 'https://go.dev/dl/'
        }
        throw 'Go was not found on PATH. Install Go 1.26 or later, then rerun install.ps1.'
    }
    $versionText = (& go version) -join ' '
    Write-InstallLog -Level INFO -Message "Go OK: $versionText"
}

function Resolve-RclonePath {
    $candidates = @()
    $cmd = Get-Command rclone.exe -ErrorAction SilentlyContinue
    if ($cmd) { $candidates += $cmd.Source }
    $cmd = Get-Command rclone -ErrorAction SilentlyContinue
    if ($cmd) { $candidates += $cmd.Source }
    $candidates += @(
        'C:\self_installed\rclone\rclone.exe',
        (Join-Path $env:LOCALAPPDATA 'rclone\rclone.exe'),
        (Join-Path $env:ProgramFiles 'rclone\rclone.exe'),
        (Join-Path ${env:ProgramFiles(x86)} 'rclone\rclone.exe')
    )
    foreach ($path in $candidates) {
        if ($path -and (Test-Path -LiteralPath $path)) {
            return (Resolve-Path -LiteralPath $path).Path
        }
    }
    return $null
}

function Show-RequirementPrompt {
    param(
        [Parameter(Mandatory = $true)][string]$Title,
        [Parameter(Mandatory = $true)][string]$Message
    )
    Add-Type -AssemblyName System.Windows.Forms | Out-Null
    $result = [System.Windows.Forms.MessageBox]::Show(
        $Message,
        $Title,
        [System.Windows.Forms.MessageBoxButtons]::YesNoCancel,
        [System.Windows.Forms.MessageBoxIcon]::Warning
    )
    return $result.ToString()
}

function Ensure-RcloneRequirement {
    $existing = Resolve-RclonePath
    if ($existing) {
        Write-InstallLog -Level INFO -Message "rclone OK: $existing"
        return $existing
    }

    Write-InstallLog -Level WARN -Message 'rclone was not found on PATH or common install locations.'
    $choice = Show-RequirementPrompt -Title 'rclone required' -Message @"
rclone is required for Explorer sync status and Sync/Audit actions.

Yes = install with winget (Rclone.Rclone)
No = open the rclone download page
Cancel = abort installation
"@

    if ($choice -eq 'Cancel') {
        throw 'Installation cancelled: rclone is required.'
    }

    if ($choice -eq 'Yes') {
        $winget = Get-Command winget -ErrorAction SilentlyContinue
        if (-not $winget) {
            Start-Process 'https://rclone.org/downloads/'
            throw 'winget is not available. Install rclone from https://rclone.org/downloads/ then rerun install.ps1.'
        }
        Write-InstallLog -Level INFO -Message 'Installing rclone with winget (Rclone.Rclone).'
        & winget install --id Rclone.Rclone -e --accept-package-agreements --accept-source-agreements
        if ($LASTEXITCODE -ne 0) {
            Start-Process 'https://rclone.org/downloads/'
            throw "winget rclone install failed (exit $LASTEXITCODE). Install rclone manually, then rerun install.ps1."
        }
        $env:Path = [System.Environment]::GetEnvironmentVariable('Path', 'Machine') + ';' +
            [System.Environment]::GetEnvironmentVariable('Path', 'User')
        $existing = Resolve-RclonePath
        if (-not $existing) {
            throw 'rclone was installed but could not be located. Close this window, open a new PowerShell session, and rerun install.ps1.'
        }
        Write-InstallLog -Level INFO -Message "rclone installed: $existing"
        return $existing
    }

    Start-Process 'https://rclone.org/downloads/'
    throw 'Install rclone, ensure rclone.exe is available, then rerun install.ps1.'
}

function Ensure-OverlayIcons {
    param([Parameter(Mandatory = $true)][string]$RepoRoot)

    $synced = Join-Path $RepoRoot 'assets\synced.ico'
    if (Test-Path -LiteralPath $synced) {
        Write-InstallLog -Level INFO -Message 'Overlay icon assets are present.'
        return
    }

    $python = Get-Command python -ErrorAction SilentlyContinue
    if (-not $python) {
        $python = Get-Command py -ErrorAction SilentlyContinue
    }
    if (-not $python) {
        throw 'Overlay icons are missing and Python was not found to generate them. Run scripts\generate_icons.py after installing Python, then rerun install.ps1.'
    }
    Write-InstallLog -Level INFO -Message 'Generating overlay icons with scripts\generate_icons.py'
    & $python.Source (Join-Path $RepoRoot 'scripts\generate_icons.py')
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $synced)) {
        throw 'Failed to generate overlay icons.'
    }
}

function Register-ExplorerContextMenus {
    param(
        [Parameter(Mandatory = $true)][string]$ShellHelperPath,
        [Parameter(Mandatory = $true)][string]$IconPath
    )

    # Use .NET registry APIs so the '*' file-class key is not treated as a wildcard.
    $hkcu = [Microsoft.Win32.Registry]::CurrentUser
    $sync = $hkcu.CreateSubKey('Software\Classes\*\shell\RcloneExplorerStatus.Sync')
    $sync.SetValue('', 'Sync with Rclone Explorer Status')
    $sync.SetValue('MUIVerb', 'Sync with Rclone Explorer Status')
    if (Test-Path -LiteralPath $IconPath) {
        $sync.SetValue('Icon', $IconPath)
    }
    $syncCmd = $sync.CreateSubKey('command')
    $syncCmd.SetValue('', "`"$ShellHelperPath`" sync `"%1`"")
    $syncCmd.Close()
    $sync.Close()

    $audit = $hkcu.CreateSubKey('Software\Classes\Directory\shell\RcloneExplorerStatus.Audit')
    $audit.SetValue('', 'Audit folder with Rclone Explorer Status')
    $audit.SetValue('MUIVerb', 'Audit folder with Rclone Explorer Status')
    if (Test-Path -LiteralPath $IconPath) {
        $audit.SetValue('Icon', $IconPath)
    }
    $auditCmd = $audit.CreateSubKey('command')
    $auditCmd.SetValue('', "`"$ShellHelperPath`" audit `"%1`"")
    $auditCmd.Close()
    $audit.Close()

    Write-InstallLog -Level INFO -Message "Registered Explorer context menus for Sync (files) and Audit (folders): $ShellHelperPath"
}

function Unregister-ExplorerContextMenus {
    $hkcu = [Microsoft.Win32.Registry]::CurrentUser
    try { $hkcu.DeleteSubKeyTree('Software\Classes\*\shell\RcloneExplorerStatus.Sync', $false) } catch {}
    try { $hkcu.DeleteSubKeyTree('Software\Classes\Directory\shell\RcloneExplorerStatus.Audit', $false) } catch {}
}

function Install-StartMenuShortcuts {
    param(
        [Parameter(Mandatory = $true)][string]$AppExe,
        [Parameter(Mandatory = $true)][string]$CliExe,
        [Parameter(Mandatory = $true)][string]$InstallRoot
    )

    $candidates = @(
        (Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs\Rclone Explorer Status'),
        (Join-Path $env:ProgramData 'Microsoft\Windows\Start Menu\Programs\Rclone Explorer Status')
    )
    $programs = $null
    $wsh = New-Object -ComObject WScript.Shell
    foreach ($candidate in $candidates) {
        try {
            New-Item -ItemType Directory -Path $candidate -Force -ErrorAction Stop | Out-Null
            $appShortcut = $wsh.CreateShortcut((Join-Path $candidate 'Rclone Explorer Status.lnk'))
            $appShortcut.TargetPath = $AppExe
            $appShortcut.WorkingDirectory = $InstallRoot
            $appShortcut.Description = 'Open Rclone Explorer Status status panel'
            $appShortcut.Save()

            $cliShortcut = $wsh.CreateShortcut((Join-Path $candidate 'Rclone Explorer Status CLI.lnk'))
            $cliShortcut.TargetPath = 'powershell.exe'
            $cliShortcut.Arguments = "-NoExit -Command `"& '$CliExe' status`""
            $cliShortcut.WorkingDirectory = $InstallRoot
            $cliShortcut.Description = 'Open CLI status for Rclone Explorer Status'
            $cliShortcut.Save()

            $programs = $candidate
            break
        } catch {
            Write-InstallLog -Level DEBUG -Message "Start Menu candidate failed ($candidate): $($_.Exception.Message)"
            continue
        }
    }
    if (-not $programs) {
        throw 'Could not create a Start Menu folder for Rclone Explorer Status.'
    }

    Write-InstallLog -Level INFO -Message "Start Menu shortcuts created under $programs"
}

function Remove-StartMenuShortcuts {
    @(
        (Join-Path $env:ProgramData 'Microsoft\Windows\Start Menu\Programs\Rclone Explorer Status'),
        (Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs\Rclone Explorer Status')
    ) | ForEach-Object {
        if (Test-Path $_) {
            Remove-Item -Path $_ -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
}

function Confirm-AddRcsToPath {
    Add-Type -AssemblyName System.Windows.Forms | Out-Null
    $result = [System.Windows.Forms.MessageBox]::Show(
        @"
Add the short command 'rcs' to your user PATH?

After install you can run (PowerShell, cmd, or Git Bash):

  rcs remaining
  rcs audit "S:\courses"
  rcs doctor

Yes = add the install bin folder to your user PATH
No = skip PATH (you can still use the full exe path)
Cancel = abort installation
"@,
        'Add rcs to PATH?',
        [System.Windows.Forms.MessageBoxButtons]::YesNoCancel,
        [System.Windows.Forms.MessageBoxIcon]::Question
    )
    switch ($result.ToString()) {
        'Yes' { return $true }
        'No' { return $false }
        default { throw 'Installation cancelled while configuring PATH.' }
    }
}

function Install-RcsShim {
    param(
        [Parameter(Mandatory = $true)][string]$BinDir,
        [Parameter(Mandatory = $true)][string]$CliExe
    )

    if (-not (Test-Path -LiteralPath $CliExe)) {
        throw "CLI executable not found for rcs shim: $CliExe"
    }

    $rcsExe = Join-Path $BinDir 'rcs.exe'
    Copy-Item -LiteralPath $CliExe -Destination $rcsExe -Force

    # Git Bash / MSYS friendly launcher without .exe extension (LF-only).
    $rcsSh = Join-Path $BinDir 'rcs'
    $drive = [System.IO.Path]::GetPathRoot($BinDir).TrimEnd('\')
    $tail = $BinDir.Substring($drive.Length).TrimStart('\').Replace('\', '/')
    $unixDir = '/' + $drive.TrimEnd(':').ToLowerInvariant() + '/' + $tail
    $shim = "#!/usr/bin/env bash`nexec `"$unixDir/rclone-explorer-status.exe`" `"$@`"`n"
    $utf8NoBom = New-Object System.Text.UTF8Encoding $false
    [System.IO.File]::WriteAllText($rcsSh, $shim, $utf8NoBom)

    Write-InstallLog -Level INFO -Message "Installed short command shims: $rcsExe and $rcsSh"
    return $rcsExe
}

function Add-BinDirToUserPath {
    param([Parameter(Mandatory = $true)][string]$BinDir)

    $BinDir = [System.IO.Path]::GetFullPath($BinDir.TrimEnd('\', '/'))
    $current = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ($null -eq $current) { $current = '' }
    $parts = @($current -split ';' | Where-Object { $_ -and $_.Trim() -ne '' })
    $already = $parts | Where-Object { $_.TrimEnd('\', '/') -ieq $BinDir }
    if ($already) {
        Write-InstallLog -Level INFO -Message "User PATH already contains: $BinDir"
    } else {
        $parts += $BinDir
        $updated = ($parts -join ';').Trim(';')
        [Environment]::SetEnvironmentVariable('Path', $updated, 'User')
        Write-InstallLog -Level INFO -Message "Added to user PATH: $BinDir"
    }

    # Update this process so the current session can call rcs immediately.
    $sessionParts = @($env:Path -split ';' | Where-Object { $_ -and $_.Trim() -ne '' })
    if (-not ($sessionParts | Where-Object { $_.TrimEnd('\', '/') -ieq $BinDir })) {
        $env:Path = "$BinDir;$env:Path"
    }

    # Notify Windows that environment variables changed (new terminals pick this up).
    try {
        Add-Type -Namespace Native -Name EnvBroadcast -MemberDefinition @'
[DllImport("user32.dll", SetLastError=true, CharSet=CharSet.Auto)]
public static extern IntPtr SendMessageTimeout(IntPtr hWnd, uint Msg, UIntPtr wParam, string lParam, uint fuFlags, uint uTimeout, out UIntPtr lpdwResult);
'@ -ErrorAction SilentlyContinue
        $HWND_BROADCAST = [IntPtr]0xffff
        $WM_SETTINGCHANGE = 0x1A
        $result = [UIntPtr]::Zero
        [void][Native.EnvBroadcast]::SendMessageTimeout($HWND_BROADCAST, $WM_SETTINGCHANGE, [UIntPtr]::Zero, 'Environment', 2, 5000, [ref]$result)
    } catch {
        Write-InstallLog -Level DEBUG -Message "PATH broadcast skipped: $($_.Exception.Message)"
    }
}

function Remove-BinDirFromUserPath {
    param([Parameter(Mandatory = $true)][string]$BinDir)

    $BinDir = [System.IO.Path]::GetFullPath($BinDir.TrimEnd('\', '/'))
    $current = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ([string]::IsNullOrWhiteSpace($current)) { return }
    $parts = @($current -split ';' | Where-Object {
        $_ -and ($_.TrimEnd('\', '/') -ine $BinDir)
    })
    [Environment]::SetEnvironmentVariable('Path', ($parts -join ';'), 'User')
    Write-InstallLog -Level INFO -Message "Removed from user PATH: $BinDir"
}

function Write-JsonNoBom {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)]$Object
    )
    $json = $Object | ConvertTo-Json -Depth 8
    $utf8NoBom = New-Object System.Text.UTF8Encoding $false
    [System.IO.File]::WriteAllText($Path, $json + [Environment]::NewLine, $utf8NoBom)
}

function Save-InstallBinPath {
    param(
        [Parameter(Mandatory = $true)][string]$ConfigPath,
        [Parameter(Mandatory = $true)][string]$BinDir
    )

    if (-not (Test-Path -LiteralPath $ConfigPath)) { return }
    try {
        $raw = [System.IO.File]::ReadAllText($ConfigPath)
        # Strip UTF-8 BOM if a previous PowerShell write added one.
        if ($raw.Length -gt 0 -and [int][char]$raw[0] -eq 0xFEFF) {
            $raw = $raw.Substring(1)
        }
        $configObject = $raw | ConvertFrom-Json
        if (-not $configObject.install) {
            $configObject | Add-Member -NotePropertyName install -NotePropertyValue ([pscustomobject]@{})
        }
        $configObject.install | Add-Member -NotePropertyName bin_path -NotePropertyValue $BinDir -Force
        $configObject.install | Add-Member -NotePropertyName command -NotePropertyValue 'rcs' -Force
        Write-JsonNoBom -Path $ConfigPath -Object $configObject
        Write-InstallLog -Level INFO -Message "Saved install bin_path to config: $BinDir"
    } catch {
        Write-InstallLog -Level WARN -Message "Could not save bin_path to config: $($_.Exception.Message)"
    }
}
