<#
.SYNOPSIS
    Build hayami from this checkout and install it for the current user.

.DESCRIPTION
    The Windows counterpart of install.sh. Builds the two programs, puts them
    in a folder of their own, and adds a Start menu shortcut to the panel --
    and, with -Autostart, one in the Startup folder so the panel starts at
    login.

    Everything is per-user: no administrator rights, no registry writes, no
    PATH changes -- except with -WithSensors, whose one elevated step sets up
    LibreHardwareMonitor. Re-running is safe. uninstall_windows.ps1 removes
    exactly what this writes for hayami and leaves your settings.

    The desktop panel links OpenGL through cgo, so the build needs an x86_64
    mingw gcc. One on PATH is used; otherwise the WinLibs package winget
    installs is found where winget put it.

.PARAMETER Autostart
    Also start the panel when you log in.

.PARAMETER WithSensors
    Also set up LibreHardwareMonitor, which is where the panel reads the
    processor's temperature on Windows (specs 036, 042). Opt-in, because it is
    a second program, it runs as administrator, and it loads a kernel driver
    (PawnIO). With this switch, each step done only if it is not done already:
    winget installs it (passing the switch accepts winget's source and package
    agreements for that one package); its settings get the web server on, the
    port hayami asks, no password, and start minimized with closing the window
    hiding it rather than quitting it (a backup is kept); its startup task is
    registered, the one its own Run On Windows Startup makes; and it is started
    through that task. The settings, the task and the start are one elevated
    step, so Windows asks (UAC) once. It ends by checking that data.json has a
    processor temperature, and says which step is missing if not.

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

# The startup task's name: the one LibreHardwareMonitor's own Options -> Run On
# Windows Startup gives it, so that menu shows it as on.
$SensorsTask = "LibreHardwareMonitor"

# The processor temperatures hayami reads, in its order: core.LHMCPULabels,
# which a test holds this list to.
$SensorsCPULabels = @("Core (Tdie)", "Core (Tctl/Tdie)", "Core (Tctl)", "CPU Package")

# The port hayami asks: its lhm setting's, or LibreHardwareMonitor's default.
function Get-SensorsPort {
    $settings = Join-Path $env:APPDATA "hayami\settings.yaml"
    if (Test-Path $settings) {
        $line = Select-String -Path $settings -Pattern '^\s*lhm:\s*(\S+)' | Select-Object -First 1
        if ($line) {
            $uri = $null
            if ([Uri]::TryCreate($line.Matches[0].Groups[1].Value.Trim('"', "'"), [UriKind]::Absolute, [ref]$uri) -and $uri.Port -gt 0) {
                return $uri.Port
            }
        }
    }
    return 8085
}

# Whether the startup task is registered for this executable, at highest
# privileges. Reading a task needs no administrator rights.
function Test-SensorsTask($exe) {
    $task = Get-ScheduledTask -TaskName $SensorsTask -ErrorAction SilentlyContinue
    if (-not $task) { return $false }
    $runs = $task.Actions | Where-Object { $_.Execute -and ($_.Execute.Trim('"') -ieq $exe) }
    return [bool]$runs -and $task.Principal.RunLevel -eq "Highest"
}

# The processor's temperature from LibreHardwareMonitor's data.json, by label
# under its processor hardware, or $null. Read-only.
function Find-SensorsTemperature($url) {
    try { $root = Invoke-RestMethod -Uri $url -TimeoutSec 2 -UseBasicParsing } catch { return $null }
    $found = @{}
    $stack = New-Object System.Collections.Stack
    $stack.Push($root)
    while ($stack.Count -gt 0) {
        $node = $stack.Pop()
        if ($node.SensorId -and $node.Type -eq "Temperature" -and ($node.SensorId -like "/amdcpu/*" -or $node.SensorId -like "/intelcpu/*")) {
            if (-not $found.ContainsKey($node.Text)) { $found[$node.Text] = $node.Value }
        }
        foreach ($child in @($node.Children)) { if ($child) { $stack.Push($child) } }
    }
    foreach ($label in $SensorsCPULabels) {
        if ($found.ContainsKey($label)) { return "$label $($found[$label])" }
    }
    return ""
}

# Ask data.json for the processor's temperature for up to $seconds, and say
# which step is missing when it does not come.
function Confirm-Sensors($url, $seconds) {
    $deadline = (Get-Date).AddSeconds($seconds)
    do {
        $reading = Find-SensorsTemperature $url
        if ($reading) { Write-Ok "hayami can read it: $reading, from $url"; return $true }
        if ((Get-Date) -lt $deadline) { Start-Sleep -Seconds 2 }
    } while ((Get-Date) -lt $deadline)
    if ($null -eq $reading) {
        Write-Note "no answer from ${url}: LibreHardwareMonitor is not running, or its web server is off or on another port."
    } else {
        Write-Note "$url answers but has no processor temperature: is PawnIO installed? LibreHardwareMonitor offers it on first start."
    }
    return $false
}

# Write-FirewallNote says what keeps LibreHardwareMonitor's web server off the
# network, on every path that sets it up or would: it listens on every
# interface whatever its address setting says.
function Write-FirewallNote($port) {
    Write-Note "Its web server listens on every network interface whatever its address setting says;"
    Write-Note "Windows Firewall's default (block inbound) is what keeps port $port off your network."
    Write-Note "Do not add an inbound rule for it: the same server can change fan settings."
}

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

    $port = Get-SensorsPort
    $url = "http://127.0.0.1:$port/data.json"
    $settingsScript = Join-Path $PSScriptRoot "lhm_settings.ps1"

    if (-not $exe) {
        # A dry run on a machine without it: say every step, check nothing.
        Write-Note "would set its settings (stopped first): web server on, port $port, no password,"
        Write-Note "start minimized, closing the window hides it in the notification area"
        Write-Note "would register the startup task '$SensorsTask': at logon, highest privileges"
        Write-Note "would start it through the task: one UAC prompt for all of this; it offers PawnIO itself"
        Write-FirewallNote $port
        return
    }

    $config = [IO.Path]::ChangeExtension($exe, ".config")
    $pending = & $settingsScript -Path $config -Port $port -DryRun
    $settingsChange = -not ($pending -contains "unchanged")
    $taskReady = Test-SensorsTask $exe
    $running = [bool](Get-Process LibreHardwareMonitor -ErrorAction SilentlyContinue)

    Write-Step "Its settings, its startup task, running"
    if ($settingsChange) {
        foreach ($line in $pending) { if ($line -like "would set *") { Write-Note "settings: $line" } }
    } else {
        Write-Ok "settings: web server on port $port, no password, starts minimized, closing hides it"
    }
    if ($taskReady) { Write-Ok "startup task '$SensorsTask' at highest privileges" } else { Write-Note "startup task '$SensorsTask': not registered for $exe" }
    if ($running) { Write-Ok "running" } else { Write-Note "not running" }

    if ($settingsChange -or -not $taskReady -or -not $running) {
        $steps = @()
        if ($settingsChange -and $running) { $steps += "stop it (it rewrites its settings when it exits)" }
        if ($settingsChange) { $steps += "set its settings, keeping a backup ($config.bak-hayami)" }
        if (-not $taskReady) { $steps += "register the startup task '$SensorsTask': at logon, highest privileges" }
        $steps += "start it through the task"
        if ($DryRun) {
            foreach ($s in $steps) { Write-Note "would $s" }
            Write-Note "all of that in one elevated step: Windows asks (UAC) once; on a first start it offers PawnIO"
        } else {
            Write-Note "as administrator, in one step (Windows asks once):"
            foreach ($s in $steps) { Write-Note "  $s" }
            $user = "$env:USERDOMAIN\$env:USERNAME"
            $elevated = @"
`$ErrorActionPreference = 'Stop'
`$exe = '$($exe -replace "'", "''")'
`$dir = Split-Path -Parent `$exe
if (`$$settingsChange) {
    Get-Process LibreHardwareMonitor -ErrorAction SilentlyContinue | Stop-Process -Force
    Get-Process LibreHardwareMonitor -ErrorAction SilentlyContinue | Wait-Process -Timeout 10 -ErrorAction SilentlyContinue
    & '$($settingsScript -replace "'", "''")' -Path '$($config -replace "'", "''")' -Port $port | Out-Null
}
if (-not `$$taskReady) {
    `$a = New-ScheduledTaskAction -Execute `$exe -WorkingDirectory `$dir
    `$t = New-ScheduledTaskTrigger -AtLogOn -User '$user'
    `$p = New-ScheduledTaskPrincipal -UserId '$user' -LogonType Interactive -RunLevel Highest
    `$s = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable -ExecutionTimeLimit ([TimeSpan]::Zero)
    Register-ScheduledTask -TaskName '$SensorsTask' -Description 'Starts LibreHardwareMonitor on Windows startup.' -Action `$a -Trigger `$t -Principal `$p -Settings `$s -Force | Out-Null
}
if (-not (Get-Process LibreHardwareMonitor -ErrorAction SilentlyContinue)) { Start-ScheduledTask -TaskName '$SensorsTask' }
"@
            $encoded = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($elevated))
            try {
                $p = Start-Process powershell.exe -Verb RunAs -Wait -PassThru -ArgumentList "-NoProfile", "-ExecutionPolicy", "Bypass", "-EncodedCommand", $encoded
                if ($p.ExitCode -ne 0) { Write-Note "the elevated step ended with code $($p.ExitCode)" }
            } catch {
                Write-Note "not done: $($_.Exception.Message)"
            }
        }
    }

    if ($DryRun -and -not $running) {
        Write-Note "would then check $url for the processor's temperature"
    } else {
        Write-Step "Checking $url"
        [void](Confirm-Sensors $url $(if ($DryRun) { 2 } else { 30 }))
    }
    Write-FirewallNote $port
}

Write-Host "Installing hayami $Version from $Root"

if ($DryRun) {
    if (-not $SkipBuild) { Write-Note "would build hayami.exe and hayami-tui.exe" }
} elseif (-not $SkipBuild) {
    Build
}

. (Join-Path $PSScriptRoot "install_binary.ps1")

Write-Step "Installing to $Destination"
foreach ($name in "hayami.exe", "hayami-tui.exe") {
    $from = Join-Path $Root $name
    $to = Join-Path $Destination $name
    if ($DryRun) { Write-Note "would copy: $from -> $to"; continue }
    if (-not (Test-Path $from)) { Fail "$from is not built. Run without -SkipBuild." }
    New-Item -ItemType Directory -Force $Destination | Out-Null
    Write-Ok (Install-Binary $from $to)
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
