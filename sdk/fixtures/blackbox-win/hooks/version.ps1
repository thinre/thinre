# Version hook: print the installed version to stdout.
$ErrorActionPreference = "Stop"
$AppDir = Split-Path -Parent $PSScriptRoot
Get-Content (Join-Path $AppDir "VERSION") -TotalCount 1
