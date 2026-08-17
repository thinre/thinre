# Health hook: exit 0 when healthy, non-zero otherwise.
#
# Failure injection for tests: create <app>\unhealthy to make the
# application report unhealthy until the marker is removed.
$ErrorActionPreference = "Stop"

$AppDir = Split-Path -Parent $PSScriptRoot

if (Test-Path (Join-Path $AppDir "unhealthy")) {
    [Console]::Error.WriteLine("unhealthy (marker present)")
    exit 1
}
$VersionFile = Join-Path $AppDir "VERSION"
if (-not (Test-Path $VersionFile)) {
    [Console]::Error.WriteLine("unhealthy: VERSION missing")
    exit 1
}
Write-Output ("healthy " + (Get-Content $VersionFile -TotalCount 1))
