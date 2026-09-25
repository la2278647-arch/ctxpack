# install.ps1 — download the right ctxpack binary for Windows from the latest
# GitHub release, verify it against the published SHA256SUMS.txt, and put it in
# a directory on PATH.
#
# Usage (PowerShell):
#   irm https://raw.githubusercontent.com/la2278647-arch/ctxpack/main/install.ps1 | iex
#
# Pin a version:
#   & ([scriptblock]::Create((irm https://raw.githubusercontent.com/la2278647-arch/ctxpack/main/install.ps1))) 'v0.1.7'
#
# Environment:
#   CTXPACK_INSTALL_DIR  override the install directory (default:
#                        %LOCALAPPDATA%\Microsoft\WindowsApps, which is on PATH).
#
<#
.SYNOPSIS
    Installs ctxpack from the GitHub releases.
.PARAMETER Version
    Optional version tag to install (e.g. v0.1.7). Defaults to the latest.
#>
param([string]$Version = "")

$ErrorActionPreference = 'Stop'
$Owner = 'la2278647-arch'
$Repo = 'ctxpack'
$Headers = @{ 'User-Agent' = 'ctxpack-installer' }

if (-not $Version) {
    # The REST API is precise but is rate-limited at 60 anonymous requests per
    # hour, which turns a routine install into a 403 on a busy machine. The
    # releases/latest page 302-redirects to /releases/tag/vX.Y.Z instead and is
    # not rate-limited at all, so it is the fallback.
    Write-Host 'Detecting latest release…' -ForegroundColor Cyan
    try {
        $rel = Invoke-RestMethod -Uri "https://api.github.com/repos/$Owner/$Repo/releases/latest" -Headers $Headers
        if ($rel.tag_name) { $Version = $rel.tag_name }
    } catch {
        Write-Host "  (REST API unavailable, trying the redirect: $($_.Exception.Message))" -ForegroundColor DarkYellow
    }
    if (-not $Version) {
        # /releases/latest 302-redirects to /releases/tag/vX.Y.Z and is not
        # rate-limited. Invoke-WebRequest with -MaximumRedirection 0 does throw,
        # but its Headers['Location'] comes back empty, so the raw response is
        # read directly.
        $Loc = $null
        try {
            $req = [System.Net.HttpWebRequest]::Create("https://github.com/$Owner/$Repo/releases/latest")
            $req.UserAgent = 'ctxpack-installer'
            $req.AllowAutoRedirect = $false
            $resp = $req.GetResponse()
            $Loc = $resp.Headers['Location']
            $resp.Close()
        } catch {
            if ($_.Exception.Response) {
                $Loc = $_.Exception.Response.Headers['Location']
                $_.Exception.Response.Close()
            }
        }
        if ($Loc -match '/releases/tag/([^/]+)') {
            $Version = $Matches[1]
        }
    }
    if (-not $Version) {
        Write-Error "ctxpack: could not determine the latest release."
        exit 1
    }
}
$Ver = $Version.TrimStart('v')

switch ($env:PROCESSOR_ARCHITECTURE) {
    'ARM64' { $Arch = 'arm64' }
    'AMD64' { $Arch = 'amd64' }
    'x86'   { $Arch = '386' }
    default {
        Write-Error "ctxpack: unsupported processor architecture: $env:PROCESSOR_ARCHITECTURE"
        exit 1
    }
}

$Asset = "ctxpack_$($Ver)_windows_$($Arch).exe"
$Base = "https://github.com/$Owner/$Repo/releases/download/v$Ver"
$Url = "$Base/$Asset"
$SumsUrl = "$Base/SHA256SUMS.txt"

$InstallDir = $env:CTXPACK_INSTALL_DIR
if (-not $InstallDir) { $InstallDir = Join-Path $env:LOCALAPPDATA 'Microsoft\WindowsApps' }
if (-not (Test-Path $InstallDir)) { New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null }

$Dest = Join-Path $InstallDir 'ctxpack.exe'
# Download to a temp path and move into place only after the checksum verifies,
# so a failed download cannot leave a truncated binary on PATH.
$Tmp = Join-Path $env:TEMP "ctxpack-$Ver-$PID.exe"
$SumsTmp = Join-Path $env:TEMP "ctxpack-$Ver-$PID.sha256sums"
try {
    Write-Host "Downloading $Url" -ForegroundColor Cyan
    Invoke-WebRequest -Uri $Url -OutFile $Tmp -Headers $Headers -UseBasicParsing
    Invoke-WebRequest -Uri $SumsUrl -OutFile $SumsTmp -Headers $Headers -UseBasicParsing

    # The two spaces are part of the match, so the asset cannot match a
    # filename that merely has it as a prefix.
    $Line = Get-Content $SumsTmp | Where-Object { $_ -like "*  $Asset" }
    if (-not $Line) {
        Write-Error "ctxpack: $Asset is not listed in SHA256SUMS.txt"
        exit 1
    }
    $Want = (($Line -split '\s+', 2))[0].ToLower()
    $Got = (Get-FileHash $Tmp -Algorithm SHA256).Hash.ToLower()
    if ($Got -ne $Want) {
        Write-Error "ctxpack: SHA256 mismatch for $Asset"
        Write-Error "  expected $Want"
        Write-Error "  got      $Got"
        exit 1
    }
    Write-Host "SHA256 verified: $Got" -ForegroundColor DarkGreen

    Move-Item -Force $Tmp $Dest
} finally {
    Remove-Item $Tmp, $SumsTmp -ErrorAction SilentlyContinue
}

Write-Host "Installed ctxpack v$Ver -> $Dest" -ForegroundColor Green
& $Dest version
