$ErrorActionPreference = 'Stop'

$src = 'c:\projects\work\queteddy\explorer_status\assets'
$dst = 'C:\Program Files\RcloneExplorerStatus\assets'
if (-not (Test-Path $dst)) {
    New-Item -ItemType Directory -Force -Path $dst | Out-Null
}

Copy-Item -Path (Join-Path $src '*') -Destination $dst -Force
Write-Host "Deployed icons to $dst"
Get-ChildItem $dst | Select-Object Name, Length | Format-Table -AutoSize

$dll = 'C:\Program Files\RcloneExplorerStatus\bin\RcloneOverlay.dll'
if (Test-Path $dll) {
    & regsvr32.exe /s $dll
    Write-Host 'Overlay DLL re-registered'
}

# Ask Explorer to reload shell associations / overlays.
Add-Type -Namespace Native -Name Shell -MemberDefinition @'
[DllImport("shell32.dll")] public static extern void SHChangeNotify(int wEventId, uint uFlags, IntPtr dwItem1, IntPtr dwItem2);
'@
[Native.Shell]::SHChangeNotify(0x08000000, 0x0000, [IntPtr]::Zero, [IntPtr]::Zero) # SHCNE_ASSOCCHANGED
Write-Host 'Explorer association refresh notified'
