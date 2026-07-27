# ==============================================================================
# Configuration & Parameters
# ==============================================================================
[CmdletBinding()]
param (
    [Parameter(Position=0)]
    [ValidateSet("release", "debug", "size", "clean", "run", "test")]
    [string]$Mode = "release"
)

$ErrorActionPreference = 'Stop'

$BinaryName = if ($env:BINARY_NAME) { $env:BINARY_NAME } else { "tinoc" }
$BuildDir   = if ($env:BUILD_DIR)   { $env:BUILD_DIR }   else { "build" }

# ==============================================================================
# Dependency Check
# ==============================================================================
if (-not (Get-Command "odin" -ErrorAction SilentlyContinue)) {
    Write-Error "[ERROR] Odin compiler ('odin') not found in PATH."
    exit 1
}

# ==============================================================================
# Target Auto-Detection
# ==============================================================================
if ($env:ODIN_TARGET) {
    $TargetSpec = $env:ODIN_TARGET
} else {
    # 1. Detect OS
    if ($env:TARGET_OS) {
        $OS = $env:TARGET_OS
    } else {
        if ($IsWindows -or $env:OS -like "*Windows*") {
            $OS = "windows"
        } elseif ($IsMacOS) {
            $OS = "darwin"
        } elseif ($IsLinux) {
            $OS = "linux"
        } else {
            $OS = "windows"
        }
    }

    # 2. Detect Architecture
    if ($env:TARGET_ARCH) {
        $ARCH = $env:TARGET_ARCH
    } else {
        $ProcArch = $env:PROCESSOR_ARCHITECTURE
        if (-not $ProcArch -and [System.Runtime.InteropServices.RuntimeInformation]::ProcessArchitecture) {
            $ProcArch = [System.Runtime.InteropServices.RuntimeInformation]::ProcessArchitecture.ToString()
        }

        switch -Regex ($ProcArch) {
            "ARM64|aarch64"     { $ARCH = "arm64" }
            "AMD64|x64|x86_64"  { $ARCH = "amd64" }
            "x86|i386"          { $ARCH = "i386" }
            default             { $ARCH = "amd64" }
        }
    }

    $TargetSpec = "${OS}_${ARCH}"
}

$FinalBinary = $BinaryName
if ($TargetSpec -like "windows*") {
    $FinalBinary = "${BinaryName}.exe"
}
$OutputPath = Join-Path $BuildDir $FinalBinary

# ==============================================================================
# Action Handler
# ==============================================================================
if ($Mode -eq "clean") {
    Write-Host "[INFO] Cleaning build directory (${BuildDir}/)..." -ForegroundColor Blue
    if (Test-Path $BuildDir) {
        Remove-Item -Recurse -Force $BuildDir
    }
    Write-Host "[OK] Clean complete." -ForegroundColor Green
    exit 0
}

if ($Mode -eq "test") {
    Write-Host "[INFO] Running Odin package tests..." -ForegroundColor Blue
    odin test .
    exit 0
}

$OdinFlags = @()
switch ($Mode) {
    "debug"   { $OdinFlags += "-o:none"; $OdinFlags += "-debug" }
    "size"    { $OdinFlags += "-o:size" }
    "release" { $OdinFlags += "-o:speed" }
    "run"     { $OdinFlags += "-o:speed"; $ShouldRun = $true }
}

# ==============================================================================
# Build Execution
# ==============================================================================
if (-not (Test-Path $BuildDir)) {
    New-Item -ItemType Directory -Force -Path $BuildDir | Out-Null
}

Write-Host "[INFO] Building '$BinaryName' for target '$TargetSpec' ($Mode mode)..." -ForegroundColor Blue

$Stopwatch = [System.Diagnostics.Stopwatch]::StartNew()

# Build Execution
& odin build . "-out:$OutputPath" "-target:$TargetSpec" $OdinFlags

if ($LASTEXITCODE -ne 0) {
    Write-Error "[ERROR] Build failed with exit code $LASTEXITCODE"
    exit $LASTEXITCODE
}

$Stopwatch.Stop()
Write-Host "[OK] Build complete -> $OutputPath ($($Stopwatch.ElapsedMilliseconds) ms)" -ForegroundColor Green

# Print Binary Size
if (Test-Path $OutputPath) {
    $SizeBytes = (Get-Item $OutputPath).Length
    if ($SizeBytes -ge 1MB) {
        $SizeFormatted = "{0:N2} MB" -f ($SizeBytes / 1MB)
    } else {
        $SizeFormatted = "{0:N2} KB" -f ($SizeBytes / 1KB)
    }
    Write-Host "[INFO] Binary Size: $SizeFormatted" -ForegroundColor Blue
}

# Run binary if requested
if ($ShouldRun) {
    Write-Host "[INFO] Executing $OutputPath`n" -ForegroundColor Blue
    & ".\$OutputPath"
}
