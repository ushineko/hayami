<#
.SYNOPSIS
    Make the Windows resources hayami's executables carry: the icon and the
    version that Explorer, the Start menu and a file's Properties show.

.DESCRIPTION
    Draws the icon from internal/gui/assets/hayami.svg (tools/appicon) and has
    go-winres, run with `go run` at the version pinned below and installed
    nowhere, write cmd/hayami/rsrc_windows_amd64.syso and
    cmd/hayami-tui/rsrc_windows_amd64.syso. `go build` links a .syso in the
    package's directory into a Windows binary by itself. The .syso files are
    build output: .gitignore keeps them out of the repository (spec 053).

    install_windows.ps1 runs it before it builds, and so does CI's Windows job.

.PARAMETER Version
    The version written into the resources; VERSION's by default.

.PARAMETER DryRun
    Say what would be made and make nothing.
#>
param(
    [string] $Version,
    [switch] $DryRun
)

$ErrorActionPreference = "Stop"

# The one place go-winres's version is named. Pinned, not @latest: a resource
# tool that changed under the build would change every executable it touches.
$WinresVersion = "v0.3.3"

$Root = Split-Path -Parent $PSScriptRoot
if (-not $Version) { $Version = (Get-Content (Join-Path $Root "VERSION") -Raw).Trim() }

$targets = @(
    @{ Package = "cmd/hayami"; File = "hayami.exe"; Description = "A glance panel" },
    @{ Package = "cmd/hayami-tui"; File = "hayami-tui.exe"; Description = "A glance panel, in a terminal" }
)

if ($DryRun) {
    foreach ($t in $targets) {
        Write-Host "       would write $($t.Package)/rsrc_windows_amd64.syso: the icon and version $Version (go-winres $WinresVersion)"
    }
    return
}

$work = Join-Path ([IO.Path]::GetTempPath()) ("hayami-winres-" + [guid]::NewGuid().ToString("N").Substring(0, 8))
New-Item -ItemType Directory -Force $work | Out-Null
try {
    Push-Location $Root
    $png = Join-Path $work "hayami.png"
    & go run ./tools/appicon -size 256 -out $png
    if ($LASTEXITCODE -ne 0) { throw "drawing the icon failed" }

    foreach ($t in $targets) {
        $out = Join-Path $t.Package "rsrc"
        # --manifest none: the panel's DPI awareness and the rest are GLFW's to
        # set at run time; a manifest would set them a second time.
        & go run "github.com/tc-hib/go-winres@$WinresVersion" simply --arch amd64 --out $out --manifest none `
            --icon $png --product-name hayami --file-description $t.Description `
            --product-version $Version --file-version $Version --original-filename $t.File
        if ($LASTEXITCODE -ne 0) { throw "go-winres failed for $($t.Package)" }
    }
} finally {
    Pop-Location
    Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
}
