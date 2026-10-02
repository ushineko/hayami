<#
.SYNOPSIS
    Build hayami from this checkout and install it for the current user.

.DESCRIPTION
    The Windows counterpart of install.sh. Builds the two programs, puts them
    in a folder of their own, and adds a Start menu shortcut to the panel --
    and, with -Autostart, one in the Startup folder so the panel starts at
    login.

    Everything is per-user: no administrator rights, no registry writes, no
    PATH changes. Re-running is safe. uninstall_windows.ps1 removes exactly
    what this writes and leaves your settings.

    The desktop panel links OpenGL through cgo, so the build needs an x86_64
    mingw gcc. One on PATH is used; otherwise the WinLibs package winget
    installs is found where winget put it.

.PARAMETER Autostart
    Also start the panel when you log in.

.PARAMETER DryRun
    Say what would be done and change nothing. Needs neither Go nor gcc.

.PARAMETER SkipBuild
    Install the hayami.exe and hayami-tui.exe already in the checkout.

.PARAMETER Destination
    Where the programs go. Defaults to %LOCALAPPDATA%\Programs\hayami.

.EXAMPLE
    .\scripts\install_windows.ps1 -Autostart
#>
[CmdletBinding()]
param(
    [switch] $Autostart,
    [switch] $DryRun,
    [switch] $SkipBuild,
    [string] $Destination = (Join-Path $env:LOCALAPPDATA "Programs\hayami"),
    [string] $StartMenuDir = [Environment]::GetFolderPath("Programs"),
    [string] $StartupDir = [Environment]::GetFolderPath("Startup")
)

$ErrorActionPreference = "Stop"

function Write-Step($message) { Write-Host "[step] $message" -ForegroundColor Cyan }
function Write-Ok($message) { Write-Host "[ok]   $message" -ForegroundColor Green }
function Write-Note($message) { Write-Host "       $message" }
function Fail($message) { Write-Host "[FAIL] $message" -ForegroundColor Red; exit 1 }

if ($env:OS -ne "Windows_NT") { Fail "This installs for Windows; on Linux run ./install.sh." }

$Root = Split-Path -Parent $PSScriptRoot
$Version = (Get-Content (Join-Path $Root "VERSION") -Raw).Trim()
$Panel = Join-Path $Destination "hayami.exe"
$Pane = Join-Path $Destination "hayami-tui.exe"
$Shortcut = Join-Path $StartMenuDir "hayami.lnk"
$Startup = Join-Path $StartupDir "hayami.lnk"

# The mingw gcc: on PATH, or where winget installs WinLibs.
function Find-Gcc {
    $onPath = Get-Command gcc -ErrorAction SilentlyContinue
    if ($onPath) { return $onPath.Source }
    $packages = Join-Path $env:LOCALAPPDATA "Microsoft\WinGet\Packages"
    $found = Get-ChildItem $packages -Directory -Filter "BrechtSanders.WinLibs*" -ErrorAction SilentlyContinue |
        ForEach-Object { Join-Path $_.FullName "mingw64\bin\gcc.exe" } |
        Where-Object { Test-Path $_ } | Select-Object -First 1
    return $found
}

function Build {
    Write-Step "Building hayami $Version"
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) { Fail "Go is not on PATH. winget install --id GoLang.Go -e" }
    $gcc = Find-Gcc
    if (-not $gcc) { Fail "No mingw gcc. The panel links OpenGL through cgo: winget install --id BrechtSanders.WinLibs.POSIX.UCRT -e" }
    $target = (& $gcc -dumpmachine | Out-String).Trim()
    if ($target -notmatch '^x86_64-.*mingw') { Fail "gcc at $gcc targets '$target'; the panel needs an x86_64 mingw gcc." }
    Write-Ok "gcc $((& $gcc -dumpfullversion | Out-String).Trim()) ($target)"

    $ldflags = "-X github.com/ushineko/hayami/internal/buildinfo.version=$Version"
    $savedPath = $env:PATH
    $savedCgo = $env:CGO_ENABLED
    try {
        $env:PATH = (Split-Path -Parent $gcc) + ";" + $env:PATH
        Push-Location $Root
        # migrated_fynedo: see the Makefile. -H windowsgui: the panel is a
        # window, and a console would open beside it.
        $env:CGO_ENABLED = "1"
        & go build -trimpath -tags migrated_fynedo -ldflags "$ldflags -H windowsgui" -o hayami.exe ./cmd/hayami
        if ($LASTEXITCODE -ne 0) { Fail "Building the panel failed." }
        $env:CGO_ENABLED = "0"
        & go build -trimpath -ldflags $ldflags -o hayami-tui.exe ./cmd/hayami-tui
        if ($LASTEXITCODE -ne 0) { Fail "Building the terminal panel failed." }
    } finally {
        Pop-Location
        $env:PATH = $savedPath
        $env:CGO_ENABLED = $savedCgo
    }
    Write-Ok "hayami.exe and hayami-tui.exe"
}

function New-Shortcut($path, $target) {
    if ($DryRun) { Write-Note "would create: $path -> $target"; return }
    New-Item -ItemType Directory -Force (Split-Path -Parent $path) | Out-Null
    $shell = New-Object -ComObject WScript.Shell
    $link = $shell.CreateShortcut($path)
    $link.TargetPath = $target
    $link.WorkingDirectory = Split-Path -Parent $target
    $link.Description = "Peripheral batteries, bandwidth, cooler and usage, read at a glance"
    $link.Save()
    Write-Ok $path
}

Write-Host "Installing hayami $Version from $Root"

if ($DryRun) {
    if (-not $SkipBuild) { Write-Note "would build hayami.exe and hayami-tui.exe" }
} elseif (-not $SkipBuild) {
    Build
}

# A running panel holds its file open, and Windows will not replace it.
$running = Get-Process hayami -ErrorAction SilentlyContinue | Where-Object { $_.Path -eq $Panel }
if ($running -and -not $DryRun) { Fail "hayami is running from $Destination. Quit it (right-click the panel, Quit) and run this again." }

Write-Step "Installing to $Destination"
foreach ($name in "hayami.exe", "hayami-tui.exe") {
    $from = Join-Path $Root $name
    $to = Join-Path $Destination $name
    if ($DryRun) { Write-Note "would copy: $from -> $to"; continue }
    if (-not (Test-Path $from)) { Fail "$from is not built. Run without -SkipBuild." }
    New-Item -ItemType Directory -Force $Destination | Out-Null
    Copy-Item -Force $from $to
    Write-Ok $to
}

Write-Step "Start menu"
New-Shortcut $Shortcut $Panel
if ($Autostart) {
    Write-Step "Starting it at login"
    New-Shortcut $Startup $Panel
}

Write-Host ""
Write-Host "Done."
Write-Host ""
Write-Host "  hayami        the panel, from the Start menu. Drag it anywhere to move it;"
Write-Host "                right-click it for the preferences."
Write-Host "  hayami-tui    the same readings in a terminal: $Pane"
if (-not $Autostart) {
    Write-Host ""
    Write-Host "  .\scripts\install_windows.ps1 -Autostart    and start it when you log in"
}
