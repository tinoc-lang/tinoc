#!/usr/bin/env pwsh
<#
.SYNOPSIS
    Tinoc installer.
.DESCRIPTION
    Fetches the latest release from GitHub and installs it into $HOME\.tinoc
    (override with TINOC_HOME or -Dir). Run './install.ps1 -Help' for flags.

    The installer is a companion to build.ps1/build.sh: pass -Local to build
    from source with ./build.ps1 build instead of downloading a release.
#>

[CmdletBinding()]
param(
    [switch]$Local,
    [switch]$Force,
    [switch]$Yes,
    [switch]$Check,
    [switch]$Uninstall,
    [switch]$Verbose,
    [switch]$Help,
    [string]$Version,
    [string]$Dir,
    [string]$Repo
)

$ErrorActionPreference = "Stop"

$script:TinocRepo = if ($Repo) { $Repo } elseif ($env:TINOC_REPO) { $env:TINOC_REPO } else { "tinoc-lang/tinoc" }
$script:ApiBase = if ($env:TINOC_API_BASE) { $env:TINOC_API_BASE } else { "https://api.github.com" }
$script:DownloadBase = if ($env:TINOC_DOWNLOAD_BASE) { $env:TINOC_DOWNLOAD_BASE } else { "https://github.com" }
$script:TinocHome = if ($Dir) { $Dir } elseif ($env:TINOC_HOME) { $env:TINOC_HOME } else { Join-Path $HOME ".tinoc" }
$script:SpecVersion = $Version
$script:Force = $Force -or $Yes

$UseColor = -not $env:NO_COLOR

function Write-Info ($msg) { if ($UseColor) { Write-Host "==> " -NoNewline -ForegroundColor Blue; Write-Host $msg -ForegroundColor White } else { Write-Host "==> $msg" } }
function Write-Step ($msg) { if ($UseColor) { Write-Host "  -> " -NoNewline -ForegroundColor Cyan } else { Write-Host -NoNewline "  -> " }; Write-Host $msg }
function Write-Ok   ($msg) { if ($UseColor) { Write-Host "OK   " -NoNewline -ForegroundColor Green } else { Write-Host -NoNewline "OK   " }; Write-Host $msg }
function Write-Warn ($msg) { if ($UseColor) { Write-Host "WARN " -NoNewline -ForegroundColor Yellow } else { Write-Host -NoNewline "WARN " }; Write-Host $msg }
function Write-ErrLine ($msg) { if ($UseColor) { Write-Host "FAIL " -NoNewline -ForegroundColor Red } else { Write-Host -NoNewline "FAIL " }; Write-Host $msg }
function Write-Kv   ($k, $v) { if ($UseColor) { Write-Host ("  {0,-10}" -f $k) -NoNewline -ForegroundColor DarkGray } else { Write-Host -NoNewline ("  {0,-10}" -f $k) }; Write-Host $v }

function Get-TargetOS {
    if ($env:GOOS) { return $env:GOOS }
    return "windows"
}

function Get-TargetArch {
    if ($env:GOARCH) { return $env:GOARCH }
    $arch = $env:PROCESSOR_ARCHITECTURE
    if ($arch -eq "ARM64") { return "arm64" }
    if ([Environment]::Is64BitOperatingSystem) { return "amd64" }
    return "386"
}

function Get-BinaryName { return "tinoc.exe" } # PowerShell flow targets Windows

function Get-AssetName($os, $arch) {
    if ($os -eq "windows") { return "tinoc-$os-$arch.zip" }
    return "tinoc-$os-$arch.tar.gz"
}

function Test-SupportedTarget($os, $arch) {
    return (@("linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64", "windows/amd64") -contains "$os/$arch")
}

function Format-Version($v) { return $v.TrimStart("v", "V") }

function Get-InstalledVersion {
    $file = Join-Path $script:TinocHome "VERSION"
    if (Test-Path $file) { return (Get-Content $file -Raw).Trim() }
    return ""
}

function Invoke-DownloadFile($url, $dest) {
    try {
        Invoke-WebRequest -UseBasicParsing -Uri $url -OutFile $dest
        return $true
    } catch {
        return $false
    }
}

function Invoke-FetchLatestTag {
    $url = "$($script:ApiBase.TrimEnd('/'))/repos/$($script:TinocRepo)/releases/latest"
    try {
        $json = Invoke-RestMethod -Uri $url -Headers @{ "User-Agent" = "tinoc-installer" }
    } catch {
        Write-ErrLine "Could not fetch the latest release from GitHub"
        Write-ErrLine "  $url"
        Write-ErrLine "Check your network connection, or install a specific version with -Version"
        exit 1
    }
    if (-not $json.tag_name) {
        Write-ErrLine "GitHub returned no release for $($script:TinocRepo) - no releases published yet?"
        Write-ErrLine "Build from source instead: ./install.ps1 -Local"
        exit 1
    }
    return $json.tag_name
}

function Get-Sha256($path) {
    return (Get-FileHash -Algorithm SHA256 -Path $path).Hash.ToLower()
}

# Confirmation prompt; -Force/-Yes skips it.
function Confirm-Tinoc($msg) {
    if ($script:Force) { return $true }
    $yn = Read-Host "$msg [y/N]"
    return ($yn -match '^(y|yes)$')
}

function Update-PathMaybe {
    $binDir = Join-Path $script:TinocHome "bin"
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($userPath -and ($userPath -split ';' -contains $binDir)) { return }

    Write-Step "$binDir is not on your PATH"
    if (Confirm-Tinoc "Add '$binDir' to your user PATH?") {
        try {
            $newPath = if ($userPath) { "$userPath;$binDir" } else { $binDir }
            [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
            Write-Ok "PATH updated - open a new terminal to use tinoc"
        } catch {
            Write-Warn "Could not update PATH automatically"
            Write-Info "Add it manually: setx PATH \"$binDir;%PATH%\""
        }
    } else {
        Write-Info "Add it manually: setx PATH \"$binDir;%PATH%\""
    }
}

# Shared install step: move $src to $TinocHome\bin\tinoc.exe, write VERSION,
# then verify it runs and offer PATH setup. $cleanupDir (optional) is removed
# on the early-exit paths (already installed / aborted).
function Install-TinocBinary($src, $version, $cleanupDir) {
    $binDir = Join-Path $script:TinocHome "bin"
    New-Item -ItemType Directory -Force -Path $binDir | Out-Null

    $prev = Get-InstalledVersion
    if ($prev) {
        if ($prev -eq $version) {
            if ($cleanupDir) { Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $cleanupDir }
            Write-Ok "tinoc v$version is already installed ($binDir\tinoc.exe)"
            Write-Info "Nothing to do - run './install.ps1 -Force' to reinstall"
            exit 0
        }
        Write-Warn "New version available: v$version (installed v$prev)"
        if (-not (Confirm-Tinoc "Update tinoc from v$prev to v$version?")) {
            if ($cleanupDir) { Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $cleanupDir }
            Write-Info "Aborted - keeping v$prev"
            exit 0
        }
    }

    Write-Info "Installing tinoc v$version"
    $tmp = Join-Path $script:TinocHome (".install.{0}" -f [guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Force -Path $tmp | Out-Null

    try {
        Copy-Item -Path $src -Destination (Join-Path $tmp "tinoc.exe") -Force
        Move-Item -Path (Join-Path $tmp "tinoc.exe") -Destination (Join-Path $binDir "tinoc.exe") -Force
        Set-Content -Path (Join-Path $script:TinocHome "VERSION") -Value $version -NoNewline
    } catch {
        Write-ErrLine "Failed to install binary to $binDir\tinoc.exe"
        exit 1
    } finally {
        Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $tmp
    }

    $installed = Join-Path $binDir "tinoc.exe"
    & $installed version *> $null
    if ($LASTEXITCODE -eq 0) {
        Write-Ok "Installed tinoc v$version -> $installed"
    } else {
        Write-ErrLine "Installed binary failed to run - the release may be corrupt"
        exit 1
    }
    Update-PathMaybe
}

function Invoke-Remote {
    $os = Get-TargetOS
    $arch = Get-TargetArch
    if (-not (Test-SupportedTarget $os $arch)) {
        Write-ErrLine "No prebuilt release for $os/$arch"
        Write-ErrLine "Supported: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64"
        Write-ErrLine "Build from source instead: ./install.ps1 -Local"
        exit 1
    }

    $tag = ""
    $version = ""
    if ($script:SpecVersion) {
        $version = Format-Version $script:SpecVersion
        $tag = "v$version"
    } else {
        $tag = Invoke-FetchLatestTag
        $version = Format-Version $tag
    }

    $asset = Get-AssetName $os $arch
    $base = "$($script:DownloadBase.TrimEnd('/'))/$($script:TinocRepo)/releases/download/$tag"

    Write-Info "Fetching tinoc v$version from GitHub"
    Write-Kv "Version" "v$version"
    Write-Kv "Platform" "$os/$arch"
    Write-Kv "Install"  $script:TinocHome
    Write-Kv "Asset"    "$base/$asset"

    New-Item -ItemType Directory -Force -Path $script:TinocHome | Out-Null
    $tmp = Join-Path $script:TinocHome (".install.{0}" -f [guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Force -Path $tmp | Out-Null

    Write-Step "Downloading $asset"
    if (-not (Invoke-DownloadFile "$base/$asset" (Join-Path $tmp "archive"))) {
        Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $tmp
        Write-ErrLine "Download failed - does release $tag contain '$asset'?"
        exit 1
    }

    $shaUrl = "$base/SHA256SUMS"
    if (Invoke-DownloadFile $shaUrl (Join-Path $tmp "sha")) {
        $shaLine = Get-Content (Join-Path $tmp "sha") |
            Where-Object { ($_.Trim() -split '\s+')[1] -eq $asset } |
            Select-Object -First 1
        $want = if ($shaLine) { ($shaLine.Trim() -split '\s+')[0] } else { "" }
        $got = Get-Sha256 (Join-Path $tmp "archive")
        if ($want -and ($want.ToLower() -eq $got)) {
            Write-Ok "Checksum verified (sha256)"
        } else {
            Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $tmp
            Write-ErrLine "Checksum mismatch for $asset"
            Write-ErrLine "  expected: $want"
            Write-ErrLine "  actual:   $got"
            exit 1
        }
    } else {
        Write-Warn "No SHA256SUMS manifest on this release - skipping checksum verification"
    }

    Write-Step "Extracting $asset"
    $x = Join-Path $tmp "x"
    New-Item -ItemType Directory -Force -Path $x | Out-Null
    if ($asset -like "*.zip") {
        Expand-Archive -Path (Join-Path $tmp "archive") -DestinationPath $x
    } else {
        tar -xzf (Join-Path $tmp "archive") -C $x
        if ($LASTEXITCODE -ne 0) {
            Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $tmp
            Write-ErrLine "Extraction failed - the downloaded archive may be corrupt"
            exit 1
        }
    }

    $bin = Get-ChildItem -Path $x -File | Where-Object { $_.Name -like "tinoc-*" } | Select-Object -First 1
    if (-not $bin) {
        $bin = Get-ChildItem -Path $x -Recurse -File | Where-Object { $_.Name -like "tinoc*" } | Select-Object -First 1
    }
    if (-not $bin) {
        Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $tmp
        Write-ErrLine "No tinoc binary found inside $asset"
        exit 1
    }

    Install-TinocBinary $bin.FullName $version $tmp
    Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $tmp
}

function Invoke-Local {
    if (-not (Test-Path "build.ps1") -and -not (Test-Path "build.sh")) {
        Write-ErrLine "No build.ps1/build.sh found in $(Get-Location)"
        Write-ErrLine "Run './install.ps1 -Local' from the repository root"
        exit 1
    }

    Write-Info "Building tinoc from source"
    if (Test-Path "build.ps1") {
        ./build.ps1 build
    } else {
        Write-Warn "Windows: run './install.ps1 -Local' for the PowerShell flow"
        ./build.sh build
    }

    $bin = Join-Path "build" "tinoc.exe"
    if (-not (Test-Path $bin)) {
        Write-ErrLine "Build did not produce $bin"
        exit 1
    }

    $version = ((& $bin version 2>$null) -split " ")[2]
    if (-not $version) { $version = "dev" }
    Install-TinocBinary $bin $version
}

function Invoke-Check {
    $installed = Get-InstalledVersion
    if (-not $installed) {
        Write-Info "tinoc is not installed ($($script:TinocHome))"
        Write-Info "Install the latest release with: ./install.ps1"
        return
    }
    Write-Info "Checking for updates"
    Write-Kv "Installed" "v$installed"

    $latest = Format-Version (Invoke-FetchLatestTag)
    Write-Kv "Latest" "v$latest"

    if ($installed -eq $latest) {
        Write-Ok "tinoc is up to date (v$latest)"
    } else {
        Write-Warn "Update available: v$latest (installed v$installed)"
        Write-Info "Run './install.ps1' to update"
    }
}

function Invoke-Uninstall {
    if (-not (Test-Path $script:TinocHome)) {
        Write-Info "Nothing to uninstall - $($script:TinocHome) does not exist"
        return
    }
    Write-Info "This will remove $($script:TinocHome) (binaries, VERSION, and data)"
    if (Confirm-Tinoc "Remove tinoc from $($script:TinocHome)?") {
        Remove-Item -Recurse -Force $script:TinocHome
        Write-Ok "Uninstalled tinoc ($($script:TinocHome))"
        Write-Info "Remove the PATH entry from your user environment if you added it"
    } else {
        Write-Info "Aborted - nothing was removed"
    }
}

function Show-Help {
    Write-Host "Tinoc installer" -ForegroundColor White
    Write-Host ""
    Write-Host "Usage:"
    Write-Host "  ./install.ps1 [flags]"
    Write-Host ""
    Write-Host "Flags:"
    Write-Host "  -Local         Build with ./build.ps1 build and install the local binary"
    Write-Host "  -Check         Compare installed version with the latest release, make no changes"
    Write-Host "  -Uninstall     Remove the entire TINOC_HOME install"
    Write-Host "  -Version       Install a specific release version (e.g. -Version 0.1.0)"
    Write-Host "  -Force, -Yes   Skip all confirmation prompts"
    Write-Host "  -Dir           Override install directory (default: $HOME\.tinoc)"
    Write-Host "  -Repo          Override the GitHub repository (default: $($script:TinocRepo))"
    Write-Host "  -Verbose       Print the underlying commands being run"
    Write-Host "  -Help          Show this message"
    Write-Host ""
    Write-Host "Environment:"
    Write-Host "  TINOC_HOME       Install directory (default: $HOME\.tinoc)"
    Write-Host "  TINOC_REPO       GitHub repository (default: tinoc-lang/tinoc)"
    Write-Host "  TINOC_API_BASE   GitHub API base URL (default: https://api.github.com)"
    Write-Host "  NO_COLOR         Disable colored output"
    Write-Host ""
    Write-Host "Examples:"
    Write-Host "  ./install.ps1"
    Write-Host "  ./install.ps1 -Check"
    Write-Host "  ./install.ps1 -Local"
    Write-Host "  ./install.ps1 -Version 0.1.0 -Yes"
    Write-Host "  ./install.ps1 -Uninstall"
}

if ($Help) { Show-Help; exit 0 }

if ($Uninstall) {
    Invoke-Uninstall
} elseif ($Check) {
    Invoke-Check
} elseif ($Local) {
    if ($script:SpecVersion) {
        Write-ErrLine "-Version cannot be combined with -Local"
        exit 1
    }
    Invoke-Local
} else {
    Invoke-Remote
}
