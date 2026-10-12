<#
.SYNOPSIS
    Build hayami's installer for Windows with fynstall.

.DESCRIPTION
    Builds hayami.exe and hayami-tui.exe as install_windows.ps1 does, draws the
    icon, and runs `fynstall build` on fynstall.yaml (spec 059). The result is
    dist\hayami-<version>-windows-amd64-installer.exe, which holds both
    programs and needs no toolchain to run.

    fynstall is github.com/ushineko/fynstall. The installer is built against
    the fynstall that go.mod requires, so the builder must be that version: a
    released builder, or a checkout named with -RuntimePath.

    The wizard links OpenGL through cgo, so the build needs an x86_64 mingw
    gcc, found as install_windows.ps1 finds it.

.PARAMETER Fynstall
    The fynstall builder. Default: fynstall on PATH.

.PARAMETER RuntimePath
    A fynstall checkout to build the installer against, for a fynstall that is
    not released yet.

.PARAMETER SkipBuild
    Use the hayami.exe and hayami-tui.exe already in the checkout.

.PARAMETER WithCliOnly
    Also build the command-line-only installer, which has no wizard.

.PARAMETER Windowed
    Build the installer with the wizard, and its uninstaller, as programs with
    a window (fynstall's --windowed). They show no console on any Windows; on
    a terminal they print but ask nothing, and the command-line-only installer
    is the one for a terminal. A release is built this way, with -WithCliOnly.

.EXAMPLE
    go install github.com/ushineko/fynstall/cmd/fynstall@v0.2.1
    .\scripts\installer_windows.ps1 -Windowed -WithCliOnly

.EXAMPLE
    .\scripts\installer_windows.ps1 -Fynstall ..\fynstall\bin\fynstall.exe -RuntimePath ..\fynstall
#>
[CmdletBinding()]
param(
    [string] $Fynstall = "fynstall",
    [string] $RuntimePath,
    [switch] $SkipBuild,
    [switch] $WithCliOnly,
    [switch] $Windowed
)

$ErrorActionPreference = "Stop"

function Write-Step($message) { Write-Host "[step] $message" -ForegroundColor Cyan }
function Write-Ok($message) { Write-Host "[ok]   $message" -ForegroundColor Green }
function Fail($message) { Write-Host "[FAIL] $message" -ForegroundColor Red; exit 1 }

$Root = Split-Path -Parent $PSScriptRoot

# The mingw gcc: on PATH, or where winget installs WinLibs. The same search
# as install_windows.ps1's.
function Find-Gcc {
    $onPath = Get-Command gcc -ErrorAction SilentlyContinue
    if ($onPath) { return $onPath.Source }
    $packages = Join-Path $env:LOCALAPPDATA "Microsoft\WinGet\Packages"
    return Get-ChildItem $packages -Directory -Filter "BrechtSanders.WinLibs*" -ErrorAction SilentlyContinue |
        ForEach-Object { Join-Path $_.FullName "mingw64\bin\gcc.exe" } |
        Where-Object { Test-Path $_ } | Select-Object -First 1
}

if (-not (Get-Command $Fynstall -ErrorAction SilentlyContinue)) {
    Fail "No fynstall at '$Fynstall'. Install the version go.mod requires (go install github.com/ushineko/fynstall/cmd/fynstall@<version>), or pass -Fynstall."
}

if (-not $SkipBuild) {
    & (Join-Path $PSScriptRoot "install_windows.ps1") -BuildOnly
    if ($LASTEXITCODE -ne 0) { Fail "Building hayami failed." }
}
foreach ($name in "hayami.exe", "hayami-tui.exe") {
    if (-not (Test-Path (Join-Path $Root $name))) { Fail "$name is not built. Run without -SkipBuild." }
}

$gcc = Find-Gcc
if (-not $gcc) { Fail "No mingw gcc. The wizard links OpenGL through cgo: winget install --id BrechtSanders.WinLibs.POSIX.UCRT -e" }

$savedPath = $env:PATH
try {
    $env:PATH = (Split-Path -Parent $gcc) + ";" + $env:PATH
    Push-Location $Root

    Write-Step "Drawing the icon"
    New-Item -ItemType Directory -Force (Join-Path $Root "dist") | Out-Null
    & go run ./tools/appicon -size 512 -out (Join-Path $Root "dist\hayami.png")
    if ($LASTEXITCODE -ne 0) { Fail "Drawing the icon failed." }
    Write-Ok "dist\hayami.png"

    Write-Step "Building the installer"
    $arguments = @("build", "-c", (Join-Path $Root "fynstall.yaml"))
    if ($RuntimePath) { $arguments += @("--runtime-path", (Resolve-Path $RuntimePath).Path) }
    if ($WithCliOnly) { $arguments += "--with-cli-only" }
    if ($Windowed) { $arguments += "--windowed" }
    & $Fynstall @arguments
    if ($LASTEXITCODE -ne 0) { Fail "fynstall build failed." }
} finally {
    Pop-Location
    $env:PATH = $savedPath
}

Write-Host ""
Write-Host "Done. Run the installer in dist\ : from Explorer it opens its wizard,"
if ($Windowed) {
    Write-Host "and --dry-run lists what it would change. It asks nothing on a terminal:"
    Write-Host "the cli-installer, built with -WithCliOnly, is the one that does."
} else {
    Write-Host "from a terminal it asks there, and --dry-run lists what it would change."
}
