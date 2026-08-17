# Rollback hook: restore the version saved by the last upgrade.
$ErrorActionPreference = "Stop"

$AppDir = Split-Path -Parent $PSScriptRoot
$Previous = Join-Path $AppDir "previous"

if (-not (Test-Path $Previous)) {
    [Console]::Error.WriteLine("rollback: no previous version available")
    exit 1
}
foreach ($f in "VERSION", "payload") {
    $src = Join-Path $Previous $f
    if (Test-Path $src) { Copy-Item $src (Join-Path $AppDir $f) -Force }
}
Write-Output ("rolled back to " + (Get-Content (Join-Path $AppDir "VERSION") -TotalCount 1))
