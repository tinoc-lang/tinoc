$ErrorActionPreference = 'Stop'

param (
    [string]$Action = "build"
)

$BuildDir = "build"

# Detect Target OS (respects env:TARGET_OS or env:ODIN_TARGET if set)
if ($env:TARGET_OS) {
    $TargetOS = $env:TARGET_OS
} elseif ($env:ODIN_TARGET) {
    $TargetOS = $env:ODIN_TARGET
} else {
    if ($IsWindows -or $env:OS -like "*Windows*") {
        $TargetOS = "windows"
    } elseif ($IsMacOS) {
        $TargetOS = "darwin"
    } elseif ($IsLinux) {
        $TargetOS = "linux"
    } else {
        $TargetOS = "windows"
    }
}

# Set binary extension
$BinaryName = "tinoc"
if ($TargetOS -eq "windows") {
    $BinaryName = "tinoc.exe"
}

$OutputPath = Join-Path $BuildDir $BinaryName

# Handle clean
if ($Action -eq "clean") {
    if (Test-Path $BuildDir) {
        Remove-Item -Recurse -Force $BuildDir
        Write-Host "Cleaned ${BuildDir}/"
    }
    exit 0
}

# Build steps
if (-not (Test-Path $BuildDir)) {
    New-Item -ItemType Directory -Force -Path $BuildDir | Out-Null
}

Write-Host "Building ${BinaryName} for target: ${TargetOS}..."

# Run Odin Build
odin build . "-out:$OutputPath" -o:speed "-target:$TargetOS"

if ($LASTEXITCODE -ne 0) {
    Write-Error "Build failed with exit code $LASTEXITCODE"
    exit $LASTEXITCODE
}

Write-Host "Build complete -> $OutputPath"

# Print file size
if (Test-Path $OutputPath) {
    $SizeBytes = (Get-Item $OutputPath).Length
    if ($SizeBytes -ge 1MB) {
        $SizeFormatted = "{0:N2} MB" -f ($SizeBytes / 1MB)
    } else {
        $SizeFormatted = "{0:N2} KB" -f ($SizeBytes / 1KB)
    }
    Write-Host "Size: $SizeFormatted"
}

