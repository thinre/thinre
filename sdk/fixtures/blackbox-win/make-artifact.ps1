# Build a blackbox-win release artifact: blackbox-win-<version>.zip
# containing VERSION and a payload file, plus its SHA-256.
# Usage: make-artifact.ps1 <version> [output-dir]
$ErrorActionPreference = "Stop"

$Version = $args[0]
if (-not $Version) { Write-Error "usage: make-artifact.ps1 <version> [output-dir]"; exit 1 }
$OutDir = if ($args[1]) { $args[1] } else { "." }

$Stage = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid())
New-Item -ItemType Directory -Path $Stage | Out-Null
try {
    Set-Content -Path (Join-Path $Stage "VERSION") -Value $Version -NoNewline
    Add-Content -Path (Join-Path $Stage "VERSION") -Value ""
    Set-Content -Path (Join-Path $Stage "payload") -Value "blackbox-win payload for version $Version"

    $Artifact = Join-Path $OutDir "blackbox-win-$Version.zip"
    if (Test-Path $Artifact) { Remove-Item $Artifact }
    Compress-Archive -Path (Join-Path $Stage "*") -DestinationPath $Artifact

    $Sha = (Get-FileHash -Algorithm SHA256 $Artifact).Hash.ToLower()
    Write-Output "artifact: $Artifact"
    Write-Output "sha256:   $Sha"
} finally {
    Remove-Item -Recurse -Force $Stage
}
