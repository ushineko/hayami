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

.PARAMETER WithSensors
    Also install LibreHardwareMonitor, which is where the panel reads the
    processor's temperature on Windows (spec 036). Opt-in, because it is a
    second program, it runs as administrator, and it loads a kernel driver
    (PawnIO). With this switch: winget installs it (and passing the switch
    accepts winget's source and package agreements for that one package);
    its settings are written with the web server on, if it has none yet; and
    it is started elevated, so Windows asks (UAC) and LibreHardwareMonitor
    offers PawnIO itself. Nothing else is installed for you.

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
    [switch] $WithSensors,
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

# LibreHardwareMonitor, where winget's portable install puts it.
$SensorsPackage = "LibreHardwareMonitor.LibreHardwareMonitor"
function Find-Sensors {
    $packages = Join-Path $env:LOCALAPPDATA "Microsoft\WinGet\Packages"
    return Get-ChildItem $packages -Directory -Filter "$SensorsPackage*" -ErrorAction SilentlyContinue |
        ForEach-Object { Join-Path $_.FullName "LibreHardwareMonitor.exe" } |
        Where-Object { Test-Path $_ } | Select-Object -First 1
}

# The settings LibreHardwareMonitor reads at start, beside its executable:
# the web server on, on its default port, with no password, which is what
# hayami asks (spec 036).
$SensorsConfig = @"
<?xml version="1.0" encoding="utf-8"?>
<configuration>
  <appSettings>
    <add key="runWebServerMenuItem" value="true" />
    <add key="listenerIp" value="127.0.0.1" />
    <add key="listenerPort" value="8085" />
    <add key="authenticationEnabled" value="false" />
  </appSettings>
</configuration>
"@

function Install-Sensors {
    Write-Step "LibreHardwareMonitor, for the processor's temperature"
    $exe = Find-Sensors
    if ($exe) {
        Write-Ok "already installed: $exe"
    } elseif ($DryRun) {
        Write-Note "would run: winget install --id $SensorsPackage --exact --accept-source-agreements --accept-package-agreements"
    } else {
        if (-not (Get-Command winget -ErrorAction SilentlyContinue)) {
            Write-Note "winget is not here. Install LibreHardwareMonitor from"
            Write-Note "https://github.com/LibreHardwareMonitor/LibreHardwareMonitor/releases and see the README, On Windows."
            return
        }
        & winget install --id $SensorsPackage --exact --accept-source-agreements --accept-package-agreements
        if ($LASTEXITCODE -ne 0) { Write-Note "winget did not install it; the panel runs without a CPU temperature."; return }
        $exe = Find-Sensors
        if (-not $exe) { Write-Note "installed, but not where winget puts it; see the README, On Windows."; return }
        Write-Ok $exe
    }

    $config = if ($exe) { [IO.Path]::ChangeExtension($exe, ".config") } else { "LibreHardwareMonitor.config, beside it" }
    if ($exe -and (Test-Path $config)) {
        Write-Ok "its settings are left as they are: $config"
        Write-Note "hayami reads its web server, on port 8085: Options -> Remote Web Server -> Run."
    } elseif ($DryRun) {
        Write-Note "would write, if it has no settings yet: $config (web server on, port 8085)"
    } else {
        [IO.File]::WriteAllText($config, $SensorsConfig, (New-Object Text.UTF8Encoding $false))
        Write-Ok "settings: web server on, port 8085: $config"
    }

    if ($DryRun) {
        Write-Note "would start it as administrator: Windows asks (UAC), and it offers PawnIO itself"
    } elseif (Get-Process LibreHardwareMonitor -ErrorAction SilentlyContinue) {
        Write-Ok "already running"
    } else {
        Write-Note "starting it as administrator: Windows asks (UAC), and on its first start it"
        Write-Note "offers to install PawnIO, the driver it reads the processor through. Say yes."
        try { Start-Process -FilePath $exe -Verb RunAs } catch { Write-Note "not started: $($_.Exception.Message)" }
    }
    Write-Note "In LibreHardwareMonitor: Options -> Run On Windows Startup keeps it running after a restart."
    Write-Note "Its web server listens on every network interface whatever its address setting says;"
    Write-Note "Windows Firewall's default (block inbound) is what keeps port 8085 off your network."
    Write-Note "Do not add an inbound rule for it: the same server can change fan settings."
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
if ($WithSensors) { Install-Sensors }

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
if (-not $WithSensors) {
    Write-Host "  .\scripts\install_windows.ps1 -WithSensors  and LibreHardwareMonitor, for the CPU temperature"
}
