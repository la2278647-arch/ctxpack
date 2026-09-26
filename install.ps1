# install.ps1 — download the right ctxpack binary for Windows from the latest
# GitHub release, verify it against the published SHA256SUMS.txt, and put it in
# a directory on PATH.
#
# Usage (PowerShell):
#   irm https://raw.githubusercontent.com/la2278647-arch/ctxpack/main/install.ps1 | iex
#
# Pin a version:
#   & ([scriptblock]::Create((irm https://raw.githubusercontent.com/la2278647-arch/ctxpack/main/install.ps1))) 'v0.1.10'
#
# Environment:
#   CTXPACK_INSTALL_DIR  override the install directory (default:
#                        %LOCALAPPDATA%\Microsoft\WindowsApps, which is on PATH).
#
<#
.SYNOPSIS
    Installs ctxpack from the GitHub releases.
.PARAMETER Version
    Optional version tag to install (e.g. v0.1.10). Defaults to the latest.
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
# A reset connection or a transient 5xx from GitHub aborts an install, and
# Invoke-WebRequest has no retry of its own. Back off and try again a few times
# before giving up.
function DownloadWithRetry([string]$Uri, [string]$OutFile, [int]$Attempts = 4) {
    for ($i = 1; $i -le $Attempts; $i++) {
        try {
            Invoke-WebRequest -Uri $Uri -OutFile $OutFile -Headers $Headers -UseBasicParsing
            return
        } catch {
            if ($i -eq $Attempts) { throw }
            Write-Host "  retry $i/$Attempts after a failed download" -ForegroundColor DarkYellow
            Start-Sleep -Seconds (2 * $i)
        }
    }
}

try {
    Write-Host "Downloading $Url" -ForegroundColor Cyan
    DownloadWithRetry $Url $Tmp
    DownloadWithRetry $SumsUrl $SumsTmp

    # A sha256sum line is "<hash> <marker><name>", where the marker is "*" in
    # binary mode and a space in text mode. Which marker a host emits is
    # platform-dependent: GNU coreutils defaults to binary mode when given file
    # arguments, so a checksum file produced on one machine is not guaranteed to
    # parse on another. Accept both and compare the name after stripping the
    # marker.
    $Want = $null
    foreach ($Line in (Get-Content $SumsTmp)) {
        $parts = @($Line -split '\s+')
        if ($parts.Count -lt 2) { continue }
        if ($parts[$parts.Count - 1].TrimStart('*') -ceq $Asset) {
            $Want = $parts[0]
            break
        }
    }
    if (-not $Want) {
        Write-Error "ctxpack: $Asset is not listed in SHA256SUMS.txt"
        exit 1
    }
    $Want = $Want.ToLower()
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
