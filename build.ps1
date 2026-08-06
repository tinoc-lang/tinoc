#!/usr/bin/env pwsh
<#
.SYNOPSIS
    Tinoc build script.
.DESCRIPTION
    Usage: ./build.ps1 [command] [flags]
    Run './build.ps1 help' for the full command list.
#>

# Note: no [CmdletBinding()] and no [Parameter(...)] attributes here.
# Either one turns this script into an *advanced function*, which
# auto-injects the common -Verbose/-Debug parameters and collides with
# the explicit switches declared below ("A parameter with the name
# 'Verbose' was defined multiple times"). In a plain script the first
# parameter binds positionally anyway, so ./build.ps1 ci still works.
param(
    [ValidateSet("build", "build-all", "run", "test", "cover", "vet", "fmt", "fmt-check",
                 "lint", "deps", "check", "install", "clean", "ci", "version", "help")]
    [string]$Command = "build",

    [switch]$Debug,
    [switch]$Race,
    [switch]$Verbose,
    [switch]$Help
)

$ErrorActionPreference = "Stop"

$BuildDir    = "build"
$DistDir     = "dist"
$CoverDir    = "coverage"
$BinaryBase  = "tinoc"
$ModulePath  = "main.go"
$InstallDir  = if ($env:INSTALL_DIR) { $env:INSTALL_DIR } else { "$Env:ProgramFiles\tinoc" }
$TestTimeout = if ($env:TEST_TIMEOUT) { $env:TEST_TIMEOUT } else { "120s" }

function Get-GitValue([string[]]$GitArgs, [string]$Fallback) {
    try {
        $result = & git @GitArgs 2>$null
        if ($LASTEXITCODE -eq 0 -and $result) { return $result.Trim() }
    } catch {}
    return $Fallback
}

$Version   = if ($env:VERSION) { $env:VERSION } else { Get-GitValue @("describe", "--tags", "--always", "--dirty") "dev" }
$Commit    = Get-GitValue @("rev-parse", "--short", "HEAD") "unknown"
$BuildDate = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$GoVersion = try { ((& go version) -replace '^go version ', '').Trim() } catch { "unknown" }

$Mode      = if ($Debug)   { "debug" } else { "release" }
$UseRace   = [bool]$Race
$IsVerbose = [bool]$Verbose
$UseColor  = -not $env:NO_COLOR

function Write-Info ($msg) { if ($UseColor) { Write-Host "==> " -NoNewline -ForegroundColor Blue; Write-Host $msg -ForegroundColor White } else { Write-Host "==> $msg" } }
function Write-Step ($msg) { if ($UseColor) { Write-Host "  -> " -NoNewline -ForegroundColor Cyan } else { Write-Host -NoNewline "  -> " }; Write-Host $msg }
function Write-Ok   ($msg) { if ($UseColor) { Write-Host "OK   " -NoNewline -ForegroundColor Green } else { Write-Host -NoNewline "OK   " }; Write-Host $msg }
function Write-Warn ($msg) { if ($UseColor) { Write-Host "WARN " -NoNewline -ForegroundColor Yellow } else { Write-Host -NoNewline "WARN " }; Write-Host $msg }
function Write-ErrLine ($msg) { if ($UseColor) { Write-Host "FAIL " -NoNewline -ForegroundColor Red } else { Write-Host -NoNewline "FAIL " }; Write-Host $msg }
function Write-Kv   ($k, $v) { if ($UseColor) { Write-Host ("  {0,-10}" -f $k) -NoNewline -ForegroundColor DarkGray } else { Write-Host -NoNewline ("  {0,-10}" -f $k) }; Write-Host $v }

function Get-TargetOS   { if ($env:GOOS)   { return $env:GOOS }; return "windows" }
function Get-TargetArch {
    if ($env:GOARCH) { return $env:GOARCH }
    if ([Environment]::Is64BitOperatingSystem) { return "amd64" }
    return "386"
}
function Get-BinaryName($os) { if ($os -eq "windows") { return "$BinaryBase.exe" }; return $BinaryBase }

function Get-LdFlags($mode) {
    # Inject into src.Version (what `tinoc version` prints; the installers
    # record it in VERSION). Strip a leading "v" so the binary reports
    # "0.1.0". (The old -X main.version target never existed, so binaries
    # always reported the hardcoded 0.1.0.)
    $ver = $Version.TrimStart("v", "V")
    $flags = "-X github.com/tinoc-lang/tinoc/src.Version=$ver"
    if ($mode -eq "release") { $flags = "-s -w $flags" }
    return $flags
}

function Test-Go {
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
        Write-ErrLine "Go toolchain not found. Install it from https://go.dev/dl/"
        exit 1
    }
}

function Test-Module {
    if (-not (Test-Path $ModulePath) -and -not (Test-Path "go.mod")) {
        Write-ErrLine "No go.mod or $ModulePath found in $(Get-Location). Run this script from the project root."
        exit 1
    }
}

function Get-FileSizeHuman($path) {
    $bytes = (Get-Item $path).Length
    if ($bytes -gt 1MB) { return "{0:N1} MB" -f ($bytes / 1MB) }
    return "{0:N1} KB" -f ($bytes / 1KB)
}

function Invoke-Build {
    Test-Go
    Test-Module
    $os = Get-TargetOS
    $arch = Get-TargetArch
    New-Item -ItemType Directory -Force -Path $BuildDir | Out-Null
    $out = Join-Path $BuildDir (Get-BinaryName $os)

    Write-Info "Building tinoc"
    Write-Kv "Version" $Version
    Write-Kv "Commit"  $Commit
    Write-Kv "Target"  "$os/$arch"
    Write-Kv "Mode"    $Mode
    Write-Kv "Go"      $GoVersion
    Write-Kv "Output"  $out
    Write-Host ""

    $env:GOOS = $os
    $env:GOARCH = $arch
    $ldflags = Get-LdFlags $Mode

    $goArgs = @("build", "-trimpath", "-ldflags=$ldflags", "-o", $out, $ModulePath)
    if ($Mode -eq "debug") {
        $goArgs = @("build", "-trimpath", "-gcflags=all=-N -l", "-ldflags=$ldflags", "-o", $out, $ModulePath)
    }

    if ($IsVerbose) { Write-Step "GOOS=$os GOARCH=$arch go $($goArgs -join ' ')" }

    & go @goArgs
    if ($LASTEXITCODE -eq 0) {
        Write-Ok "Build succeeded"
        if (Test-Path $out) { Write-Kv "Size" (Get-FileSizeHuman $out) }
    } else {
        Write-ErrLine "Build failed"
        exit 1
    }
}

function Invoke-BuildAll {
    Test-Go
    Test-Module
    Write-Info "Cross-compiling tinoc for all platforms"
    New-Item -ItemType Directory -Force -Path $DistDir | Out-Null

    $targets = @(
        @{os="linux";   arch="amd64"}, @{os="linux";   arch="arm64"},
        @{os="darwin";  arch="amd64"}, @{os="darwin";  arch="arm64"},
        @{os="windows"; arch="amd64"}
    )
    $failures = 0

    foreach ($t in $targets) {
        $ext = if ($t.os -eq "windows") { ".exe" } else { "" }
        $out = Join-Path $DistDir "$BinaryBase-$($t.os)-$($t.arch)$ext"
        Write-Step "$($t.os)/$($t.arch)"

        $env:GOOS = $t.os
        $env:GOARCH = $t.arch
        & go build -trimpath -ldflags="$(Get-LdFlags 'release')" -o $out $ModulePath 2>$null

        if ($LASTEXITCODE -eq 0) {
            Write-Ok "  $out ($(Get-FileSizeHuman $out))"
        } else {
            Write-ErrLine "  $($t.os)/$($t.arch) failed"
            $failures++
        }
    }
    Write-Host ""
    if ($failures -eq 0) {
        Write-Ok "Cross-compile complete -> $DistDir/"
    } else {
        Write-ErrLine "$failures target(s) failed"
        exit 1
    }
}

function Invoke-Run {
    Test-Go
    Test-Module
    Write-Info "Running tinoc"
    & go run $ModulePath @args
}

function Invoke-Test {
    Test-Go
    Test-Module
    Write-Info "Running test suite"
    $testArgs = @("test", "-timeout", $TestTimeout, "./...")
    if ($UseRace)   { $testArgs = @("test", "-timeout", $TestTimeout, "-race", "./...") }
    if ($IsVerbose) { $testArgs += "-v" }

    & go @testArgs
    if ($LASTEXITCODE -eq 0) { Write-Ok "All tests passed" }
    else { Write-ErrLine "Tests failed"; exit 1 }
}

function Invoke-Cover {
    Test-Go
    Test-Module
    Write-Info "Running tests with coverage"
    New-Item -ItemType Directory -Force -Path $CoverDir | Out-Null
    $profile = Join-Path $CoverDir "coverage.out"

    & go test -timeout $TestTimeout -covermode=atomic -coverprofile=$profile ./...
    if ($LASTEXITCODE -eq 0) {
        Write-Ok "Coverage profile written -> $profile"
        & go tool cover -func=$profile | Select-Object -Last 1
        $htmlOut = Join-Path $CoverDir "coverage.html"
        & go tool cover -html=$profile -o $htmlOut
        Write-Ok "HTML report -> $htmlOut"
    } else {
        Write-ErrLine "Tests failed, no coverage report generated"
        exit 1
    }
}

function Invoke-Vet {
    Test-Go
    Test-Module
    Write-Info "Running go vet"
    & go vet ./...
    if ($LASTEXITCODE -eq 0) { Write-Ok "Vet passed" }
    else { Write-ErrLine "Vet found issues"; exit 1 }
}

function Invoke-Fmt {
    Test-Go
    Write-Info "Formatting source"
    $unformatted = & gofmt -l .
    if ($unformatted) {
        & gofmt -l -w .
        Write-Step "Reformatted:"
        $unformatted | ForEach-Object { Write-Host "    $_" }
    }
    Write-Ok "Formatting complete"
}

function Invoke-FmtCheck {
    Test-Go
    Write-Info "Checking formatting"
    $unformatted = & gofmt -l .
    if ($unformatted) {
        Write-ErrLine "The following files are not gofmt'd:"
        $unformatted | ForEach-Object { Write-Host "    $_" }
        Write-ErrLine "Run './build.ps1 fmt' to fix"
        exit 1
    }
    Write-Ok "All files formatted correctly"
}

function Invoke-Lint {
    if (Get-Command golangci-lint -ErrorAction SilentlyContinue) {
        Write-Info "Running golangci-lint"
        & golangci-lint run ./...
        if ($LASTEXITCODE -eq 0) { Write-Ok "Lint passed" }
        else { Write-ErrLine "Lint found issues"; exit 1 }
    } else {
        Write-Warn "golangci-lint not installed, skipping (see https://golangci-lint.run)"
    }
}

function Invoke-Deps {
    Test-Go
    Test-Module
    Write-Info "Verifying module dependencies"
    & go mod tidy
    & go mod verify
    Write-Ok "Dependencies tidy and verified"
}

function Invoke-Check {
    Write-Info "Running pre-commit checks"
    Invoke-FmtCheck
    Invoke-Vet
    Invoke-Lint
    Invoke-Test
    Write-Ok "All checks passed"
}

function Invoke-Install {
    Invoke-Build
    $os = Get-TargetOS
    $out = Join-Path $BuildDir (Get-BinaryName $os)
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    $dest = Join-Path $InstallDir (Get-BinaryName $os)

    Write-Info "Installing to $dest"
    try {
        Copy-Item -Path $out -Destination $dest -Force
        Write-Ok "Installed -> $dest"
    } catch {
        Write-ErrLine "Install failed (try running as Administrator, or set `$env:INSTALL_DIR)"
        exit 1
    }
}

function Invoke-Clean {
    Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $BuildDir, $DistDir, $CoverDir
    Write-Ok "Cleaned $BuildDir/, $DistDir/, and $CoverDir/"
}

function Invoke-CI {
    Write-Info "Running CI pipeline"
    Invoke-Deps
    Invoke-FmtCheck
    Invoke-Vet
    Invoke-Lint
    $script:UseRace = $true
    Invoke-Test
    $script:Mode = "release"
    Invoke-Build
    Write-Ok "CI pipeline complete"
}

function Invoke-Version {
    Write-Kv "Version" $Version
    Write-Kv "Commit"  $Commit
    Write-Kv "Built"   $BuildDate
    Write-Kv "Go"      $GoVersion
}

function Invoke-Help {
    Write-Host "Tinoc build script" -ForegroundColor White
    Write-Host ""
    Write-Host "Usage:"
    Write-Host "  ./build.ps1 [command] [flags]"
    Write-Host ""
    Write-Host "Commands:"
    Write-Host "  build         Build the tinoc binary for the current platform (default)"
    Write-Host "  build-all     Cross-compile for linux, darwin, windows (amd64/arm64) into dist/"
    Write-Host "  run           Build and run tinoc directly via 'go run'"
    Write-Host "  test          Run the test suite"
    Write-Host "  cover         Run tests with coverage and generate an HTML report"
    Write-Host "  vet           Run 'go vet'"
    Write-Host "  fmt           Format source with gofmt, in place"
    Write-Host "  fmt-check     Check formatting without modifying files (used in CI)"
    Write-Host "  lint          Run golangci-lint, if installed"
    Write-Host "  deps          Tidy and verify module dependencies"
    Write-Host "  check         Run fmt-check, vet, lint, and test (fast pre-commit gate)"
    Write-Host "  install       Build and install to `$env:INSTALL_DIR"
    Write-Host "  ci            Full pipeline: deps, fmt-check, vet, lint, race tests, build"
    Write-Host "  clean         Remove build/, dist/, and coverage/"
    Write-Host "  version       Print version, commit, and toolchain info"
    Write-Host "  help          Show this message"
    Write-Host ""
    Write-Host "Flags:"
    Write-Host "  -Debug        Build with debug symbols, skip optimization stripping"
    Write-Host "  -Race         Enable the race detector for 'test'"
    Write-Host "  -Verbose      Print the underlying commands being run"
    Write-Host ""
    Write-Host "Environment:"
    Write-Host "  GOOS, GOARCH   Override target platform/architecture"
    Write-Host "  VERSION        Override the injected version string"
    Write-Host "  INSTALL_DIR    Override install destination"
    Write-Host "  TEST_TIMEOUT   Override test timeout (default: 120s)"
    Write-Host "  NO_COLOR       Disable colored output"
    Write-Host ""
    Write-Host "Examples:"
    Write-Host "  ./build.ps1"
    Write-Host "  ./build.ps1 build -Debug"
    Write-Host "  ./build.ps1 test -Race"
    Write-Host "  `$env:GOOS='linux'; ./build.ps1 build"
    Write-Host "  ./build.ps1 check"
    Write-Host "  ./build.ps1 ci"
}

if ($Help) { $Command = "help" }

switch ($Command) {
    "build"     { Invoke-Build }
    "build-all" { Invoke-BuildAll }
    "run"       { Invoke-Run }
    "test"      { Invoke-Test }
    "cover"     { Invoke-Cover }
    "vet"       { Invoke-Vet }
    "fmt"       { Invoke-Fmt }
    "fmt-check" { Invoke-FmtCheck }
    "lint"      { Invoke-Lint }
    "deps"      { Invoke-Deps }
    "check"     { Invoke-Check }
    "install"   { Invoke-Install }
    "clean"     { Invoke-Clean }
    "ci"        { Invoke-CI }
    "version"   { Invoke-Version }
    "help"      { Invoke-Help }
    default {
        Write-ErrLine "Unknown command: $Command"
        Invoke-Help
        exit 1
    }
}
