# Install-Binary copies one program into place, whether or not it is running,
# and returns what it did, for the installer to print. install_windows.ps1
# dot-sources it; the structure tests call it against a running program.
#
# A running program holds its file open and Windows will not overwrite it, but
# it will rename it: the running copy moves aside to <name>.old and the new one
# takes its name. What is running keeps the old version until it is restarted;
# the next install removes the old file once nothing holds it. The panel and
# the terminal pane are meant to stay open, so refusing to install while
# either runs failed in the ordinary case.
function Install-Binary($from, $to) {
    foreach ($stale in Get-ChildItem -Path "$to.old*" -ErrorAction SilentlyContinue) {
        Remove-Item -Force $stale.FullName -ErrorAction SilentlyContinue
    }
    try {
        Copy-Item -Force $from $to -ErrorAction Stop
        return $to
    } catch [System.IO.IOException] {
        if (-not (Test-Path $to)) { throw }
    }
    $aside = "$to.old"
    if (Test-Path $aside) { $aside = "$to.old-$([DateTime]::Now.ToString('yyyyMMddHHmmss'))" }
    Move-Item -Force $to $aside -ErrorAction Stop
    Copy-Item -Force $from $to -ErrorAction Stop
    return "$to (the running copy was moved aside to $(Split-Path -Leaf $aside); restart it for this version)"
}
