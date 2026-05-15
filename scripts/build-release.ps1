#!/usr/bin/env pwsh
# SerialHub Release Build Script (Windows)
# Usage: .\build-release.ps1 [-Version <version>] [-OutputDir <output_dir>]
# Examples:
#   .\build-release.ps1                    # Use version from code
#   .\build-release.ps1 -Version "0.2.0"   # Specify version
#   .\build-release.ps1 -OutputDir "dist"  # Specify output directory

param(
    [string]$Version = "",
    [string]$OutputDir = "dist",
    [switch]$SkipVet = $false
)

$ErrorActionPreference = "Stop"

# Color definitions
$Colors = @{
    Cyan = "Cyan"
    Yellow = "Yellow"
    Green = "Green"
    Red = "Red"
    Gray = "Gray"
    White = "White"
}

# Get base version from code
function Get-BaseVersion {
    $versionFile = Join-Path $PSScriptRoot "..\pkg\version\version.go"
    if (Test-Path $versionFile) {
        $content = Get-Content $versionFile -Raw
        if ($content -match 'Version\s*=\s*"([^"]+)"') {
            return $matches[1]
        }
    }
    return "0.1.0"
}

# Get version: manual > code
$baseVersion = Get-BaseVersion
if (-not $Version) {
    $Version = $baseVersion
}

# Remove v prefix if present
$Version = $Version -replace '^v', ''

Write-Host "========================================" -ForegroundColor $Colors.Cyan
Write-Host "SerialHub Release Build Script" -ForegroundColor $Colors.Cyan
Write-Host "Version: $Version" -ForegroundColor $Colors.Cyan
Write-Host "========================================" -ForegroundColor $Colors.Cyan

# Get project root
$ProjectRoot = Split-Path -Parent $PSScriptRoot
$OriginalLocation = Get-Location

# Ensure return to original directory on exit
trap {
    Set-Location $OriginalLocation
    Write-Host "`nScript exited abnormally, returned to original directory" -ForegroundColor Yellow
    break
}

Set-Location $ProjectRoot

# Create output directory
$DistDir = Join-Path $ProjectRoot $OutputDir
$BuildDir = Join-Path $DistDir "serialhub-$Version-windows-amd64"

$step = 1
$totalSteps = 5

# Step 1: Code check
Write-Host "`n[$step/$totalSteps] Code check..." -ForegroundColor $Colors.Yellow
if (-not $SkipVet) {
    Write-Host "  Running go vet..." -ForegroundColor $Colors.Gray
    go vet ./...
    if ($LASTEXITCODE -ne 0) {
        Write-Error "go vet failed, please fix errors before building"
        exit 1
    }
    Write-Host "  go vet passed" -ForegroundColor $Colors.Green
} else {
    Write-Host "  Skipping go vet (using -SkipVet)" -ForegroundColor $Colors.Gray
}

# Step 2: Clean old build files
$step++
Write-Host "`n[$step/$totalSteps] Clean old build files..." -ForegroundColor $Colors.Yellow
Write-Host "  Output directory: $DistDir" -ForegroundColor $Colors.Gray

# Stop any running serialhub processes
Get-Process -Name "serialhub" -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Start-Sleep -Milliseconds 500

if (Test-Path $DistDir) {
    Remove-Item -Path "$DistDir\*" -Recurse -Force -ErrorAction SilentlyContinue
    Write-Host "  Cleaned output directory" -ForegroundColor $Colors.Gray
}

# Create new build directory
New-Item -ItemType Directory -Force -Path $BuildDir | Out-Null

# Step 3: Build executable
$step++
Write-Host "`n[$step/$totalSteps] Build executable..." -ForegroundColor $Colors.Yellow
$env:CGO_ENABLED = "0"
$BinDir = Join-Path $BuildDir "bin"

New-Item -ItemType Directory -Force -Path $BinDir | Out-Null

# Build settings
$ldflags = "-s -w"
$buildTime = Get-Date -Format "yyyy-MM-dd_HH:mm:ss"
$gitCommit = (git rev-parse --short HEAD 2>$null); if (-not $gitCommit) { $gitCommit = "unknown" }

Write-Host "  Build flags: -ldflags \"$ldflags\"" -ForegroundColor $Colors.Gray
Write-Host "  Building: go build -o bin\serialhub.exe .\cmd\serialhub" -ForegroundColor $Colors.Gray

go build -ldflags "$ldflags" -o "$BinDir\serialhub.exe" .\cmd\serialhub

if ($LASTEXITCODE -ne 0) {
    Write-Error "Build failed"
    exit 1
}

$exeSize = (Get-Item "$BinDir\serialhub.exe").Length / 1KB
Write-Host "  Build success: bin\serialhub.exe ($([math]::Round($exeSize, 2)) KB)" -ForegroundColor $Colors.Green

# Step 4: Copy config files and resources
$step++
Write-Host "`n[$step/$totalSteps] Copy config files and resources..." -ForegroundColor $Colors.Yellow

$filesToCopy = @(
    @{Source = ".\config.example.toml"; Destination = "$BuildDir\config.toml"; Name = "Config"},
    @{Source = ".\start.ps1"; Destination = "$BuildDir\start.ps1"; Name = "Start script"},
    @{Source = ".\README.md"; Destination = "$BuildDir\README.md"; Name = "README"},
    @{Source = ".\MCP.md"; Destination = "$BuildDir\MCP.md"; Name = "MCP doc"},
    @{Source = ".\QUICKSTART.md"; Destination = "$BuildDir\QUICKSTART.md"; Name = "Quick start"},
    @{Source = ".\scripts\opencode.json"; Destination = "$BuildDir\opencode.json"; Name = "OpenCode config"}
)

foreach ($file in $filesToCopy) {
    if (Test-Path $file.Source) {
        Copy-Item -Path $file.Source -Destination $file.Destination -Force
        $fileSize = (Get-Item $file.Destination).Length / 1KB
        Write-Host "  Copy: $($file.Name) -> $(Split-Path $file.Destination -Leaf) ($([math]::Round($fileSize, 2)) KB)" -ForegroundColor $Colors.Gray
    } else {
        Write-Warning "  Skip: $($file.Name) (file not found)"
    }
}

# Create version info file
$versionInfo = @"
SerialHub v$Version
Build Time: $buildTime
Git Commit: $gitCommit
Platform: windows-amd64
"@
$versionInfo | Out-File -FilePath "$BuildDir\VERSION" -Encoding UTF8
Write-Host "  Create: VERSION file" -ForegroundColor $Colors.Gray

# Step 5: Package release files
$step++
Write-Host "`n[$step/$totalSteps] Package release files..." -ForegroundColor $Colors.Yellow
$ZipFile = "$DistDir\serialhub-$Version-windows-amd64.zip"
Write-Host "  Output: $ZipFile" -ForegroundColor $Colors.Gray

# Show package contents
Write-Host "  Package contents:" -ForegroundColor $Colors.Gray
$items = Get-ChildItem $BuildDir -Recurse
foreach ($item in $items) {
    $relativePath = $item.FullName.Substring($BuildDir.Length + 1)
    if ($item.PSIsContainer) {
        Write-Host "    [DIR]  $relativePath" -ForegroundColor $Colors.Gray
    } else {
        $size = $item.Length / 1KB
        Write-Host "    [FILE] $relativePath ($([math]::Round($size, 2)) KB)" -ForegroundColor $Colors.Gray
    }
}

Compress-Archive -Path "$BuildDir\*" -DestinationPath $ZipFile -Force
$zipSize = (Get-Item $ZipFile).Length / 1MB
Write-Host "  Package complete: serialhub-$Version-windows-amd64.zip ($([math]::Round($zipSize, 2)) MB)" -ForegroundColor $Colors.Green

# Complete info
Write-Host "`n========================================" -ForegroundColor $Colors.Green
Write-Host "Release build complete!" -ForegroundColor $Colors.Green
Write-Host "========================================" -ForegroundColor $Colors.Green
Write-Host "Version:    $Version" -ForegroundColor $Colors.White
Write-Host "Build time: $buildTime" -ForegroundColor $Colors.White
Write-Host "Git Commit: $gitCommit" -ForegroundColor $Colors.White
Write-Host "Output:     $ZipFile" -ForegroundColor $Colors.White
Write-Host "File size:  $([math]::Round($zipSize, 2)) MB" -ForegroundColor $Colors.White

# Calculate checksum
$hash = Get-FileHash -Path $ZipFile -Algorithm SHA256
Write-Host "`nSHA256 Checksum:" -ForegroundColor $Colors.Gray
Write-Host "  $($hash.Hash)" -ForegroundColor $Colors.Gray
Write-Host "`nVerify command:" -ForegroundColor $Colors.Gray
Write-Host "  certutil -hashfile \"$ZipFile\" SHA256" -ForegroundColor $Colors.Gray

# Return to original directory
Set-Location $OriginalLocation
