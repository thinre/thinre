# Configuration validate hook: receives the staged bundle directory and
# accepts or rejects the complete revision before anything goes live.
#
# Failure injection for tests: create <app>\fail-validate to reject every
# revision until the marker is removed.
$ErrorActionPreference = "Stop"

$AppDir = Split-Path -Parent $PSScriptRoot
$Staged = $args[0]
if (-not $Staged) { Write-Error "staged bundle directory expected"; exit 1 }

if (Test-Path (Join-Path $AppDir "fail-validate")) {
    [Console]::Error.WriteLine("validation failing on request (fail-validate marker present)")
    exit 1
}
if (-not (Test-Path $Staged) -or -not (Get-ChildItem $Staged)) {
    [Console]::Error.WriteLine("staged bundle is missing or empty")
    exit 1
}
exit 0
