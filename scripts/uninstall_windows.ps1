<#
.SYNOPSIS
    Remove what install_windows.ps1 installed, keeping your settings.

.DESCRIPTION
    The Windows counterpart of uninstall.sh: the two programs, their folder if
    that leaves it empty, and the Start menu and Startup shortcuts. Your
    settings and the last readings are left where they are, and named.

.PARAMETER DryRun
    Say what would be removed and remove nothing.

.EXAMPLE
    .\scripts\uninstall_windows.ps1
#>
[CmdletBinding()]
param(
    [switch] $DryRun,
    [string] $Destination = (Join-Path $env:LOCALAPPDATA "Programs\hayami"),
    [string] $StartMenuDir = [Environment]::GetFolderPath("Programs"),
    [string] $StartupDir = [Environment]::GetFolderPath("Startup")
)

$ErrorActionPreference = "Stop"

function Fail($message) { Write-Host "[FAIL] $message" -ForegroundColor Red; exit 1 }

if ($env:OS -ne "Windows_NT") { Fail "This removes the Windows install; on Linux run ./uninstall.sh." }

$Panel = Join-Path $Destination "hayami.exe"
$running = Get-Process hayami -ErrorAction SilentlyContinue | Where-Object { $_.Path -eq $Panel }
if ($running -and -not $DryRun) { Fail "hayami is running from $Destination. Quit it (right-click the panel, Quit) and run this again." }

Write-Host "Removing hayami ..."
$removed = 0
$paths = @(
    (Join-Path $StartupDir "hayami.lnk"),
    (Join-Path $StartMenuDir "hayami.lnk"),
    $Panel,
    (Join-Path $Destination "hayami-tui.exe")
)
foreach ($path in $paths) {
    if (-not (Test-Path $path)) { continue }
    $removed++
    if ($DryRun) { Write-Host "  would remove $path"; continue }
    Remove-Item -Force $path
    Write-Host "  removed $path"
}
if ((Test-Path $Destination) -and -not (Get-ChildItem $Destination) -and -not $DryRun) {
    Remove-Item $Destination
}
if ($removed -eq 0) { Write-Host "  nothing to remove; install_windows.ps1 has not run, or has already been undone" }

Write-Host ""
Write-Host "Left alone:"
Write-Host "  $(Join-Path $env:APPDATA 'hayami\settings.yaml')   your settings"
Write-Host "  $(Join-Path $env:LOCALAPPDATA 'hayami\sections.json')   the last readings"
