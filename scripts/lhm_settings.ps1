<#
.SYNOPSIS
    Make LibreHardwareMonitor's settings file say what hayami needs.

.DESCRIPTION
    install_windows.ps1 -WithSensors runs this (spec 042); it is a file of its
    own so that the elevated step and the tests run the same code.

    LibreHardwareMonitor keeps its settings in an XML file beside its
    executable and rewrites the whole file when it exits, so this is run while
    it is stopped. It sets the keys below and no others; every other key, and
    every value it does not need, is left as it is. An existing file is copied
    once, to <file>.bak-hayami, before the first change; a backup already there
    is never overwritten. Run again with nothing to change, it writes nothing.

    Keys set:
      runWebServerMenuItem   true   the web server hayami reads data.json from
      listenerPort           -Port  8085 unless hayami's lhm setting says otherwise
      authenticationEnabled  false  hayami sends no password
      startMinMenuItem       true   start with no window
      minCloseMenuItem       true   closing the window hides it, rather than quitting
      minTrayMenuItem        true   hidden means the notification area

.PARAMETER Path
    The settings file, LibreHardwareMonitor.config beside the executable.

.PARAMETER Port
    The web server's port.

.PARAMETER DryRun
    Say what would change and write nothing.

.OUTPUTS
    One line per key it changes ("set key=value" or "would set key=value"),
    "unchanged" when there is nothing to do, and a last line "changes: N".
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)] [string] $Path,
    [int] $Port = 8085,
    [switch] $DryRun
)

$ErrorActionPreference = "Stop"

$required = [ordered]@{
    runWebServerMenuItem  = "true"
    listenerPort          = "$Port"
    authenticationEnabled = "false"
    startMinMenuItem      = "true"
    minCloseMenuItem      = "true"
    minTrayMenuItem       = "true"
}

$doc = New-Object Xml.XmlDocument
$doc.PreserveWhitespace = $true
$exists = Test-Path $Path
if ($exists) {
    $doc.Load($Path)
} else {
    $doc.LoadXml("<?xml version=""1.0"" encoding=""utf-8""?>`r`n<configuration>`r`n  <appSettings>`r`n  </appSettings>`r`n</configuration>`r`n")
}
$settings = $doc.SelectSingleNode("/configuration/appSettings")
if (-not $settings) {
    $configuration = $doc.SelectSingleNode("/configuration")
    if (-not $configuration) { throw "$Path is not a LibreHardwareMonitor settings file: it has no <configuration>." }
    $settings = $configuration.AppendChild($doc.CreateElement("appSettings"))
}

$changes = 0
foreach ($key in $required.Keys) {
    $want = $required[$key]
    $node = $settings.SelectSingleNode("add[@key='$key']")
    if ($node -and $node.GetAttribute("value") -eq $want) { continue }
    $changes++
    if ($DryRun) { Write-Output "would set $key=$want"; continue }
    if (-not $node) {
        $node = $doc.CreateElement("add")
        $node.SetAttribute("key", $key)
        [void]$settings.AppendChild($doc.CreateWhitespace("  "))
        [void]$settings.AppendChild($node)
        [void]$settings.AppendChild($doc.CreateWhitespace("`r`n  "))
    }
    $node.SetAttribute("value", $want)
    Write-Output "set $key=$want"
}

if ($changes -eq 0) {
    Write-Output "unchanged"
} elseif (-not $DryRun) {
    $backup = "$Path.bak-hayami"
    if ($exists -and -not (Test-Path $backup)) {
        Copy-Item $Path $backup
        Write-Output "backup: $backup"
    }
    $writer = New-Object IO.StreamWriter($Path, $false, (New-Object Text.UTF8Encoding $true))
    try { $doc.Save($writer) } finally { $writer.Dispose() }
}
Write-Output "changes: $changes"
