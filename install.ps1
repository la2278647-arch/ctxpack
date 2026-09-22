# install.ps1 — download the right ctxpack binary for Windows from the latest
# GitHub release and put it in a directory on PATH.
#
# Usage (PowerShell):
#   irm https://raw.githubusercontent.com/la2278647-arch/ctxpack/main/install.ps1 | iex
#
# Pin a version:
#   & ([scriptblock]::Create((irm https://raw.githubusercontent.com/la2278647-arch/ctxpack/main/install.ps1))) 'v0.1.2'

<#
.SYNOPSIS
    Installs ctxpack from the GitHub releases.
.PARAMETER Version
    Optional version tag to install (e.g. v0.1.2). Defaults to the latest.
#>
param([string]$Version = "")

$ErrorActionPreference = 'Stop'
$Owner = 'la2278647-arch'
$Repo = 'ctxpack'

if (-not $Version) {
    Write-Host 'Detecting latest release…' -ForegroundColor Cyan
    $rel = Invoke-RestMethod -Uri "https://api.github.com/repos/$Owner/$Repo/releases/latest" -Headers @{ 'User-Agent' = 'ctxpack-installer' }
    $Version = $rel.tag_name
}
$Ver = $Version.TrimStart('v')

$Arch = 'amd64'
switch ($env:PROCESSOR_ARCHITECTURE) {
    'ARM64' { $Arch = 'arm64' }
    'AMD64' { $Arch = 'amd64' }
    'x86'   { $Arch = '386' }
}

$Asset = "ctxpack_$($Ver)_windows_$($Arch).exe"
$Url = "https://github.com/$Owner/$Repo/releases/download/v$Ver/$Asset"

$InstallDir = $env:CTXPACK_INSTALL_DIR
if (-not $InstallDir) { $InstallDir = Join-Path $env:LOCALAPPDATA 'Microsoft\WindowsApps' }
if (-not (Test-Path $InstallDir)) { New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null }

$Dest = Join-Path $InstallDir 'ctxpack.exe'
Write-Host "Downloading $Url" -ForegroundColor Cyan
Invoke-WebRequest -Uri $Url -OutFile $Dest -UseBasicParsing

Write-Host "Installed ctxpack v$Ver -> $Dest" -ForegroundColor Green
& $Dest version
