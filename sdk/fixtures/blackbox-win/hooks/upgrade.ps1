# Upgrade hook: install the artifact zip as the new application.
# Usage: upgrade.ps1 <artifact-path>
#
# Failure injection for tests: create <app>\fail-upgrade to make the next
# upgrade fail before touching anything; <app>\slow-upgrade stretches the
# upgrade so a test can kill the supervisor mid-operation.
$ErrorActionPreference = "Stop"

$AppDir = Split-Path -Parent $PSScriptRoot
$Artifact = $args[0]
if (-not $Artifact) { Write-Error "usage: upgrade.ps1 <artifact-path>"; exit 1 }

if (Test-Path (Join-Path $AppDir "fail-upgrade")) {
    [Console]::Error.WriteLine("upgrade: failing on request (fail-upgrade marker present)")
    exit 1
}
if (Test-Path (Join-Path $AppDir "slow-upgrade")) {
    Start-Sleep -Seconds 10
}

# Keep the previous version as the rollback candidate. Hooks and config
# are part of the installation, not the versioned app payload, so only
# VERSION and payload are replaced.
$Previous = Join-Path $AppDir "previous"
if (Test-Path $Previous) { Remove-Item -Recurse -Force $Previous }
New-Item -ItemType Directory -Path $Previous | Out-Null
foreach ($f in "VERSION", "payload") {
    $src = Join-Path $AppDir $f
    if (Test-Path $src) { Copy-Item $src (Join-Path $Previous $f) }
}

Expand-Archive -Path $Artifact -DestinationPath $AppDir -Force
Write-Output ("upgraded to " + (Get-Content (Join-Path $AppDir "VERSION") -TotalCount 1))
